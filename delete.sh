#!/bin/sh
kustomize build config/default | kubectl delete -f -
