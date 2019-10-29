/*

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	farmcontrollerv1alpha1 "farmcontroller/api/v1alpha1"

	// my imports
	core "k8s.io/api/core/v1"
	//meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	//"k8s.io/apimachinery/pkg/runtime"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	c "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"strconv"
	"strings"
)

// FarmReconciler reconciles a Farm object
type FarmReconciler struct {
	client.Client
	Log      logr.Logger
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=farmcontroller.toinfn.it,resources=farms,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=farmcontroller.toinfn.it,resources=farms/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=farmcontroller.toinfn.it,resources=farms/scale,verbs=get;update;patch

// ignore (not requeue) NotFound errors
func ignoreNotFound(err error) error {
	if apierrs.IsNotFound(err) {
		return nil
	}
	return err
}

func (r *FarmReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("farm", req.NamespacedName)

	// your logic here
	log.Info("fetching Farm resource")
	farm := farmcontrollerv1alpha1.Farm{}
	if err := r.Client.Get(ctx, req.NamespacedName, &farm); err != nil {
		if apierrs.IsNotFound(err) {
			log.Info("Pods not owned by this controller, skipping event.")
			err = nil
		} else {
			log.Error(err, "failed to get Farm resource")
		}
		return ctrl.Result{}, err
	}

	log.Info("get owned executors")
	podlist, err := r.getExecutors(ctx, farm.Namespace, farm.Spec.LabelKey, farm.Spec.LabelValue)
	if err != nil {

		log.Error(err, "failed to get owned executors")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// get executor states
	runningPods, pendingPods, failedPods, _ := r.getExecutorStates(podlist)

	log.Info("check for executors in Error state")
	if deleted, err := r.cleanupErrorPods(ctx, log, failedPods); err != nil {
		//log.Error(err, "failed to clean up pods in Error state")
		//return ctrl.Result{}, client.IgnoreNotFound(err)
		return ctrl.Result{}, ignoreNotFound(err)
	} else if deleted > 0 {
		// register event
		r.Recorder.Eventf(&farm, core.EventTypeNormal, "Deleted", "Deleted %d pods", deleted)
	}

	log.Info("check for overquota")
	if overquota := int32(len(runningPods)) - farm.Spec.MinExecutors; overquota > 0 {
		farm.Status.Overquota = overquota
		// register event
		r.Recorder.Eventf(&farm, core.EventTypeNormal, "Overquota", "Over quota by %d executors", overquota)
	} else {
		farm.Status.Overquota = 0
	}

	// Scaledown if requested by farm manager
	if *farm.Spec.Replicas > int32(0) && *farm.Spec.Replicas < int32(len(runningPods)) && farm.Status.Overquota > int32(0) {
		replicas := *farm.Spec.Replicas
		if replicas < farm.Spec.MinExecutors {
			log.Info("cannot scale below MinExecutors, setting desired replicas to: " + strconv.FormatInt(int64(farm.Spec.MinExecutors), 10))
			replicas = farm.Spec.MinExecutors
			farm.Spec.Replicas = &replicas
		}
		log.Info("scaling down")
		if scaled, err := r.scaledownExecutors(ctx, log, runningPods, pendingPods, replicas); err != nil {
			log.Error(err, "failed to scaledown farm")
			return ctrl.Result{}, err
		} else {
			// register event
			r.Recorder.Eventf(&farm, core.EventTypeNormal, "Scaled", "Farm scaled down by "+strconv.FormatInt(int64(scaled), 10))
		}

	}

	// get owned executors again, since they might have changed during reconcile
	log.Info("get owned executors")
	podlist, err = r.getExecutors(ctx, farm.Namespace, farm.Spec.LabelKey, farm.Spec.LabelValue)
	if err != nil {

		log.Error(err, "failed to get owned executors")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// get executor states
	runningPods, pendingPods, failedPods, _ = r.getExecutorStates(podlist)

	// Update the farm status
	log.Info("updating farm status")
	farm.Status.AllExecutors = int32(len(podlist.Items))
	farm.Status.RunningExecutors = int32(len(runningPods))
	farm.Status.Replicas = int32(len(runningPods))
	farm.Status.PendingExecutors = int32(len(pendingPods))
	farm.Status.ErrorExecutors = int32(len(failedPods))

	err = r.Update(ctx, &farm)
	if err != nil {
		return ctrl.Result{}, err
	}
	// register event
	r.Recorder.Event(&farm, core.EventTypeNormal, "Updated", "Farm status updated")
	if &farm.Status.Replicas != farm.Spec.Replicas {
		return ctrl.Result{Requeue: true}, nil
	}
	return ctrl.Result{}, nil
}

func (r *FarmReconciler) SetupWithManager(mgr ctrl.Manager) error {

	// Define a mapping from the object in the event to one or more
	// objects to Reconcile
	mapFn := handler.ToRequestsFunc(
		func(a handler.MapObject) []reconcile.Request {
			// This will work only for Farms with name matching
			// the first 2 strings of the Pod name
			farmname := strings.Split(a.Meta.GetName(), "-")
			return []reconcile.Request{
				{NamespacedName: types.NamespacedName{
					Name:      farmname[0] + "-" + farmname[1],
					Namespace: a.Meta.GetNamespace(),
				}},
			}
		})

	// Judge if an event about the object is what we want.
	// If that is true, the event will be processed by the reconciler.
	p := predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			// The object doesn't contain a correct label,
			// so the event will be ignored.
			// TODO: do not hardcode label
			label := "spark-role"
			if _, ok := e.MetaOld.GetLabels()[label]; !ok {
				return false
			}
			return e.ObjectOld != e.ObjectNew
		},
		CreateFunc: func(e event.CreateEvent) bool {
			// TODO: do not hardcode label
			label := "spark-role"
			if _, ok := e.Meta.GetLabels()[label]; !ok {
				return false
			}
			return true
		},
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&farmcontrollerv1alpha1.Farm{}).
		Watches(&source.Kind{Type: &core.Pod{}},
			&handler.EnqueueRequestsFromMapFunc{
				ToRequests: mapFn,
			}).
		WithEventFilter(p).
		WithOptions(c.Options{MaxConcurrentReconciles: 10}).
		Complete(r)
}

func (r *FarmReconciler) getExecutors(ctx context.Context, namespace string, key string, value string) (*core.PodList, error) {

	list := core.PodList{}
	err := r.List(ctx, &list, client.InNamespace(namespace), client.MatchingLabels{key: value})

	return &list, err
}

func (r *FarmReconciler) getExecutorStates(podlist *core.PodList) ([]*core.Pod, []*core.Pod, []*core.Pod, []*core.Pod) {

	var runningPods []*core.Pod
	var pendingPods []*core.Pod
	var failedPods []*core.Pod
	var otherPods []*core.Pod

	for i, pod := range podlist.Items {
		switch pod.Status.Phase {
		case core.PodRunning:
			runningPods = append(runningPods, &podlist.Items[i])
		case core.PodPending:
			pendingPods = append(pendingPods, &podlist.Items[i])
		case core.PodFailed:
			failedPods = append(failedPods, &podlist.Items[i])
		default:
			otherPods = append(otherPods, &podlist.Items[i])
		}
	}
	return runningPods, pendingPods, failedPods, otherPods
}

func (r *FarmReconciler) scaledownExecutors(ctx context.Context, log logr.Logger, running []*core.Pod, pending []*core.Pod, replicas int32) (int32, error) {

	reduce := int32(len(running)) - replicas
	count := int32(0)
	for _, pod := range running {
		if count < reduce {
			log.Info("Deleting pod: " + pod.Name + " in Running state")
			//gracePeriod := int64(10)
			//deleteOptions := &client.DeleteOptions{GracePeriodSeconds: &gracePeriod}
			//if err := r.Client.Delete(ctx, pod, deleteOptions); err != nil {
			if err := r.Client.Delete(ctx, pod); err != nil {
				log.Error(err, "failed to delete pod resource")
				return count, err
			}
			count += 1
		} else {
			break
		}
	}
	log.Info("Deleted " + strconv.FormatInt(int64(count), 10) + " running pods")

	// we also delete all pending pods to make room for other farms to scale up
	i := 0
	for _, pod := range pending {
		if err := r.Client.Delete(ctx, pod); err != nil {
			log.Error(err, "failed to delete pod resource")
		} else {
			i++
		}
	}
	log.Info("Deleted " + strconv.FormatInt(int64(i), 10) + " pending pods")
	return count, nil

}

func (r *FarmReconciler) cleanupErrorPods(ctx context.Context, log logr.Logger, podlist []*core.Pod) (int, error) {

	count := 0
	for _, pod := range podlist {
		log.Info("Deleting pod: " + pod.Name + " in Error state")
		if err := r.Client.Delete(ctx, pod); err != nil {
			log.Error(err, "failed to delete pod resource")
			return count, ignoreNotFound(err)
		}
		count += 1
	}

	return count, nil

}
