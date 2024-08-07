// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project
// Licensed under the Apache License 2.0

package rendering

import (
	"context"
	"os"
	"path"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	operatorconfig "github.com/stolostron/multicluster-observability-operator/operators/pkg/config"
	rendererutil "github.com/stolostron/multicluster-observability-operator/operators/pkg/rendering"
	templatesutil "github.com/stolostron/multicluster-observability-operator/operators/pkg/rendering/templates"
	"github.com/stretchr/testify/assert"
)

func getAllowlistCM(ns string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operatorconfig.AllowlistConfigMapName,
			Namespace: ns,
		},
		Data: map[string]string{
			metricsConfigMapKey: `
names:
  - apiserver_watch_events_sizes_bucket
`},
	}
}

func TestRender(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working dir %v", err)
	}
	templatesPath := path.Join(path.Dir(path.Dir(wd)), "manifests")
	os.Setenv(templatesutil.TemplatesPathEnvVar, templatesPath)
	defer os.Unsetenv(templatesutil.TemplatesPathEnvVar)

	renderer := rendererutil.NewRenderer()
	hubInfo := &operatorconfig.HubInfo{
		ClusterName:              "foo",
		ObservatoriumAPIEndpoint: "testing.com",
		AlertmanagerEndpoint:     "testing.com",
		AlertmanagerRouterCA:     "testing",
	}
	c := fake.NewClientBuilder().WithRuntimeObjects([]runtime.Object{getAllowlistCM("test-ns")}...).Build()

	objs, err := Render(context.Background(), renderer, c, hubInfo, "test-ns")
	if err != nil {
		t.Fatalf("failed to render endpoint templates: %v", err)
	}

	assert.Greater(t, len(objs), 2)
}
