// Copyright 2026 biwsantang. SPDX-License-Identifier: Apache-2.0
package hook

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"testing"
	"time"
)

func TestExpiry(t *testing.T) {
	now := time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC)
	token := func(n int64) []byte {
		return []byte("synthetic." + base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"exp":%d}`, n))) + ".not-a-signature")
	}
	for _, tc := range []struct {
		name  string
		b     []byte
		valid bool
	}{{"bounded", token(now.Add(time.Hour).Unix()), true}, {"expired", token(now.Add(-time.Hour).Unix()), false}, {"excessive", token(now.Add(time.Hour * 25).Unix()), false}, {"malformed", []byte("redacted"), false}} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Expiry(tc.b, "", now, 86400)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
		})
	}
}

func TestFailureClassification(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		code int
	}{
		{"success", nil, Submitted},
		{"authorization", apierrors.NewForbidden(schema.GroupResource{Resource: "secrets"}, "synthetic", errors.New("synthetic private value")), Permanent},
		{"missing", apierrors.NewNotFound(schema.GroupResource{Resource: "fleets"}, "synthetic"), Permanent},
		{"conflict", apierrors.NewConflict(schema.GroupResource{Resource: "orders"}, "synthetic", errors.New("conflict")), Retryable},
		{"timeout", context.DeadlineExceeded, Retryable},
		{"transport", errors.New("synthetic private value"), Retryable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if Classify(tc.err) != tc.code {
				t.Fatal("incorrect hook exit classification")
			}
		})
	}
	result := Run(context.Background(), nil, Input{}, []byte("synthetic private value"), time.Now())
	if result.Code != Permanent || result.Reason != "MissingInput" {
		t.Fatal("missing input not sanitized")
	}
}
