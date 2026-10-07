//go:build integration

// Copyright 2026 biwsantang. SPDX-License-Identifier: Apache-2.0
package controller_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	api "github.com/biwsantang/claude-code-cloud-operator/api/v1alpha1"
	"github.com/biwsantang/claude-code-cloud-operator/internal/admission"
	"github.com/biwsantang/claude-code-cloud-operator/internal/builders"
	"github.com/biwsantang/claude-code-cloud-operator/internal/contract"
	"github.com/biwsantang/claude-code-cloud-operator/internal/controller"
	"github.com/biwsantang/claude-code-cloud-operator/internal/hook"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"os"
	"path/filepath"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	webadmission "sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAPIRecovery(t *testing.T) {
	ctrl.SetLogger(logr.Discard())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	version := os.Getenv("ENVTEST_VERSION")
	if version == "" {
		version = "1.33.0"
	}
	e := &envtest.Environment{CRDDirectoryPaths: []string{filepath.Join("..", "..", "config", "crd", "bases")}, ErrorIfCRDPathMissing: true, DownloadBinaryAssets: os.Getenv("KUBEBUILDER_ASSETS") == "", DownloadBinaryAssetsVersion: version, WebhookInstallOptions: envtest.WebhookInstallOptions{Paths: []string{filepath.Join("..", "..", "config", "webhook", "manifests.yaml")}}}
	cfg, err := e.Start()
	if err != nil {
		t.Fatalf("envtest bootstrap: %v", err)
	}
	defer func() {
		cancel()
		if err := e.Stop(); err != nil {
			t.Errorf("envtest Stop: %v", err)
		}
	}()
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = api.AddToScheme(scheme)
	admin, err := client.New(cfg, client.Options{Scheme: scheme})
	must(t, err)
	namespace := "cloud-operator-system"
	must(t, admin.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}))
	managerCfg := rest.CopyConfig(cfg)
	managerCfg.Impersonate = rest.ImpersonationConfig{UserName: contract.ManagerUser(namespace, "manager"), Groups: []string{"system:masters", "system:authenticated"}}
	manager, err := client.New(managerCfg, client.Options{Scheme: scheme})
	must(t, err)
	server := webhook.NewServer(webhook.Options{Host: e.WebhookInstallOptions.LocalServingHost, Port: e.WebhookInstallOptions.LocalServingPort, CertDir: e.WebhookInstallOptions.LocalServingCertDir})
	admissionReads := &faultReader{Reader: admin}
	server.Register("/validate", &webadmission.Webhook{Handler: &admission.Handler{Reader: admissionReads, Namespace: namespace, ManagerAccount: "manager"}})
	serverDone := make(chan error, 1)
	go func() { serverDone <- server.Start(ctx) }()
	defer func() {
		cancel()
		select {
		case err := <-serverDone:
			if err != nil {
				t.Errorf("webhook stop: %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("webhook did not stop")
		}
	}()
	deadline := time.Now().Add(10 * time.Second)
	for server.StartedChecker()(nil) != nil {
		if time.Now().After(deadline) {
			t.Fatal("webhook not ready")
		}
		time.Sleep(50 * time.Millisecond)
	}
	fixture := func(t *testing.T, name string) (*api.ClaudeRunnerFleet, client.Client, []byte) {
		t.Helper()
		image := "example.invalid/runtime@sha256:" + strings.Repeat("a", 64)
		f := &api.ClaudeRunnerFleet{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}, Spec: api.ClaudeRunnerFleetSpec{EnvironmentID: "ccpool_" + name, EnvironmentSecretRef: api.LocalReference{Name: name + "-environment"}, OrchestratorImage: image, HookImage: image, Suspended: true, OrchestratorReplicas: 2, HookTimeoutSeconds: 15, SpawnLeaseSeconds: 120, NetworkReportRef: api.LocalReference{Name: name + "-network"}, DeletionPolicy: "Drain", Execution: api.ExecutionPolicy{RunnerImage: image, SecurityRevision: "revision-1", Proxy: api.ProxyPolicy{URL: "http://proxy.proxy.svc:3128", Namespace: "proxy", PodLabels: map[string]string{"app": "proxy"}, Port: 3128}, Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1"), corev1.ResourceMemory: resource.MustParse("1Gi")}, Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("2Gi")}}, WorkspaceSize: "8Gi", MaxTokenLifetimeSeconds: 86400, ClockMarginSeconds: 60, DiagnosticRetentionSeconds: 300}}}
		must(t, admin.Create(ctx, f))
		fr := &controller.ClaudeRunnerFleetReconciler{Client: manager, Scheme: scheme, AdmissionReady: func(context.Context) bool { return true }}
		_, err := fr.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(f)})
		must(t, err)
		must(t, manager.Get(ctx, client.ObjectKeyFromObject(f), f))
		must(t, admin.Create(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: f.Spec.EnvironmentSecretRef.Name, Namespace: namespace}, Data: map[string][]byte{contract.EnvironmentKey: []byte("synthetic-environment")}}))
		report := contract.Report{FleetUID: string(f.UID), PolicyDigest: contract.PolicyDigest(f.Spec.Execution), TestedAt: time.Now().UTC(), ValidUntil: time.Now().Add(time.Hour).UTC(), DirectDenied: true, PrivateDenied: true, MetadataDenied: true, KubernetesDenied: true, ProxyAllowed: true, ProxyDenied: true, FreshPodTested: true, Evidence: "synthetic fixture; not an enforced-network acceptance claim"}
		b, _ := json.Marshal(report)
		must(t, admin.Create(ctx, &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: f.Spec.NetworkReportRef.Name, Namespace: namespace}, Data: map[string]string{"report.json": string(b)}}))
		f.Spec.Suspended = false
		must(t, admin.Update(ctx, f))
		_, err = fr.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(f)})
		must(t, err)
		hookCfg := rest.CopyConfig(cfg)
		hookCfg.Impersonate = rest.ImpersonationConfig{UserName: contract.ManagerUser(namespace, contract.HookAccount(name)), Groups: []string{"system:authenticated", "system:serviceaccounts", "system:serviceaccounts:" + namespace}}
		hc, err := client.New(hookCfg, client.Options{Scheme: scheme})
		must(t, err)
		payload := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"exp":%d}`, time.Now().Add(time.Hour).Unix())))
		must(t, manager.Get(ctx, client.ObjectKeyFromObject(f), f))
		return f, hc, []byte("synthetic." + payload + ".not-a-signature")
	}
	intake := func(t *testing.T, f *api.ClaudeRunnerFleet, hc client.Client, token []byte, order string) *api.ClaudeWorkOrder {
		t.Helper()
		in := hook.Input{Namespace: namespace, Fleet: f.Name, Pool: f.Spec.EnvironmentID, Order: order}
		var result hook.Result
		for i := 0; i < 8; i++ {
			result = hook.Run(ctx, hc, in, token, time.Now())
			if result.Code == 0 {
				break
			}
			if result.Code == hook.Permanent {
				t.Fatalf("intake rejected: %s", result.Reason)
			}
		}
		if result.Code != 0 {
			t.Fatalf("intake failed: %s", result.Reason)
		}
		w := &api.ClaudeWorkOrder{}
		must(t, manager.Get(ctx, client.ObjectKey{Namespace: namespace, Name: result.Name}, w))
		encoded, err := json.Marshal(w)
		must(t, err)
		if strings.Contains(string(encoded), string(token)) {
			t.Fatal("receipt leaked credential")
		}
		return w
	}
	t.Run("schema-default-and-activation-guards", func(t *testing.T) {
		f, hc, _ := fixture(t, "guards")
		_ = hc
		if f.Spec.Suspended {
			t.Fatal("fixture activation failed")
		}
		bad := f.DeepCopy()
		bad.Spec.EnvironmentSecretRef.Namespace = "other"
		if admin.Update(ctx, bad) == nil {
			t.Fatal("cross-namespace secret reference admitted")
		}
		bad = f.DeepCopy()
		bad.Spec.Execution.Resources.Limits[corev1.ResourceCPU] = resource.MustParse("1")
		if admin.Update(ctx, bad) == nil {
			t.Fatal("unsafe CPU limit admitted")
		}
		bad = f.DeepCopy()
		bad.Spec.Suspended = true
		bad.Spec.Execution.DiagnosticRetentionSeconds = 604801
		if admin.Update(ctx, bad) == nil {
			t.Fatal("uncapped diagnostic retention admitted")
		}
		bad = f.DeepCopy()
		bad.Spec.Suspended = true
		bad.Spec.Execution.MaxTokenLifetimeSeconds = 604801
		if admin.Update(ctx, bad) == nil {
			t.Fatal("uncapped token lifetime admitted")
		}
		cm := &corev1.ConfigMap{}
		must(t, admin.Get(ctx, client.ObjectKey{Namespace: namespace, Name: f.Spec.NetworkReportRef.Name}, cm))
		must(t, admin.Delete(ctx, cm))
		f.Spec.Suspended = true
		must(t, admin.Update(ctx, f))
		f.Spec.Suspended = false
		if admin.Update(ctx, f) == nil {
			t.Fatal("activation without report admitted")
		}
		fr := &controller.ClaudeRunnerFleetReconciler{Client: manager, Scheme: scheme, AdmissionReady: func(context.Context) bool { return true }}
		_, err := fr.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(f)})
		must(t, err)
		d := &appsv1.Deployment{}
		must(t, manager.Get(ctx, client.ObjectKey{Namespace: namespace, Name: f.Name + "-orchestrator"}, d))
		if *d.Spec.Replicas != 0 {
			t.Fatal("polling enabled without report")
		}
	})
	t.Run("fleet-converges-without-writes", func(t *testing.T) {
		f, _, _ := fixture(t, "stable")
		d := &appsv1.Deployment{}
		key := client.ObjectKey{Namespace: namespace, Name: f.Name + "-orchestrator"}
		must(t, manager.Get(ctx, key, d))
		revision := d.ResourceVersion
		fr := &controller.ClaudeRunnerFleetReconciler{Client: manager, Scheme: scheme, AdmissionReady: func(context.Context) bool { return true }}
		for i := 0; i < 3; i++ {
			_, err := fr.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(f)})
			must(t, err)
		}
		must(t, manager.Get(ctx, key, d))
		if d.ResourceVersion != revision {
			t.Fatal("unchanged reconcile rewrote Deployment")
		}
	})
	t.Run("credential-rotation-network-and-unrelated-collision", func(t *testing.T) {
		f, hc, token := fixture(t, "rotation")
		w := intake(t, f, hc, token, "order-before-rotation")
		fr := &controller.ClaudeRunnerFleetReconciler{Client: manager, Scheme: scheme, AdmissionReady: func(context.Context) bool { return true }}
		key := client.ObjectKey{Namespace: namespace, Name: f.Name + "-orchestrator"}
		d := &appsv1.Deployment{}
		must(t, manager.Get(ctx, key, d))
		before := d.Spec.Template.Annotations[contract.Group+"/credential-revision"]
		s := &corev1.Secret{}
		must(t, admin.Get(ctx, client.ObjectKey{Namespace: namespace, Name: f.Spec.EnvironmentSecretRef.Name}, s))
		s.Data[contract.EnvironmentKey] = []byte("rotated-synthetic-environment")
		must(t, admin.Update(ctx, s))
		_, err := fr.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(f)})
		must(t, err)
		must(t, manager.Get(ctx, key, d))
		if d.Spec.Template.Annotations[contract.Group+"/credential-revision"] == before {
			t.Fatal("rotation did not roll poller")
		}
		must(t, manager.Get(ctx, client.ObjectKeyFromObject(w), w))
		if !w.Spec.Complete || w.Status.LaunchAttempted {
			t.Fatal("rotation changed accepted receipt")
		}
		cm := &corev1.ConfigMap{}
		must(t, admin.Get(ctx, client.ObjectKey{Namespace: namespace, Name: f.Spec.NetworkReportRef.Name}, cm))
		var report contract.Report
		must(t, json.Unmarshal([]byte(cm.Data["report.json"]), &report))
		report.ValidUntil = time.Now().Add(-time.Minute)
		b, _ := json.Marshal(report)
		cm.Data["report.json"] = string(b)
		must(t, admin.Update(ctx, cm))
		_, err = fr.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(f)})
		must(t, err)
		must(t, manager.Get(ctx, key, d))
		if *d.Spec.Replicas != 0 {
			t.Fatal("stale report left poller enabled")
		}
		// An unrelated deterministic-name collision must never be adopted or rewritten.
		must(t, admin.Delete(ctx, d))
		d = &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: key.Name, Namespace: namespace}, Spec: appsv1.DeploymentSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"foreign": "true"}}, Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"foreign": "true"}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "foreign", Image: "example.invalid/foreign"}}}}}}
		must(t, admin.Create(ctx, d))
		rv := d.ResourceVersion
		_, err = fr.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(f)})
		if err == nil {
			t.Fatal("foreign deployment adopted")
		}
		must(t, admin.Get(ctx, key, d))
		if d.ResourceVersion != rv || len(d.OwnerReferences) != 0 {
			t.Fatal("foreign resource rewritten")
		}
	})
	t.Run("foreign-credential-collision", func(t *testing.T) {
		f, hc, token := fixture(t, "collision")
		name := contract.Name(f.Spec.EnvironmentID, "order-collision")
		foreign := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name + "-credential", Namespace: namespace}, Data: map[string][]byte{"foreign": []byte("synthetic")}}
		must(t, admin.Create(ctx, foreign))
		rv := foreign.ResourceVersion
		result := hook.Run(ctx, hc, hook.Input{Namespace: namespace, Fleet: f.Name, Pool: f.Spec.EnvironmentID, Order: "order-collision"}, token, time.Now())
		if result.Code != hook.Permanent {
			t.Fatal("foreign credential accepted")
		}
		must(t, admin.Get(ctx, client.ObjectKeyFromObject(foreign), foreign))
		if foreign.ResourceVersion != rv || len(foreign.OwnerReferences) != 0 {
			t.Fatal("foreign secret modified")
		}
		w := &api.ClaudeWorkOrder{}
		must(t, manager.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, w))
		if w.Spec.Complete {
			t.Fatal("collision completed receipt")
		}
	})
	t.Run("concurrent-intake-and-mismatch", func(t *testing.T) {
		f, hc, token := fixture(t, "concurrent")
		in := hook.Input{Namespace: namespace, Fleet: f.Name, Pool: f.Spec.EnvironmentID, Order: "order-one"}
		var wg sync.WaitGroup
		results := make(chan hook.Result, 12)
		for i := 0; i < 12; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); results <- hook.Run(ctx, hc, in, token, time.Now()) }()
		}
		wg.Wait()
		close(results)
		accepted := 0
		for r := range results {
			if r.Code == 0 {
				accepted++
			}
		}
		if accepted == 0 {
			t.Fatal("no concurrent caller succeeded")
		}
		w := intake(t, f, hc, token, "order-one")
		if !w.Spec.Complete {
			t.Fatal("receipt not completed")
		}
		changedToken := append(append([]byte{}, token...), []byte("changed")...)
		if r := hook.Run(ctx, hc, in, changedToken, time.Now()); r.Code != hook.Permanent {
			t.Fatal("credential mismatch accepted")
		}
		s := &corev1.Secret{}
		if hc.Get(ctx, client.ObjectKey{Namespace: namespace, Name: f.Spec.EnvironmentSecretRef.Name}, s) == nil {
			t.Fatal("hook can read environment key")
		}
		altered := w.DeepCopy()
		altered.Spec.Execution.NodeSelector = map[string]string{"unsafe": "change"}
		if hc.Update(ctx, altered) == nil {
			t.Fatal("snapshot changed")
		}
		altered = w.DeepCopy()
		altered.Status.LaunchAttempted = true
		if hc.Status().Update(ctx, altered) == nil {
			t.Fatal("hook can write launch fence")
		}
	})
	t.Run("admission-rejects-fleet-uid-expiry-and-foreign-actors", func(t *testing.T) {
		f, hc, token := fixture(t, "actors")
		for _, mutation := range []string{"fleet-uid", "expiry"} {
			in := hook.Input{Namespace: namespace, Fleet: f.Name, Pool: f.Spec.EnvironmentID, Order: "order-" + mutation}
			r := hook.Run(ctx, &mutatedReceiptClient{Client: hc, mutation: mutation}, in, token, time.Now())
			if r.Code != hook.Permanent {
				t.Fatalf("%s mutation accepted or transient", mutation)
			}
			w := &api.ClaudeWorkOrder{}
			err := manager.Get(ctx, client.ObjectKey{Namespace: namespace, Name: contract.Name(in.Pool, in.Order)}, w)
			if mutation == "fleet-uid" && !apierrors.IsNotFound(err) {
				t.Fatal("wrong Fleet UID created a receipt")
			}
			if mutation == "expiry" {
				must(t, err)
				if w.Spec.Complete {
					t.Fatal("mismatched token expiry completed")
				}
			}
		}
		w := intake(t, f, hc, token, "order-actor")
		// Administrator authorization alone cannot perform hook-only receipt updates.
		if admin.Update(ctx, w.DeepCopy()) == nil {
			t.Fatal("foreign actor redelivery admitted")
		}
		if hc.Delete(ctx, w) == nil {
			t.Fatal("hook can remove retained receipt")
		}
		altered := w.DeepCopy()
		altered.Spec.Complete = false
		if hc.Update(ctx, altered) == nil {
			t.Fatal("completed receipt reopened")
		}
		altered = w.DeepCopy()
		altered.Spec.Execution.SecurityRevision = "mutated"
		if hc.Update(ctx, altered) == nil {
			t.Fatal("accepted security snapshot changed")
		}
		secret := &corev1.Secret{}
		must(t, manager.Get(ctx, client.ObjectKey{Namespace: namespace, Name: w.Spec.CredentialSecretRef.Name}, secret))
		secret.Data[contract.CredentialKey] = []byte("synthetic replacement")
		if hc.Update(ctx, secret) == nil {
			t.Fatal("hook mutated immutable credential")
		}
	})
	t.Run("admission-api-read-faults-remain-retryable", func(t *testing.T) {
		f, hc, token := fixture(t, "api-read-faults")
		for _, boundary := range []string{"fleet", "environment", "report", "receipt", "credential"} {
			in := hook.Input{Namespace: namespace, Fleet: f.Name, Pool: f.Spec.EnvironmentID, Order: "order-" + boundary}
			name := contract.Name(in.Pool, in.Order)
			target := map[string]string{"fleet": f.Name, "environment": f.Spec.EnvironmentSecretRef.Name, "report": f.Spec.NetworkReportRef.Name, "receipt": name, "credential": name + "-credential"}[boundary]
			admissionReads.arm(target)
			result := hook.Run(ctx, hc, in, token, time.Now())
			if result.Code != hook.Retryable {
				t.Fatalf("%s read failure classified as %d", boundary, result.Code)
			}
			if strings.Contains(result.Reason, "sensitive") || strings.Contains(result.Reason, string(token)) {
				t.Fatal("admission infrastructure response leaked source text")
			}
			w := intake(t, f, hc, token, in.Order)
			if !w.Spec.Complete {
				t.Fatal("retry after API recovery did not repair intake")
			}
		}
	})
	t.Run("expired-pending-startup-races", func(t *testing.T) {
		for _, boundary := range []string{"never-started", "fresh-read", "delete-conflict"} {
			t.Run(boundary, func(t *testing.T) {
				f, hc, token := fixture(t, "startup-"+boundary)
				w := intake(t, f, hc, token, "order-startup")
				var calls atomic.Int32
				r := &controller.ClaudeWorkOrderReconciler{Client: manager, Scheme: scheme, AdmissionReady: func(context.Context) bool { return true }, CreatePod: func(ctx context.Context, p *corev1.Pod) error { calls.Add(1); return manager.Create(ctx, p) }}
				_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(w)})
				must(t, err)
				r.Now = func() time.Time {
					return w.Spec.ExpiresAt.Add(time.Duration(w.Spec.Execution.ClockMarginSeconds+1) * time.Second)
				}
				if boundary != "never-started" {
					r.Client = &startupRaceClient{Client: manager, boundary: boundary}
				}
				_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(w)})
				if boundary == "delete-conflict" {
					if !apierrors.IsConflict(err) {
						t.Fatal("startup resourceVersion race did not reject delete")
					}
				} else {
					must(t, err)
				}
				// Restart after either the observation or conflict; the fence must survive.
				r.Client = manager
				for i := 0; i < 2; i++ {
					_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(w)})
					must(t, err)
				}
				must(t, manager.Get(ctx, client.ObjectKeyFromObject(w), w))
				p := &corev1.Pod{}
				err = manager.Get(ctx, client.ObjectKey{Namespace: namespace, Name: w.Name}, p)
				if boundary == "never-started" {
					if !apierrors.IsNotFound(err) || !w.Status.Terminal {
						t.Fatal("expired pending execution remained")
					}
				} else {
					must(t, err)
					if !p.DeletionTimestamp.IsZero() || p.Status.Phase != corev1.PodRunning || w.Status.Terminal {
						t.Fatal("observed startup was terminated")
					}
				}
				if calls.Load() != 1 {
					t.Fatal("expiry recovery resubmitted Pod")
				}
			})
		}
	})
	t.Run("abort-restart-preserves-unrelated-resources-and-retention", func(t *testing.T) {
		f, hc, token := fixture(t, "abort")
		w := intake(t, f, hc, token, "order-abort")
		r := &controller.ClaudeWorkOrderReconciler{Client: manager, Scheme: scheme, AdmissionReady: func(context.Context) bool { return true }, CreatePod: func(ctx context.Context, p *corev1.Pod) error { return manager.Create(ctx, p) }}
		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(w)})
		must(t, err)
		p := &corev1.Pod{}
		must(t, manager.Get(ctx, client.ObjectKey{Namespace: namespace, Name: w.Name}, p))
		p.Status.Phase = corev1.PodRunning
		must(t, manager.Status().Update(ctx, p))
		foreign := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "abort-unrelated", Namespace: namespace, Labels: map[string]string{contract.FleetLabel: f.Name}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "foreign", Image: "example.invalid/foreign"}}}}
		must(t, admin.Create(ctx, foreign))
		foreignUID := foreign.UID
		must(t, admin.Get(ctx, client.ObjectKeyFromObject(f), f))
		f.Spec.DeletionPolicy = "Abort"
		must(t, admin.Update(ctx, f))
		must(t, admin.Delete(ctx, f))
		fr := &controller.ClaudeRunnerFleetReconciler{Client: manager, Scheme: scheme}
		_, err = fr.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(f)})
		must(t, err)
		err = manager.Get(ctx, client.ObjectKeyFromObject(p), p)
		if err == nil {
			if p.DeletionTimestamp.IsZero() {
				t.Fatal("explicit abort did not request termination")
			}
			// Envtest has no kubelet; model termination acknowledgement if a grace period remains.
			must(t, manager.Delete(ctx, p, client.GracePeriodSeconds(0)))
		} else if !apierrors.IsNotFound(err) {
			must(t, err)
		}
		must(t, manager.Get(ctx, client.ObjectKeyFromObject(f), f))
		waiting := apimeta.FindStatusCondition(f.Status.Conditions, "Ready")
		if waiting == nil || waiting.Reason != "DrainingOrdersAndRetention" || waiting.ObservedGeneration != f.Generation || f.Status.ObservedGeneration != f.Generation {
			t.Fatal("abort wait did not report current generation")
		}
		r = &controller.ClaudeWorkOrderReconciler{Client: manager, Scheme: scheme}
		_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(w)})
		must(t, err)
		must(t, manager.Get(ctx, client.ObjectKeyFromObject(w), w))
		if !w.Status.Terminal || w.Status.RetainUntil == nil {
			t.Fatal("abort restart lost diagnostic retention")
		}
		_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(w)})
		must(t, err)
		if manager.Get(ctx, client.ObjectKeyFromObject(w), w) != nil {
			t.Fatal("retained receipt removed prematurely")
		}
		r.Now = func() time.Time { return w.Status.RetainUntil.Add(time.Second) }
		_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(w)})
		must(t, err)
		fr = &controller.ClaudeRunnerFleetReconciler{Client: manager, Scheme: scheme}
		_, err = fr.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(f)})
		must(t, err)
		if err = manager.Get(ctx, client.ObjectKeyFromObject(f), f); !apierrors.IsNotFound(err) {
			t.Fatal("abort did not finish finalizer cleanup")
		}
		must(t, manager.Get(ctx, client.ObjectKeyFromObject(foreign), foreign))
		if foreign.UID != foreignUID || !foreign.DeletionTimestamp.IsZero() {
			t.Fatal("abort affected unrelated Pod")
		}
	})
	t.Run("abandoned-incomplete-intake-cleanup", func(t *testing.T) {
		f, hc, token := fixture(t, "abandoned")
		in := hook.Input{Namespace: namespace, Fleet: f.Name, Pool: f.Spec.EnvironmentID, Order: "order-abandoned"}
		result := hook.Run(ctx, &incompleteClient{Client: hc}, in, token, time.Now())
		if result.Code != hook.Retryable {
			t.Fatal("partial intake not retryable")
		}
		w := &api.ClaudeWorkOrder{}
		must(t, manager.Get(ctx, client.ObjectKey{Namespace: namespace, Name: contract.Name(in.Pool, in.Order)}, w))
		if w.Spec.Complete {
			t.Fatal("fault completed intake")
		}
		var calls atomic.Int32
		r := &controller.ClaudeWorkOrderReconciler{Client: manager, Scheme: scheme, AdmissionReady: func(context.Context) bool { return true }, CreatePod: func(context.Context, *corev1.Pod) error { calls.Add(1); return nil }}
		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(w)})
		must(t, err)
		r.Now = func() time.Time { return w.Spec.ExpiresAt.Add(time.Minute) }
		for i := 0; i < 2; i++ {
			_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(w)})
			must(t, err)
		}
		must(t, manager.Get(ctx, client.ObjectKeyFromObject(w), w))
		if !w.Status.Terminal || w.Status.LaunchAttempted || w.Status.RetainUntil == nil {
			t.Fatal("abandoned receipt not terminal and retained")
		}
		secret := &corev1.Secret{}
		err = manager.Get(ctx, client.ObjectKey{Namespace: namespace, Name: w.Spec.CredentialSecretRef.Name}, secret)
		if !apierrors.IsNotFound(err) {
			t.Fatal("abandoned credential remained")
		}
		r = &controller.ClaudeWorkOrderReconciler{Client: manager, Scheme: scheme, Now: func() time.Time { return w.Status.RetainUntil.Add(time.Second) }}
		_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(w)})
		must(t, err)
		if err = manager.Get(ctx, client.ObjectKeyFromObject(w), w); !apierrors.IsNotFound(err) {
			t.Fatal("expired tombstone remained")
		}
		if calls.Load() != 0 {
			t.Fatal("incomplete intake submitted Pod")
		}
	})
	t.Run("crash-after-fence-and-late-pod", func(t *testing.T) {
		f, hc, token := fixture(t, "fence")
		w := intake(t, f, hc, token, "order-fence")
		var calls atomic.Int32
		r := &controller.ClaudeWorkOrderReconciler{Client: manager, Scheme: scheme, AdmissionReady: func(context.Context) bool { return true }, CreatePod: func(ctx context.Context, p *corev1.Pod) error { calls.Add(1); return manager.Create(ctx, p) }, AfterFence: func() error { return errors.New("synthetic crash") }}
		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(w)})
		if err == nil {
			t.Fatal("fault not injected")
		}
		r.AfterFence = nil
		for i := 0; i < 4; i++ {
			_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(w)})
			must(t, err)
		}
		if calls.Load() != 0 {
			t.Fatal("crashed fence was resubmitted")
		}
		must(t, manager.Get(ctx, client.ObjectKeyFromObject(w), w))
		if !w.Status.LaunchAttempted || !w.Status.SubmissionUncertain {
			t.Fatal("uncertainty not persisted")
		}
		late := builders.Runner(w)
		must(t, manager.Create(ctx, late))
		_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(w)})
		must(t, err)
		must(t, manager.Get(ctx, client.ObjectKeyFromObject(w), w))
		if w.Status.PodUID != string(late.UID) {
			t.Fatal("late Pod not bound")
		}
		must(t, manager.Delete(ctx, late))
		_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(w)})
		must(t, err)
		if calls.Load() != 0 {
			t.Fatal("lost Pod replaced")
		}
	})
	t.Run("lost-create-response-and-concurrent-launch", func(t *testing.T) {
		f, hc, token := fixture(t, "response")
		w := intake(t, f, hc, token, "order-response")
		var calls atomic.Int32
		makeR := func() *controller.ClaudeWorkOrderReconciler {
			return &controller.ClaudeWorkOrderReconciler{Client: manager, Scheme: scheme, AdmissionReady: func(context.Context) bool { return true }, CreatePod: func(ctx context.Context, p *corev1.Pod) error {
				calls.Add(1)
				if err := manager.Create(ctx, p); err != nil {
					return err
				}
				return errors.New("synthetic response loss")
			}}
		}
		var wg sync.WaitGroup
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, _ = makeR().Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(w)})
			}()
		}
		wg.Wait()
		r := makeR()
		for i := 0; i < 3; i++ {
			_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(w)})
			must(t, err)
		}
		if calls.Load() != 1 {
			t.Fatalf("Pod POST count=%d", calls.Load())
		}
		must(t, manager.Get(ctx, client.ObjectKeyFromObject(w), w))
		if !w.Status.PodObserved || w.Status.SubmissionUncertain {
			t.Fatal("ambiguous accepted Pod not recovered")
		}
	})
	t.Run("partial-intake-repairs-response-loss", func(t *testing.T) {
		f, hc, token := fixture(t, "partial")
		in := hook.Input{Namespace: namespace, Fleet: f.Name, Pool: f.Spec.EnvironmentID, Order: "order-partial"}
		lost := &lostResponseClient{Client: hc, kind: "receipt"}
		if r := hook.Run(ctx, lost, in, token, time.Now()); r.Code != hook.Retryable {
			t.Fatal("lost receipt response not retryable")
		}
		lost = &lostResponseClient{Client: hc, kind: "secret"}
		if r := hook.Run(ctx, lost, in, token, time.Now()); r.Code != hook.Retryable {
			t.Fatal("lost Secret response not retryable")
		}
		w := intake(t, f, hc, token, "order-partial")
		if !w.Spec.Complete {
			t.Fatal("partial transaction not repaired")
		}
		in.Order = "order-completion"
		lost = &lostResponseClient{Client: hc, kind: "completion"}
		if result := hook.Run(ctx, lost, in, token, time.Now()); result.Code != hook.Retryable {
			t.Fatal("lost completion response not retryable")
		}
		intake(t, f, hc, token, in.Order)
	})
	t.Run("suspension-and-expiry-preserve-running", func(t *testing.T) {
		f, hc, token := fixture(t, "suspend")
		w := intake(t, f, hc, token, "order-active")
		waiting := intake(t, f, hc, token, "order-waiting")
		var calls atomic.Int32
		r := &controller.ClaudeWorkOrderReconciler{Client: manager, Scheme: scheme, AdmissionReady: func(context.Context) bool { return true }, CreatePod: func(ctx context.Context, p *corev1.Pod) error { calls.Add(1); return manager.Create(ctx, p) }}
		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(w)})
		must(t, err)
		p := &corev1.Pod{}
		must(t, manager.Get(ctx, client.ObjectKey{Namespace: namespace, Name: w.Name}, p))
		p.Status.Phase = corev1.PodRunning
		must(t, manager.Status().Update(ctx, p))
		must(t, admin.Get(ctx, client.ObjectKeyFromObject(f), f))
		f.Spec.Suspended = true
		must(t, admin.Update(ctx, f))
		_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(waiting)})
		must(t, err)
		if calls.Load() != 1 {
			t.Fatal("suspended order launched")
		}
		r.Now = func() time.Time { return w.Spec.ExpiresAt.Add(2 * time.Hour) }
		_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(w)})
		must(t, err)
		must(t, manager.Get(ctx, client.ObjectKey{Namespace: namespace, Name: w.Name}, p))
		if !p.DeletionTimestamp.IsZero() {
			t.Fatal("running Pod deleted on expiry")
		}
		_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(waiting)})
		must(t, err)
		must(t, manager.Get(ctx, client.ObjectKeyFromObject(waiting), waiting))
		if !waiting.Status.Terminal || waiting.Status.LaunchAttempted {
			t.Fatal("expired unlaunched order not retained")
		}
	})
	t.Run("drain-waits-and-keeps-diagnostic-floor", func(t *testing.T) {
		f, hc, token := fixture(t, "drain")
		w := intake(t, f, hc, token, "order-drain")
		r := &controller.ClaudeWorkOrderReconciler{Client: manager, Scheme: scheme, AdmissionReady: func(context.Context) bool { return true }, CreatePod: func(ctx context.Context, p *corev1.Pod) error { return manager.Create(ctx, p) }}
		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(w)})
		must(t, err)
		p := &corev1.Pod{}
		must(t, manager.Get(ctx, client.ObjectKey{Namespace: namespace, Name: w.Name}, p))
		p.Status.Phase = corev1.PodRunning
		must(t, manager.Status().Update(ctx, p))
		must(t, admin.Get(ctx, client.ObjectKeyFromObject(f), f))
		must(t, admin.Delete(ctx, f))
		fr := &controller.ClaudeRunnerFleetReconciler{Client: manager, Scheme: scheme}
		_, err = fr.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(f)})
		must(t, err)
		_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(w)})
		must(t, err)
		must(t, manager.Get(ctx, client.ObjectKeyFromObject(p), p))
		if !p.DeletionTimestamp.IsZero() {
			t.Fatal("drain terminated active Pod")
		}
		p.Status.Phase = corev1.PodSucceeded
		must(t, manager.Status().Update(ctx, p))
		_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(w)})
		must(t, err)
		must(t, manager.Get(ctx, client.ObjectKeyFromObject(w), w))
		if !w.Status.Terminal || w.Status.RetainUntil == nil {
			t.Fatal("drain lost terminal observation")
		}
		r.Now = func() time.Time { return w.Status.RetainUntil.Add(time.Second) }
		_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(w)})
		must(t, err)
		if err := manager.Get(ctx, client.ObjectKeyFromObject(w), w); !apierrors.IsNotFound(err) {
			t.Fatal("expired retained order remained")
		}
		_, err = fr.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(f)})
		must(t, err)
		if err := manager.Get(ctx, client.ObjectKeyFromObject(f), f); !apierrors.IsNotFound(err) {
			t.Fatal("drained Fleet finalizer remained")
		}
	})
	for _, boundary := range []string{"fence", "observed", "terminal"} {
		t.Run("lost-status-response-"+boundary, func(t *testing.T) {
			f, hc, token := fixture(t, "status-"+boundary)
			w := intake(t, f, hc, token, "order-status")
			var calls atomic.Int32
			fault := &lostStatusClient{Client: manager, boundary: boundary}
			r := &controller.ClaudeWorkOrderReconciler{Client: fault, Scheme: scheme, AdmissionReady: func(context.Context) bool { return true }, CreatePod: func(ctx context.Context, p *corev1.Pod) error { calls.Add(1); return manager.Create(ctx, p) }}
			_, firstErr := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(w)})
			if boundary == "terminal" {
				must(t, firstErr)
				p := &corev1.Pod{}
				must(t, manager.Get(ctx, client.ObjectKey{Namespace: namespace, Name: w.Name}, p))
				p.Status.Phase = corev1.PodSucceeded
				must(t, manager.Status().Update(ctx, p))
				_, firstErr = r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(w)})
			}
			if firstErr == nil || !fault.lost {
				t.Fatal("status response fault not injected")
			}
			r.Client = manager // restart with a fresh client and persisted API state
			for i := 0; i < 3; i++ {
				_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(w)})
				must(t, err)
			}
			want := int32(1)
			if boundary == "fence" {
				want = 0
			}
			if calls.Load() != want {
				t.Fatalf("Pod POST count=%d, want %d", calls.Load(), want)
			}
			must(t, manager.Get(ctx, client.ObjectKeyFromObject(w), w))
			if !w.Status.LaunchAttempted {
				t.Fatal("persisted fence lost")
			}
			if boundary == "terminal" && !w.Status.Terminal {
				t.Fatal("terminal observation lost")
			}
		})
	}
	t.Run("terminal-cleanup-preserves-replay", func(t *testing.T) {
		f, hc, token := fixture(t, "retention")
		w := intake(t, f, hc, token, "order-retention")
		recorder := record.NewFakeRecorder(10)
		r := &controller.ClaudeWorkOrderReconciler{Client: manager, Scheme: scheme, Recorder: recorder, AdmissionReady: func(context.Context) bool { return true }, CreatePod: func(ctx context.Context, p *corev1.Pod) error { return manager.Create(ctx, p) }}
		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(w)})
		must(t, err)
		p := &corev1.Pod{}
		must(t, manager.Get(ctx, client.ObjectKey{Namespace: namespace, Name: w.Name}, p))
		p.Status.Phase = corev1.PodSucceeded
		must(t, manager.Status().Update(ctx, p))
		for i := 0; i < 2; i++ {
			_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(w)})
			must(t, err)
		}
		secret := &corev1.Secret{}
		if err := manager.Get(ctx, client.ObjectKey{Namespace: namespace, Name: w.Spec.CredentialSecretRef.Name}, secret); !apierrors.IsNotFound(err) {
			t.Fatal("terminal credential remains")
		}
		intake(t, f, hc, token, "order-retention")
		expiredReplay := hook.Run(ctx, hc, hook.Input{Namespace: namespace, Fleet: f.Name, Pool: f.Spec.EnvironmentID, Order: "order-retention"}, token, w.Spec.ExpiresAt.Add(10*time.Second))
		if expiredReplay.Code != hook.Submitted {
			t.Fatal("retained completed receipt did not acknowledge expired redelivery")
		}
		if manager.Get(ctx, client.ObjectKey{Namespace: namespace, Name: w.Spec.CredentialSecretRef.Name}, secret) == nil {
			t.Fatal("replay recreated credential")
		}
		must(t, manager.Get(ctx, client.ObjectKeyFromObject(w), w))
		if !w.Status.Terminal || w.Status.RetainUntil.Before(&w.Spec.ExpiresAt) {
			t.Fatal("retention floor missing")
		}
		select {
		case event := <-recorder.Events:
			if strings.Contains(event, string(token)) || !strings.Contains(event, "PodExitedZero") {
				t.Fatal("event is not a sanitized infrastructure observation")
			}
		default:
			t.Fatal("terminal event missing")
		}
		fr := &controller.ClaudeRunnerFleetReconciler{Client: manager, Scheme: scheme, AdmissionReady: func(context.Context) bool { return true }}
		_, err = fr.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(f)})
		must(t, err)
		must(t, manager.Get(ctx, client.ObjectKeyFromObject(f), f))
		if f.Status.Infrastructure.Terminal != 1 {
			t.Fatal("terminal count lost after Pod removal")
		}
	})
}
func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("API operation failed (%T): %v", err, err)
	}
}

type lostResponseClient struct {
	client.Client
	kind string
	lost bool
}

type mutatedReceiptClient struct {
	client.Client
	mutation string
}

type incompleteClient struct{ client.Client }

func (c *incompleteClient) Update(ctx context.Context, o client.Object, opts ...client.UpdateOption) error {
	if w, ok := o.(*api.ClaudeWorkOrder); ok && w.Spec.Complete {
		return context.DeadlineExceeded
	}
	return c.Client.Update(ctx, o, opts...)
}

type faultReader struct {
	client.Reader
	mu    sync.Mutex
	name  string
	armed bool
}

func (r *faultReader) arm(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.name = name
	r.armed = true
}
func (r *faultReader) Get(ctx context.Context, key client.ObjectKey, o client.Object, opts ...client.GetOption) error {
	r.mu.Lock()
	fail := r.armed && key.Name == r.name
	if fail {
		r.armed = false
	}
	r.mu.Unlock()
	if fail {
		return apierrors.NewServiceUnavailable("synthetic sensitive backend detail")
	}
	return r.Reader.Get(ctx, key, o, opts...)
}

func (c *mutatedReceiptClient) Create(ctx context.Context, o client.Object, opts ...client.CreateOption) error {
	if w, ok := o.(*api.ClaudeWorkOrder); ok {
		if c.mutation == "fleet-uid" {
			w.Spec.FleetUID = "synthetic-wrong-uid"
			w.OwnerReferences[0].UID = "synthetic-wrong-uid"
		}
		if c.mutation == "expiry" {
			w.Spec.ExpiresAt = metav1.NewTime(w.Spec.ExpiresAt.Add(5 * time.Minute))
		}
	}
	return c.Client.Create(ctx, o, opts...)
}

type startupRaceClient struct {
	client.Client
	boundary string
	reads    int
	started  bool
}

func (c *startupRaceClient) Get(ctx context.Context, key client.ObjectKey, o client.Object, opts ...client.GetOption) error {
	if err := c.Client.Get(ctx, key, o, opts...); err != nil {
		return err
	}
	if p, ok := o.(*corev1.Pod); ok {
		c.reads++
		if c.boundary == "fresh-read" && c.reads == 2 {
			p.Status.Phase = corev1.PodRunning
			c.started = true
			return c.Client.Status().Update(ctx, p)
		}
	}
	return nil
}
func (c *startupRaceClient) Delete(ctx context.Context, o client.Object, opts ...client.DeleteOption) error {
	if p, ok := o.(*corev1.Pod); ok && c.boundary == "delete-conflict" && !c.started {
		fresh := &corev1.Pod{}
		if err := c.Client.Get(ctx, client.ObjectKeyFromObject(p), fresh); err != nil {
			return err
		}
		fresh.Status.Phase = corev1.PodRunning
		if err := c.Client.Status().Update(ctx, fresh); err != nil {
			return err
		}
		c.started = true
	}
	return c.Client.Delete(ctx, o, opts...)
}

func (c *lostResponseClient) Update(ctx context.Context, o client.Object, opts ...client.UpdateOption) error {
	if err := c.Client.Update(ctx, o, opts...); err != nil {
		return err
	}
	if w, ok := o.(*api.ClaudeWorkOrder); ok && w.Spec.Complete && c.kind == "completion" && !c.lost {
		c.lost = true
		return errors.New("synthetic completion response loss")
	}
	return nil
}

type lostStatusClient struct {
	client.Client
	boundary string
	lost     bool
}

func (c *lostStatusClient) Status() client.SubResourceWriter {
	return &lostStatusWriter{SubResourceWriter: c.Client.Status(), parent: c}
}

type lostStatusWriter struct {
	client.SubResourceWriter
	parent *lostStatusClient
}

func (s *lostStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	if err := s.SubResourceWriter.Update(ctx, obj, opts...); err != nil {
		return err
	}
	w, ok := obj.(*api.ClaudeWorkOrder)
	if !ok || s.parent.lost {
		return nil
	}
	matched := s.parent.boundary == "fence" && w.Status.LaunchAttempted && !w.Status.PodObserved || s.parent.boundary == "observed" && w.Status.PodObserved || s.parent.boundary == "terminal" && w.Status.Terminal
	if matched {
		s.parent.lost = true
		return errors.New("synthetic status response loss")
	}
	return nil
}

func (c *lostResponseClient) Create(ctx context.Context, o client.Object, opts ...client.CreateOption) error {
	err := c.Client.Create(ctx, o, opts...)
	if err != nil {
		return err
	}
	_, secret := o.(*corev1.Secret)
	_, receipt := o.(*api.ClaudeWorkOrder)
	if !c.lost && ((c.kind == "secret" && secret) || (c.kind == "receipt" && receipt)) {
		c.lost = true
		return errors.New("synthetic response loss")
	}
	return nil
}
