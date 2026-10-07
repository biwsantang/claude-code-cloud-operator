// Copyright 2026 biwsantang. SPDX-License-Identifier: Apache-2.0
package main

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type probeTransport func(*http.Request) (*http.Response, error)

func (f probeTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type trackedBody struct {
	io.Reader
	closed bool
}

func (b *trackedBody) Close() error { b.closed = true; return nil }

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("synthetic read failure") }

func TestNativeConnectionProbe(t *testing.T) {
	valid := `{"connected":true}`
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{"connected", 200, valid, true},
		{"native additional fields", 200, `{"connected":true,"queue_count":3}`, true},
		{"disconnected", 200, `{"connected":false}`, false},
		{"http success without connection", 200, `{}`, false},
		{"null connection", 200, `{"connected":null}`, false},
		{"wrong connection type", 200, `{"connected":"true"}`, false},
		{"malformed", 200, `{"connected":true`, false},
		{"empty", 200, "", false},
		{"trailing object", 200, valid + `{}`, false},
		{"trailing native error", 200, valid + " synthetic native error", false},
		{"exact response bound", 200, valid + strings.Repeat(" ", 4096-len(valid)), true},
		{"response above bound", 200, valid + strings.Repeat(" ", 4097-len(valid)), false},
		{"revoked response", 401, valid, false},
		{"unavailable response", 503, valid, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := &trackedBody{Reader: strings.NewReader(tc.body)}
			c := &http.Client{Transport: probeTransport(func(r *http.Request) (*http.Response, error) {
				if r.Method != http.MethodGet || r.URL.String() != "http://127.0.0.1:8080/healthz" {
					t.Fatal("probe must only request the local native health endpoint")
				}
				return &http.Response{StatusCode: tc.status, Body: body, Header: make(http.Header)}, nil
			})}
			if got := nativeConnected(c); got != tc.want {
				t.Fatalf("connected = %v, want %v", got, tc.want)
			}
			if !body.closed {
				t.Fatal("response body was not closed")
			}
		})
	}
}

func TestNativeConnectionProbeTransportAndReadFailure(t *testing.T) {
	c := &http.Client{Transport: probeTransport(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("synthetic connection failure")
	})}
	if nativeConnected(c) {
		t.Fatal("transport failure reported connected")
	}
	body := &trackedBody{Reader: failingReader{}}
	c.Transport = probeTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: body, Header: make(http.Header)}, nil
	})
	if nativeConnected(c) || !body.closed {
		t.Fatal("read failure must fail closed and close the response")
	}
}

func TestNativeConnectionProbeDoesNotFollowRedirects(t *testing.T) {
	c := nativeProbeClient()
	if c.Timeout != 2*time.Second {
		t.Fatal("probe must have the bounded native-health timeout")
	}
	calls := 0
	body := &trackedBody{Reader: strings.NewReader(`{"connected":true}`)}
	c.Transport = probeTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls > 1 {
			t.Fatal("local health probe followed a redirect")
		}
		return &http.Response{StatusCode: 302, Body: body, Header: http.Header{"Location": {"https://synthetic.invalid/healthz"}}}, nil
	})
	if nativeConnected(c) || calls != 1 || !body.closed {
		t.Fatal("redirect must fail closed locally and close the response")
	}
}
