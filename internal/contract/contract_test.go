// Copyright 2026 biwsantang. SPDX-License-Identifier: Apache-2.0
package contract

import (
	api "github.com/biwsantang/claude-code-cloud-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"strings"
	"testing"
)

func TestNamesAndBudget(t *testing.T) {
	if Name("pool-a", "order-a") == Name("pool-b", "order-a") || Name("pool", "a") == Name("pool", "b") {
		t.Fatal("dedup key lost identity")
	}
	if strings.Contains(Name("pool", "private-value"), "private-value") {
		t.Fatal("name leaks identity")
	}
	p := api.ExecutionPolicy{RunnerImage: "example.invalid/runtime@sha256:" + strings.Repeat("a", 64), SecurityRevision: "revision-1", Proxy: api.ProxyPolicy{URL: "http://proxy.proxy.svc:3128", Namespace: "proxy", PodLabels: map[string]string{"app": "proxy"}, Port: 3128}, Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1"), corev1.ResourceMemory: resource.MustParse("1Gi")}, Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("2Gi")}}, WorkspaceSize: "8Gi", MaxTokenLifetimeSeconds: 86400, ClockMarginSeconds: 300, DiagnosticRetentionSeconds: 3600}
	if err := ValidateExecution(p); err != nil {
		t.Fatal(err)
	}
	p.Resources.Limits[corev1.ResourceCPU] = resource.MustParse("1")
	if ValidateExecution(p) == nil {
		t.Fatal("CPU limit permitted")
	}
	delete(p.Resources.Limits, corev1.ResourceCPU)
	p.Proxy.URL = "http://169.254.169.254:3128"
	if ValidateExecution(p) == nil {
		t.Fatal("metadata proxy permitted")
	}
}

func TestPortableTolerations(t *testing.T) {
	zero, negative := int64(0), int64(-1)
	for _, toleration := range []corev1.Toleration{
		{Key: "example.invalid/pool", Value: "production", Effect: corev1.TaintEffectNoSchedule},
		{Key: "pool", Operator: corev1.TolerationOpExists},
		{Operator: corev1.TolerationOpExists},
		{Key: "pool", Effect: corev1.TaintEffectNoExecute, TolerationSeconds: &zero},
	} {
		if err := validateToleration(toleration); err != nil {
			t.Fatal("portable placement rejected", err)
		}
	}
	for _, toleration := range []corev1.Toleration{
		{},
		{Key: "bad key", Operator: corev1.TolerationOpExists},
		{Key: "pool", Operator: corev1.TolerationOpExists, Value: "unexpected"},
		{Key: "pool", Operator: "FeatureGatedComparison"},
		{Key: "pool", Effect: "InvalidEffect"},
		{Key: "pool", Value: "bad value"},
		{Key: "pool", Effect: corev1.TaintEffectNoSchedule, TolerationSeconds: &zero},
		{Key: "pool", Effect: corev1.TaintEffectNoExecute, TolerationSeconds: &negative},
	} {
		if validateToleration(toleration) == nil {
			t.Fatal("unsafe or unsupported placement accepted")
		}
	}
}
