// Copyright 2026 biwsantang. SPDX-License-Identifier: Apache-2.0
package kubeclient

import (
	"context"
	"io"
	corev1 "k8s.io/api/core/v1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestProjectedTokenAndSinglePOST(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	if os.WriteFile(path, []byte("synthetic-one"), 0600) != nil {
		t.Fatal("fixture write")
	}
	calls := 0
	seen := ""
	cfg := FreshCredentials(&rest.Config{Host: "https://example.invalid", BearerTokenFile: path, WrapTransport: func(http.RoundTripper) http.RoundTripper {
		return roundTripFunc(func(r *http.Request) (*http.Response, error) {
			calls++
			seen = r.Header.Get("Authorization")
			return &http.Response{StatusCode: 503, Header: http.Header{"Content-Type": []string{"application/json"}, "Retry-After": []string{"1"}}, Body: io.NopCloser(strings.NewReader(`{"kind":"Status","apiVersion":"v1","status":"Failure","code":503,"reason":"ServiceUnavailable"}`)), Request: r}, nil
		})
	}})
	cfg.GroupVersion = &corev1.SchemeGroupVersion
	cfg.APIPath = "/api"
	cfg.NegotiatedSerializer = clientgoscheme.Codecs.WithoutConversion()
	rc, err := rest.RESTClientFor(cfg)
	if err != nil {
		t.Fatal(err)
	}
	_ = rc.Post().Namespace("synthetic").Resource("pods").Body(&corev1.Pod{}).MaxRetries(0).Do(context.Background()).Error()
	if calls != 1 || seen != "Bearer synthetic-one" {
		t.Fatal("POST retried or initial projected token missed")
	}
	if os.WriteFile(path, []byte("synthetic-two"), 0600) != nil {
		t.Fatal("fixture rotation")
	}
	_ = rc.Post().Namespace("synthetic").Resource("pods").Body(&corev1.Pod{}).MaxRetries(0).Do(context.Background()).Error()
	if calls != 2 || seen != "Bearer synthetic-two" {
		t.Fatal("projected rotation not observed on next request")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal("fixture removal")
	}
	_ = rc.Post().Namespace("synthetic").Resource("pods").Body(&corev1.Pod{}).MaxRetries(0).Do(context.Background()).Error()
	if calls != 2 {
		t.Fatal("request sent after credential removal")
	}
}
