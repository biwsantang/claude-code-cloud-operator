// Copyright 2026 biwsantang. SPDX-License-Identifier: Apache-2.0
package hook

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	api "github.com/biwsantang/claude-code-cloud-operator/api/v1alpha1"
	"github.com/biwsantang/claude-code-cloud-operator/internal/contract"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"net/http"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
	"time"
)

const (
	Submitted = 0
	Retryable = 1
	Permanent = 2
)

type Input struct{ Namespace, Fleet, Pool, Order, ServerTime string }
type Result struct {
	Code   int
	Reason string
	Name   string
}

func fail(reason string, code int) Result { return Result{Code: code, Reason: reason} }

var ErrClockSkew = errors.New("server clock outside accepted skew")

func clockOffset(serverTime string, now time.Time) (int64, error) {
	if serverTime == "" {
		return 0, nil
	}
	server, err := http.ParseTime(serverTime)
	if err != nil {
		return 0, errors.New("malformed server time")
	}
	offset := now.Unix() - server.Unix()
	if offset < -3600 || offset > 3600 {
		return 0, ErrClockSkew
	}
	return offset, nil
}

func invalidCredential(err error) Result {
	if errors.Is(err, ErrClockSkew) {
		return fail("ClockSkewOutsideBudget", Retryable)
	}
	return fail("InvalidCredential", Permanent)
}
func Classify(err error) int {
	if err == nil {
		return Submitted
	}
	if apierrors.IsForbidden(err) || apierrors.IsInvalid(err) || apierrors.IsBadRequest(err) || apierrors.IsUnauthorized(err) || apierrors.IsNotFound(err) {
		return Permanent
	}
	return Retryable
}

// Expiry extracts bounded lifetime bookkeeping only. Native registration verifies the signature.
func Expiry(token []byte, serverTime string, now time.Time, maxLifetime int64) (time.Time, error) {
	token = []byte(strings.TrimSpace(string(token)))
	parts := strings.Split(string(token), ".")
	if len(token) > 32768 || len(parts) != 3 || len(parts[2]) == 0 {
		return time.Time{}, errors.New("malformed credential")
	}
	b, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, errors.New("malformed credential")
	}
	var claims struct {
		Exp json.Number `json:"exp"`
	}
	if err := json.Unmarshal(b, &claims); err != nil {
		return time.Time{}, errors.New("malformed expiry")
	}
	n, err := claims.Exp.Int64()
	if err != nil {
		return time.Time{}, errors.New("malformed expiry")
	}
	expiry := time.Unix(n, 0).UTC()
	reference := now
	if serverTime != "" {
		t, err := http.ParseTime(serverTime)
		if err != nil {
			return time.Time{}, errors.New("malformed server time")
		}
		if _, err := clockOffset(serverTime, now); err != nil {
			return time.Time{}, err
		}
		reference = t
	}
	if !expiry.After(reference) || expiry.After(reference.Add(time.Duration(maxLifetime)*time.Second)) {
		return time.Time{}, errors.New("credential lifetime outside bounds")
	}
	return expiry, nil
}

// Run uses an uncached API client. It never reads Secrets; completion admission checks collisions.
func Run(ctx context.Context, c client.Client, in Input, token []byte, now time.Time) Result {
	if in.Namespace == "" || in.Fleet == "" || in.Pool == "" || in.Order == "" {
		return fail("MissingInput", Permanent)
	}
	token = []byte(strings.TrimSpace(string(token)))
	f := &api.ClaudeRunnerFleet{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: in.Namespace, Name: in.Fleet}, f); err != nil {
		return fail("FleetUnavailable", Classify(err))
	}
	if in.Pool != f.Spec.EnvironmentID {
		return fail("PoolMismatch", Permanent)
	}
	name := contract.Name(in.Pool, in.Order)
	w := &api.ClaudeWorkOrder{}
	key := client.ObjectKey{Namespace: in.Namespace, Name: name}
	err := c.Get(ctx, key, w)
	if err != nil && !apierrors.IsNotFound(err) {
		return fail("ReceiptUnavailable", Classify(err))
	}
	if err == nil {
		// Match the original policy, not the latest Fleet revision. A completed tombstone never repairs a Secret.
		if w.Spec.FleetUID != string(f.UID) || w.Spec.FleetName != in.Fleet || w.Spec.PoolID != in.Pool || w.Spec.OrderID != in.Order || w.Spec.TokenDigest != contract.Hash(token) || !contract.Owns(f, w, "ClaudeRunnerFleet") || !w.DeletionTimestamp.IsZero() {
			return fail("ReceiptMismatch", Permanent)
		}
		if w.Spec.Complete {
			return Result{Code: Submitted, Reason: "Redelivered", Name: name}
		}
		// A retry whose gateway omits Date still uses the first receipt's frozen time basis.
		reference := now
		if in.ServerTime == "" {
			reference = now.Add(-time.Duration(w.Spec.ClockOffsetSeconds) * time.Second)
		}
		expiry, err := Expiry(token, in.ServerTime, reference, w.Spec.Execution.MaxTokenLifetimeSeconds)
		if err != nil {
			return invalidCredential(err)
		}
		if !w.Spec.ExpiresAt.Time.Equal(expiry) {
			return fail("ReceiptMismatch", Permanent)
		}
	} else {
		expiry, err := Expiry(token, in.ServerTime, now, f.Spec.Execution.MaxTokenLifetimeSeconds)
		if err != nil {
			return invalidCredential(err)
		}
		offset, err := clockOffset(in.ServerTime, now)
		if err != nil {
			return invalidCredential(err)
		}
		if f.Spec.Suspended || !f.DeletionTimestamp.IsZero() {
			return fail("FleetSuspended", Retryable)
		}
		w = &api.ClaudeWorkOrder{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: in.Namespace, Finalizers: []string{contract.OrderFinalizer}, OwnerReferences: []metav1.OwnerReference{contract.Owner(f, "ClaudeRunnerFleet")}}, Spec: api.ClaudeWorkOrderSpec{FleetName: f.Name, FleetUID: string(f.UID), PoolID: in.Pool, OrderID: in.Order, TokenDigest: contract.Hash(token), ExpiresAt: metav1.NewTime(expiry), ClockOffsetSeconds: offset, CredentialSecretRef: api.LocalReference{Name: name + "-credential"}, PolicyDigest: contract.PolicyDigest(f.Spec.Execution), Execution: *f.Spec.Execution.DeepCopy()}}
		if err := contract.ValidateOrder(w); err != nil {
			return fail("InvalidReceipt", Permanent)
		}
		if err := c.Create(ctx, w); err != nil {
			if apierrors.IsAlreadyExists(err) {
				return Run(ctx, c, in, token, now)
			}
			return fail("ReceiptWriteFailed", Classify(err))
		}
	}
	immutable := true
	s := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: w.Spec.CredentialSecretRef.Name, Namespace: w.Namespace, OwnerReferences: []metav1.OwnerReference{contract.Owner(w, "ClaudeWorkOrder")}}, Immutable: &immutable, Type: corev1.SecretTypeOpaque, Data: map[string][]byte{contract.CredentialKey: token}}
	if err := c.Create(ctx, s); err != nil && !apierrors.IsAlreadyExists(err) {
		return recoverCompletedReceipt(ctx, c, f, w, err, "CredentialWriteFailed")
	}
	w.Spec.Complete = true
	// Admission reads the Secret uncached and checks owner, immutability and digest before completion.
	// Controller status writes can race intake. Retry only receipt conflicts, never a Pod submission.
	redelivered := false
	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		latest := &api.ClaudeWorkOrder{}
		if err := c.Get(ctx, key, latest); err != nil {
			return err
		}
		if latest.UID != w.UID || !latest.DeletionTimestamp.IsZero() || !contract.Owns(f, latest, "ClaudeRunnerFleet") || !contract.SameReceipt(w.Spec, latest.Spec) {
			return apierrors.NewForbidden(api.GroupVersion.WithResource("claudeworkorders").GroupResource(), w.Name, errors.New("receipt identity changed"))
		}
		if latest.Spec.Complete {
			redelivered = true
			return nil
		}
		if latest.Status.Terminal {
			return apierrors.NewForbidden(api.GroupVersion.WithResource("claudeworkorders").GroupResource(), w.Name, errors.New("receipt is terminal"))
		}
		latest.Spec.Complete = true
		return c.Update(ctx, latest)
	}); err != nil {
		return recoverCompletedReceipt(ctx, c, f, w, err, "ReceiptCompletionFailed")
	}
	if redelivered {
		return Result{Code: Submitted, Reason: "Redelivered", Name: name}
	}
	return Result{Code: Submitted, Reason: "Accepted", Name: name}
}

// Admission can observe a completed receipt before a concurrent caller's
// credential create or completion update reaches storage. A matching durable
// completion is sufficient to acknowledge delivery; never repair its Secret.
func recoverCompletedReceipt(ctx context.Context, c client.Client, f *api.ClaudeRunnerFleet, w *api.ClaudeWorkOrder, cause error, reason string) Result {
	latest := &api.ClaudeWorkOrder{}
	if err := c.Get(ctx, client.ObjectKeyFromObject(w), latest); err != nil {
		if !apierrors.IsNotFound(err) {
			return fail("ReceiptUnavailable", Classify(err))
		}
	} else if latest.UID == w.UID && latest.Spec.Complete && latest.DeletionTimestamp.IsZero() && contract.Owns(f, latest, "ClaudeRunnerFleet") && contract.SameReceipt(w.Spec, latest.Spec) {
		return Result{Code: Submitted, Reason: "Redelivered", Name: w.Name}
	}
	return fail(reason, Classify(cause))
}
