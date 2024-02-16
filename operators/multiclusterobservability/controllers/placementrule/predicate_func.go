// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package placementrule

import (
	"reflect"

	appsv1 "k8s.io/api/apps/v1"

	addonv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

func getClusterPreds() predicate.Funcs {

	createFunc := func(e event.CreateEvent) bool {
		log.Info("Coleen CreateFunc", "managedCluster", e.Object.GetName())

		if e.Object.GetName() == "local-cluster" {
			log.Info("Coleen CreateFunc", "local-cluster", e.Object.GetName())
			delete(managedClusterList, "local-cluster")
		}

		if isAutomaticAddonInstallationDisabled(e.Object) {
			return false
		}
		if e.Object.GetName() != "local-cluster" {
			updateManagedClusterList(e.Object)
		}
		updateManagedClusterImageRegistry(e.Object)

		return true
	}

	updateFunc := func(e event.UpdateEvent) bool {
		log.Info("UpdateFunc", "managedCluster", e.ObjectNew.GetName())

		//if e.ObjectOld.GetName() == "local-cluster" {
		//	return false
		//}
		if e.ObjectNew.GetResourceVersion() == e.ObjectOld.GetResourceVersion() {
			return false
		}
		if isAutomaticAddonInstallationDisabled(e.ObjectNew) {
			return false
		}

		if e.ObjectNew.GetDeletionTimestamp() != nil {
			log.Info("managedcluster is in terminating state", "managedCluster", e.ObjectNew.GetName())
			managedClusterListMutex.Lock()
			delete(managedClusterList, e.ObjectNew.GetName())
			managedClusterListMutex.Unlock()
			managedClusterImageRegistryMutex.Lock()
			delete(managedClusterImageRegistry, e.ObjectNew.GetName())
			managedClusterImageRegistryMutex.Unlock()
		} else {
			updateManagedClusterList(e.ObjectNew)
			updateManagedClusterImageRegistry(e.ObjectNew)
		}

		return true
	}

	deleteFunc := func(e event.DeleteEvent) bool {
		log.Info("DeleteFunc", "managedCluster", e.Object.GetName())

		if isAutomaticAddonInstallationDisabled(e.Object) {
			return false
		}

		managedClusterListMutex.Lock()
		delete(managedClusterList, e.Object.GetName())
		managedClusterListMutex.Unlock()
		managedClusterImageRegistryMutex.Lock()
		delete(managedClusterImageRegistry, e.Object.GetName())
		managedClusterImageRegistryMutex.Unlock()

		return true
	}

	return predicate.Funcs{
		CreateFunc: createFunc,
		UpdateFunc: updateFunc,
		DeleteFunc: deleteFunc,
	}
}

func GetAddOnDeploymentPredicates() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if !reflect.DeepEqual(e.ObjectNew.(*addonv1alpha1.AddOnDeploymentConfig).Spec.ProxyConfig,
				e.ObjectOld.(*addonv1alpha1.AddOnDeploymentConfig).Spec.ProxyConfig) {
				log.Info("AddonDeploymentConfig is updated", e.ObjectNew.GetName(), "name", e.ObjectNew.GetNamespace(), "namespace")
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return true
		},
	}
}

func getHubEndpointOperatorPredicates() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectNew.GetName() == hubEndpointOperatorName && !reflect.DeepEqual(e.ObjectNew.(*appsv1.Deployment).Spec.Template.Spec,
				e.ObjectOld.(*appsv1.Deployment).Spec.Template.Spec) {
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if e.Object.GetName() == hubEndpointOperatorName {
				return true
			}
			return false
		},
	}
}
