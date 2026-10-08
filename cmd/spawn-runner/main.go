// Copyright 2026 biwsantang. SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"encoding/json"
	"fmt"
	api "github.com/biwsantang/claude-code-cloud-operator/api/v1alpha1"
	"github.com/biwsantang/claude-code-cloud-operator/internal/hook"
	"github.com/biwsantang/claude-code-cloud-operator/internal/kubeclient"
	"io"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"net/http"
	"os"
	"path/filepath"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "install" {
		install()
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "probe" {
		probe()
		return
	}
	in := hook.Input{Namespace: os.Getenv("OPERATOR_NAMESPACE"), Fleet: os.Getenv("OPERATOR_FLEET"), Pool: os.Getenv("CLAUDE_RUNNER_POOL_ID"), Order: os.Getenv("CLAUDE_RUNNER_ORDER_ID"), ServerTime: os.Getenv("CLAUDE_RUNNER_ORDER_SERVER_TIME")}
	f, err := os.Open(os.Getenv("CLAUDE_RUNNER_WORK_ORDER_FILE"))
	if err != nil {
		exit(hook.Result{Code: hook.Permanent, Reason: "CredentialFileUnavailable"})
	}
	defer f.Close()
	token, err := io.ReadAll(io.LimitReader(f, 32769))
	if err != nil || len(token) > 32768 {
		exit(hook.Result{Code: hook.Permanent, Reason: "CredentialFileInvalid"})
	}
	cfg, err := rest.InClusterConfig()
	if err != nil {
		exit(hook.Result{Code: hook.Permanent, Reason: "ClusterConfigurationUnavailable"})
	}
	cfg = kubeclient.FreshCredentials(cfg)
	cfg.Timeout = 10 * time.Second
	scheme := runtime.NewScheme()
	_ = api.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		exit(hook.Result{Code: hook.Retryable, Reason: "ClientUnavailable"})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	exit(hook.Run(ctx, c, in, token, time.Now()))
}
func exit(r hook.Result) { fmt.Fprintln(os.Stderr, r.Reason); os.Exit(r.Code) }
func install() {
	if len(os.Args) != 3 {
		os.Exit(2)
	}
	dest := filepath.Join(os.Args[2], "spawn-runner")
	src, err := os.Executable()
	if err != nil {
		os.Exit(1)
	}
	b, err := os.ReadFile(src)
	if err != nil {
		os.Exit(1)
	}
	if err := os.WriteFile(dest, b, 0555); err != nil {
		os.Exit(1)
	}
}

// Probe is local and bounded. It exposes only native connected, never the native error body.
func probe() {
	if !nativeConnected(nativeProbeClient()) {
		os.Exit(1)
	}
}

func nativeProbeClient() *http.Client {
	return &http.Client{Timeout: 2 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
}

func nativeConnected(c *http.Client) bool {
	resp, err := c.Get("http://127.0.0.1:8080/healthz")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return false
	}
	// Read the extra byte to distinguish a bounded body from a valid JSON prefix
	// followed by an oversized response. Unmarshal rejects trailing JSON too.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4097))
	if err != nil || len(body) > 4096 {
		return false
	}
	var b struct {
		Connected bool `json:"connected"`
	}
	return json.Unmarshal(body, &b) == nil && b.Connected
}
