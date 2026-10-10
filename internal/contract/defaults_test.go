// Copyright 2026 biwsantang. SPDX-License-Identifier: Apache-2.0
package contract

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	api "github.com/biwsantang/claude-code-cloud-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestInheritedDefaultsAndSnapshotCompatibility(t *testing.T) {
	image := "example.invalid/runtime@sha256:" + strings.Repeat("a", 64)
	d := FleetDefaults{RuntimeImage: image, HookImage: "example.invalid/hook@sha256:" + strings.Repeat("b", 64), Proxy: &api.ProxyPolicy{URL: "http://proxy.proxy.svc:3128", Namespace: "proxy", PodLabels: map[string]string{"app": "proxy"}, Port: 3128}}
	f := &api.ClaudeRunnerFleet{ObjectMeta: metav1.ObjectMeta{Name: "team-a"}, Spec: api.ClaudeRunnerFleetSpec{EnvironmentID: "ccpool_example", EnvironmentSecretRef: api.LocalReference{Name: "environment"}, Suspended: true}}
	before := f.DeepCopy()
	resolved := ResolveFleet(f, d)
	if err := ValidateFleet(resolved); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(f, before) || resolved.Spec.OrchestratorImage != image || resolved.Spec.NetworkReportRef.Name != "team-a-network-report" {
		t.Fatal("resolution changed user configuration or inheritance")
	}
	budget := resolved.Spec.Execution.Resources
	if budget.Requests.Memory().Cmp(resource.MustParse("4Gi")) != 0 || budget.Limits.Memory().Cmp(resource.MustParse("4Gi")) != 0 || budget.Requests.Cpu().Cmp(resource.MustParse("2")) != 0 {
		t.Fatal("default session budget incorrect")
	}
	accepted := AcceptedExecution(resolved)
	d.RuntimeImage = "example.invalid/runtime@sha256:" + strings.Repeat("c", 64)
	d.HookImage = "example.invalid/hook@sha256:" + strings.Repeat("d", 64)
	d.Proxy.PodLabels["app"] = "changed"
	if accepted.RunnerImage != image || accepted.Proxy.PodLabels["app"] != "proxy" || accepted.SessionConfigImage != resolved.Spec.HookImage {
		t.Fatal("snapshot aliases mutable defaults")
	}
	// Complete existing manifests win over defaults and retain their old bytes/digest.
	legacy := resolved.DeepCopy()
	wire, _ := json.Marshal(legacy.Spec.Execution)
	newResolved := ResolveFleet(legacy, d)
	newWire, _ := json.Marshal(newResolved.Spec.Execution)
	if string(wire) != string(newWire) || PolicyDigest(legacy.Spec.Execution) != PolicyDigest(newResolved.Spec.Execution) || !reflect.DeepEqual(legacy.Spec, newResolved.Spec) {
		t.Fatal("legacy declared configuration changed")
	}
	// A custom development image doesn't replace the installation's poller runtime.
	f.Spec.Execution.RunnerImage = "example.invalid/custom@sha256:" + strings.Repeat("e", 64)
	custom := ResolveFleet(f, d)
	if custom.Spec.Execution.RunnerImage != f.Spec.Execution.RunnerImage || custom.Spec.OrchestratorImage != d.RuntimeImage {
		t.Fatal("custom runner polluted standard poller image")
	}
}

func TestDefaultsNeverRepairUnsafeOverrides(t *testing.T) {
	d := FleetDefaults{Proxy: &api.ProxyPolicy{URL: "http://proxy.proxy.svc:3128", Namespace: "proxy", PodLabels: map[string]string{"app": "proxy"}, Port: 3128}}
	f := &api.ClaudeRunnerFleet{}
	f.Spec.Execution.Proxy.URL = "http://169.254.169.254:3128"
	resolved := ResolveFleet(f, d)
	if !reflect.DeepEqual(resolved.Spec.Execution.Proxy, f.Spec.Execution.Proxy) || validateProxy(resolved.Spec.Execution.Proxy) == nil {
		t.Fatal("partial unsafe proxy was repaired")
	}
	f.Spec.Execution.Resources.Requests = corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("0")}
	if ResolveFleet(f, d).Spec.Execution.Resources.Requests.Cpu().Sign() != 0 {
		t.Fatal("explicit zero resource budget was replaced")
	}
}

func TestDefaultsDecoderRejectsInvalidInstallation(t *testing.T) {
	for _, raw := range []string{`{"runtimeImage":"runtime:latest"}`, `{"unknown":true}`, `{} {}`, `{"proxy":{"url":"http://169.254.169.254:3128","namespace":"proxy","podLabels":{"app":"proxy"},"port":3128}}`, strings.Repeat(" ", 32769)} {
		if _, err := DecodeDefaults(raw); err == nil {
			t.Fatalf("invalid configuration accepted: %.80s", raw)
		}
	}
	if _, err := DecodeDefaults(`{}`); err != nil {
		t.Fatal(err)
	}
}
