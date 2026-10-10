// Copyright 2026 biwsantang. SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"crypto/tls"
	"flag"
	api "github.com/biwsantang/claude-code-cloud-operator/api/v1alpha1"
	"github.com/biwsantang/claude-code-cloud-operator/internal/admission"
	"github.com/biwsantang/claude-code-cloud-operator/internal/contract"
	"github.com/biwsantang/claude-code-cloud-operator/internal/controller"
	"github.com/biwsantang/claude-code-cloud-operator/internal/kubeclient"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"os"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	webadmission "sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"strings"
)

func main() {
	var certDir, probe, account, admissionName, metricsAddress string
	var leader bool
	flag.StringVar(&metricsAddress, "metrics-bind-address", "0", "Optional metrics address; require network isolation when enabling")
	flag.StringVar(&certDir, "webhook-cert-path", "/tmp/k8s-webhook-server/serving-certs", "Mounted admission TLS directory")
	flag.StringVar(&probe, "health-probe-bind-address", ":8081", "Health address")
	flag.StringVar(&account, "manager-account", "cloud-operator-controller-manager", "Manager service account")
	flag.StringVar(&admissionName, "admission-name", "cloud-operator-admission", "Installed admission configuration name")
	flag.BoolVar(&leader, "leader-elect", true, "Elect one active manager")
	opts := zap.Options{Development: false}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	namespace := os.Getenv("WATCH_NAMESPACE")
	if namespace == "" || strings.Contains(namespace, ",") {
		ctrl.Log.Info("Exactly one WATCH_NAMESPACE is required")
		os.Exit(1)
	}
	defaults, err := contract.DecodeDefaults(os.Getenv("FLEET_DEFAULTS"))
	if err == nil && defaults.HookImage == "" {
		defaults.HookImage = os.Getenv("OPERATOR_HOOK_IMAGE")
		err = defaults.Validate()
	}
	if err != nil {
		ctrl.Log.Info("Installation defaults are invalid")
		os.Exit(1)
	}
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = api.AddToScheme(scheme)
	cfg := kubeclient.FreshCredentials(ctrl.GetConfigOrDie())
	// The transaction client is uncached. Secrets are never requested from manager informers.
	direct, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		os.Exit(1)
	}
	server := webhook.NewServer(webhook.Options{Port: 9443, CertDir: certDir, TLSOpts: []func(*tls.Config){func(c *tls.Config) { c.MinVersion = tls.VersionTLS12; c.NextProtos = []string{"http/1.1"} }}})
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{Scheme: scheme, Cache: cache.Options{DefaultNamespaces: map[string]cache.Config{namespace: {}}, ReaderFailOnMissingInformer: true}, Client: client.Options{Cache: &client.CacheOptions{DisableFor: []client.Object{&corev1.Secret{}}}}, WebhookServer: server, Metrics: metricsserver.Options{BindAddress: metricsAddress}, HealthProbeBindAddress: probe, LeaderElection: leader, LeaderElectionNamespace: namespace, LeaderElectionID: "claude-code-cloud-operator"})
	if err != nil {
		ctrl.Log.Info("Manager initialization failed")
		os.Exit(1)
	}
	handler := &admission.Handler{Defaults: defaults, Reader: direct, Namespace: namespace, ManagerAccount: account}
	mgr.GetWebhookServer().Register("/validate", &webadmission.Webhook{Handler: handler})
	ready := func(ctx context.Context) bool {
		if server.StartedChecker()(nil) != nil {
			return false
		}
		cfg := &admissionv1.ValidatingWebhookConfiguration{}
		if direct.Get(ctx, client.ObjectKey{Name: admissionName}, cfg) != nil || len(cfg.Webhooks) != 2 {
			return false
		}
		return admission.ConfigurationMatches(cfg, namespace)
	}
	post, err := controller.SinglePost(cfg)
	if err != nil {
		os.Exit(1)
	}
	if err := (&controller.ClaudeRunnerFleetReconciler{Defaults: defaults, Client: direct, Scheme: scheme, AdmissionReady: ready}).SetupWithManager(mgr); err != nil {
		os.Exit(1)
	}
	if err := (&controller.ClaudeWorkOrderReconciler{Defaults: defaults, Client: direct, Scheme: scheme, CreatePod: post, AdmissionReady: ready, Recorder: mgr.GetEventRecorderFor("claude-work-order")}).SetupWithManager(mgr); err != nil {
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder
	_ = mgr.AddHealthzCheck("healthz", healthz.Ping)
	_ = mgr.AddReadyzCheck("admission", server.StartedChecker())
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		ctrl.Log.Info("Manager stopped with an error")
		os.Exit(1)
	}
}
