// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package placementrule

import (
	"context"
	"fmt"
	"time"

	"github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/pkg/config"

	"golang.org/x/exp/slices"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	obshared "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/shared"
	obsv1beta1 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta1"
	mcov1beta2 "github.com/stolostron/multicluster-observability-operator/operators/multiclusterobservability/api/v1beta2"
	"github.com/stolostron/multicluster-observability-operator/operators/pkg/util"
)

const (
	obsAddonName          = "observability-addon"
	obsAddonFinalizer     = "observability.open-cluster-management.io/addon-cleanup"
	addonSourceAnnotation = "observability.open-cluster-management.io/addon-source"
	addonSourceMCO        = "mco"
	addonSourceOverride   = "override"
)

func deleteObsAddon(c client.Client, namespace string) error {
	found := &obsv1beta1.ObservabilityAddon{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: obsAddonName, Namespace: namespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		log.Error(err, "Failed to check observabilityaddon cr before delete", "namespace", namespace)
		return err
	}

	err = c.Delete(context.TODO(), found)
	if err != nil {
		log.Error(err, "Failed to delete observabilityaddon", "namespace", namespace)
	}

	err = removeObservabilityAddon(c, namespace)
	if err != nil {
		return err
	}

	// forcely remove observabilityaddon if it's already stuck in Terminating more than 5 minutes
	time.AfterFunc(time.Duration(5)*time.Minute, func() {
		err := deleteStaleObsAddon(c, namespace, false)
		if err != nil {
			log.Error(err, "Failed to forcely remove observabilityaddon", "namespace", namespace)
		}
	})

	log.Info("observabilityaddon is deleted", "namespace", namespace)
	return nil
}

// createObsAddon creates the default ObservabilityAddon in the spoke namespace in the hub cluster.
// It will initially mirror values from the MultiClusterObservability CR with the mco source annotation.
// If an existing addon is found with the mco source annotation it will update the existing addon with the new values.
// If the existing addon is created by the user with the override source annotation, it will not update the existing addon.
func createObsAddon(mco *mcov1beta2.MultiClusterObservability, c client.Client, namespace string) error {
	if namespace == config.GetDefaultNamespace() {
		return nil
	}
	ec := &obsv1beta1.ObservabilityAddon{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ObservabilityAddon",
			APIVersion: "v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      obsAddonName,
			Namespace: namespace,
			Annotations: map[string]string{
				addonSourceAnnotation: addonSourceMCO,
			},
			Labels: map[string]string{
				ownerLabelKey: ownerLabelValue,
			},
		},
	}

	if mco.Spec.ObservabilityAddonSpec != nil {
		setObservabilityAddonSpec(ec, mco.Spec.ObservabilityAddonSpec, config.GetOBAResources(mco.Spec.ObservabilityAddonSpec, mco.Spec.InstanceSize))
	}

	found := &obsv1beta1.ObservabilityAddon{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: obsAddonName, Namespace: namespace}, found)
	if err != nil && errors.IsNotFound(err) || err == nil && found.GetDeletionTimestamp() != nil {
		if err == nil {
			err = deleteFinalizer(c, found)
			if err != nil {
				return err
			}
		}
		log.Info("Creating observabilityaddon cr", "namespace", namespace)
		err = c.Create(context.TODO(), ec)
		if err != nil {
			log.Error(err, "Failed to create observabilityaddon cr")
			return err
		}
		return nil
	} else if err != nil {
		log.Error(err, "Failed to check observabilityaddon cr before create")
		return err
	}

	// Check if existing addon was created by MCO
	if found.Annotations != nil && found.Annotations[addonSourceAnnotation] == addonSourceMCO {
		// Only update if specs are different
		if !equality.Semantic.DeepEqual(found.Spec, ec.Spec) {
			found.Spec = ec.Spec
			err = c.Update(context.TODO(), found)
			if err != nil {
				return fmt.Errorf("failed to update observabilityaddon cr: %w", err)
			}
			log.Info("observabilityaddon updated", "namespace", namespace)
			return nil
		}
	}

	log.Info("observabilityaddon already existed/unchanged", "namespace", namespace)
	return nil
}

func deleteStaleObsAddon(c client.Client, namespace string, isForce bool) error {
	found := &obsv1beta1.ObservabilityAddon{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: obsAddonName, Namespace: namespace}, found)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		log.Error(err, "Failed to check observabilityaddon cr before delete stale ones", "namespace", namespace)
		return err
	}
	if found.GetDeletionTimestamp() == nil && !isForce {
		log.Info("observabilityaddon is not in Terminating status, skip", "namespace", namespace)
		return nil
	}
	err = deleteFinalizer(c, found)
	if err != nil {
		return err
	}
	obsaddon := &obsv1beta1.ObservabilityAddon{
		ObjectMeta: metav1.ObjectMeta{
			Name:      obsAddonName,
			Namespace: namespace,
		},
	}
	err = c.Delete(context.TODO(), obsaddon)
	if err != nil && !errors.IsNotFound(err) {
		log.Error(err, "Failed to delete observabilityaddon", "namespace", namespace)
		return err
	}
	log.Info("observabilityaddon is deleted thoroughly", "namespace", namespace)
	return nil
}

func deleteFinalizer(c client.Client, obsaddon *obsv1beta1.ObservabilityAddon) error {
	if slices.Contains(obsaddon.GetFinalizers(), obsAddonFinalizer) {
		obsaddon.SetFinalizers(util.Remove(obsaddon.GetFinalizers(), obsAddonFinalizer))
		err := c.Update(context.TODO(), obsaddon)
		if err != nil {
			log.Error(err, "Failed to delete finalizer in observabilityaddon", "namespace", obsaddon.Namespace)
			return err
		}
		log.Info("observabilityaddon's finalizer is deleted", "namespace", obsaddon.Namespace)
	}
	return nil
}

// setObservabilityAddonSpec sets the ObservabilityAddon spec fields from the given MCO spec
func setObservabilityAddonSpec(addonSpec *obsv1beta1.ObservabilityAddon, desiredSpec *obshared.ObservabilityAddonSpec, resources *corev1.ResourceRequirements) {
	if desiredSpec != nil {
		addonSpec.Spec.EnableMetrics = desiredSpec.EnableMetrics
		addonSpec.Spec.Interval = desiredSpec.Interval
		addonSpec.Spec.ScrapeSizeLimitBytes = desiredSpec.ScrapeSizeLimitBytes
		addonSpec.Spec.Workers = desiredSpec.Workers
		addonSpec.Spec.Resources = resources
	}
}
