// Copyright 2026 biwsantang. SPDX-License-Identifier: Apache-2.0
package kubeclient

import (
	"errors"
	"k8s.io/client-go/rest"
	"net/http"
	"os"
	"strings"
)

type rotatingToken struct {
	next http.RoundTripper
	path string
}

func (t rotatingToken) RoundTrip(req *http.Request) (*http.Response, error) {
	b, err := os.ReadFile(t.path)
	if err != nil || len(b) == 0 || len(b) > 1048576 {
		return nil, errors.New("projected credential unavailable")
	}
	clone := req.Clone(req.Context())
	clone.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(b)))
	return t.next.RoundTrip(clone)
}

// FreshCredentials reads projected tokens for every request, including lease and status writes.
func FreshCredentials(cfg *rest.Config) *rest.Config {
	c := rest.CopyConfig(cfg)
	if c.BearerTokenFile == "" {
		return c
	}
	path := c.BearerTokenFile
	previous := c.WrapTransport
	c.BearerToken = ""
	c.BearerTokenFile = ""
	c.WrapTransport = func(next http.RoundTripper) http.RoundTripper {
		if previous != nil {
			next = previous(next)
		}
		return rotatingToken{next: next, path: path}
	}
	return c
}
