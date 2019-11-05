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
	// custom
	core "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"math"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"strconv"
	"strings"
)

// FarmmanagerReconciler reconciles a Farmmanager object
type FarmmanagerReconciler struct {
	client.Client
	Log      logr.Logger
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=farmcontroller.toinfn.it,resources=farmmanagers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=farmcontroller.toinfn.it,resources=farmmanagers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *FarmmanagerReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("farmmanager", req.NamespacedName)

	// your logic here
	// get the current farmmanager
	log.Info("fetching Farmmanager resource")
	farmmanager := farmcontrollerv1alpha1.Farmmanager{}
	if err := r.Client.Get(ctx, req.NamespacedName, &farmmanager); err != nil {
		log.Error(err, "failed to get Farmmanager resource")
		return ctrl.Result{}, err
	}

	log.Info("get owned farms")
	farmlist, err := r.getFarms(ctx, farmmanager.Spec.LabelKey, farmmanager.Spec.LabelValue)
	if err != nil {
		log.Error(err, "failed to get owned farms")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// define status variables
	overquota := make(map[string]int32)
	underquota := make(map[string]int32)
	pending := make(map[string]int32)
	tobescaleddown := make(map[string]int32)
	tobescaledup := make(map[string]int32)
	for _, farm := range farmlist.Items {
		name := farm.Namespace + "/" + farm.Name
		log.Info("found farm: " + name)
		overquota[name] = farm.Status.Overquota
		pending[name] = farm.Status.PendingExecutors
		tobescaleddown[name] = 0
		// check if underquota
		var under int32
		if under = farm.Spec.MinExecutors - farm.Status.RunningExecutors; under < 0 {
			under = 0
		}
		underquota[name] = under
		// check if it needs to scale up
		if under > 0 && farm.Status.PendingExecutors > 0 {
			tobescaledup[name] = int32(math.Min(float64(under), float64(farm.Status.PendingExecutors)))
		}
	}
	// check how many pods we need to delete
	tot_scaleup := int32(0)
	for _, val := range tobescaledup {
		tot_scaleup += val
	}
	log.Info("pods to be scaled up: " + strconv.FormatInt(int64(tot_scaleup), 10))

	// if some farms need to scale up, we need to scale down others
	// according to some algo
	if tot_scaleup > 0 {
		tot_scaledown := int32(0)
		for _, val := range overquota {
			tot_scaledown += val
		}
		log.Info("pods to be scaled down: " + strconv.FormatInt(int64(tot_scaleup), 10))
		if tot_scaleup > tot_scaledown {
			log.Info("not enough pods to be scaled down, check your farms' minExecutors values")
			r.Recorder.Event(&farmmanager, core.EventTypeWarning, "Scale", "Not enough pods to be scaled-down to fulfill minExecutors for all farms")
		}
		// here we determine how many pods to kill for each overquota farm
		if tot_scaledown > 0 {
			for farm, over := range overquota {
				tobescaleddown[farm] = int32(math.Ceil(float64(tot_scaleup*over) / float64(tot_scaledown)))
			}
		}
	}

	// Send the scaledown signal to all farms
	for _, farm := range farmlist.Items {
		name := farm.Namespace + "/" + farm.Name
		replicas := int32(0)
		//replicas := int32(-1)
		if scale := tobescaleddown[name]; scale > 0 {
			replicas = farm.Status.RunningExecutors - scale
		}
		farm.Spec.MaxExecutors = &replicas
		log.Info("send scledown signal to farm: " + name + " = " + strconv.FormatInt(int64(replicas), 10))
		err = r.Client.Update(ctx, &farm)
		if err != nil {
			log.Info("error updating farm: " + farm.Name)
		}
	}
	// update the status
	farmmanager.Status.Overquota = overquota
	farmmanager.Status.Underquota = underquota
	farmmanager.Status.Pending = pending
	farmmanager.Status.ToBeScaledDown = tobescaleddown
	err = r.Update(ctx, &farmmanager)
	if err != nil {
		return ctrl.Result{}, err
	}
	// register event
	r.Recorder.Event(&farmmanager, core.EventTypeNormal, "Updated", "Farmmanager status updated")

	return ctrl.Result{Requeue: true}, nil
}

func (r *FarmmanagerReconciler) getFarms(ctx context.Context, key string, value string) (*farmcontrollerv1alpha1.FarmList, error) {

	list := farmcontrollerv1alpha1.FarmList{}
	err := r.List(ctx, &list, client.MatchingLabels{key: value})

	return &list, err
}

func (r *FarmmanagerReconciler) SetupWithManager(mgr ctrl.Manager) error {

	// Define a mapping from the object in the event to one or more
	// objects to Reconcile
	mapFn := handler.ToRequestsFunc(
		func(a handler.MapObject) []reconcile.Request {
			// This will work only for Managers with label matching
			// the Farm label
			// TODO: do not hardcode label
			label := "spark-role"
			farmtype := strings.Split(label, "-")
			return []reconcile.Request{
				{NamespacedName: types.NamespacedName{
					Name:      "farmmanager-" + farmtype[0],
					Namespace: "",
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
		For(&farmcontrollerv1alpha1.Farmmanager{}).
		Watches(&source.Kind{Type: &farmcontrollerv1alpha1.Farm{}},
			&handler.EnqueueRequestsFromMapFunc{
				ToRequests: mapFn,
			}).
		WithEventFilter(p).
		Complete(r)
}
