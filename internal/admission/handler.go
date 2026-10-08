// Copyright 2026 biwsantang. SPDX-License-Identifier: Apache-2.0
package admission

import (
	"context"
	"encoding/json"
	"errors"
	api "github.com/biwsantang/claude-code-cloud-operator/api/v1alpha1"
	"github.com/biwsantang/claude-code-cloud-operator/internal/contract"
	"github.com/biwsantang/claude-code-cloud-operator/internal/controller"
	"github.com/biwsantang/claude-code-cloud-operator/internal/hook"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	webhook "sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"strings"
	"time"
)

type Handler struct {
	Reader                    client.Reader
	Namespace, ManagerAccount string
	Now                       func() time.Time
}

func (h *Handler) now() time.Time {
	if h.Now != nil {
		return h.Now().UTC()
	}
	return time.Now().UTC()
}
func deny(reason string) webhook.Response { return webhook.Denied(reason) }
func readFailure(err error, reason string) webhook.Response {
	if !apierrors.IsNotFound(err) {
		return webhook.Errored(503, errors.New("Admission dependency temporarily unavailable"))
	}
	return deny(reason)
}
func prerequisiteFailure(err error, reason string) webhook.Response {
	if errors.Is(err, contract.ErrPrerequisiteAPI) || errors.Is(err, contract.ErrIntakePaused) {
		return webhook.Errored(503, errors.New("Intake prerequisites temporarily unavailable"))
	}
	return deny(reason)
}
func (h *Handler) Handle(ctx context.Context, req webhook.Request) webhook.Response {
	if req.Namespace != h.Namespace {
		return deny("Namespace is outside this operator trust boundary")
	}
	manager := req.UserInfo.Username == contract.ManagerUser(h.Namespace, h.ManagerAccount)
	hookActor := strings.HasPrefix(req.UserInfo.Username, "system:serviceaccount:"+h.Namespace+":") && strings.HasSuffix(req.UserInfo.Username, "-hook")
	switch req.Kind.Kind {
	case "ClaudeRunnerFleet":
		if hookActor {
			return deny("Hook cannot change Fleets")
		}
		if req.Operation == admissionv1.Delete {
			return webhook.Allowed("Fleet drain is controlled by finalizers")
		}
		f := &api.ClaudeRunnerFleet{}
		if json.Unmarshal(req.Object.Raw, f) != nil {
			return deny("Malformed Fleet")
		}
		if err := contract.ValidateFleet(f); err != nil {
			return deny(err.Error())
		}
		if req.SubResource == "status" {
			if !manager {
				return deny("Only manager may write Fleet status")
			}
			return webhook.Allowed("Manager status")
		}
		if !f.Spec.Suspended && f.DeletionTimestamp.IsZero() && !controllerutil.ContainsFinalizer(f, contract.FleetFinalizer) {
			return deny("Activation requires the managed Fleet finalizer")
		}
		if req.Operation == admissionv1.Create && (!f.Spec.Suspended || len(f.Status.Conditions) != 0) {
			return deny("New Fleets must be suspended and have no status")
		}
		if req.Operation == admissionv1.Update {
			old := &api.ClaudeRunnerFleet{}
			if json.Unmarshal(req.OldObject.Raw, old) != nil {
				return deny("Malformed prior Fleet")
			}
			if old.Spec.EnvironmentID != f.Spec.EnvironmentID {
				return deny("Environment identity is immutable")
			}
			if !manager && !reflect.DeepEqual(old.Finalizers, f.Finalizers) {
				return deny("Only manager controls drain finalizer")
			}
			if !f.Spec.Suspended && !manager {
				if err := contract.InputsReady(ctx, h.Reader, f, h.now()); err != nil {
					return prerequisiteFailure(err, "Activation prerequisites are missing")
				}
			}
		}
		return webhook.Allowed("Fleet validated")
	case "ClaudeWorkOrder":
		w := &api.ClaudeWorkOrder{}
		raw := req.Object.Raw
		if req.Operation == admissionv1.Delete {
			raw = req.OldObject.Raw
		}
		if json.Unmarshal(raw, w) != nil {
			return deny("Malformed receipt")
		}
		if req.Operation == admissionv1.Delete {
			if !manager {
				return deny("Only manager can delete retained receipts")
			}
			return webhook.Allowed("Manager cleanup")
		}
		if err := contract.ValidateOrder(w); err != nil {
			return deny(err.Error())
		}
		if req.SubResource == "status" {
			if !manager {
				return deny("Only manager may write receipt status")
			}
			old := &api.ClaudeWorkOrder{}
			if json.Unmarshal(req.OldObject.Raw, old) != nil {
				return deny("Malformed prior receipt")
			}
			if old.Status.LaunchAttempted && !w.Status.LaunchAttempted || old.Status.PodObserved && !w.Status.PodObserved || old.Status.PodUID != "" && old.Status.PodUID != w.Status.PodUID || old.Status.Terminal && !w.Status.Terminal {
				return deny("Submission observations are monotonic")
			}
			return webhook.Allowed("Manager observations")
		}
		if req.Operation == admissionv1.Update {
			old := &api.ClaudeWorkOrder{}
			if json.Unmarshal(req.OldObject.Raw, old) != nil {
				return deny("Malformed prior receipt")
			}
			if !contract.SameReceipt(old.Spec, w.Spec) || old.Spec.Complete && !w.Spec.Complete {
				return deny("Receipt identity and snapshot are immutable")
			}
			if manager {
				if !reflect.DeepEqual(old.Spec, w.Spec) {
					return deny("Manager cannot change accepted intent")
				}
				return webhook.Allowed("Manager finalizer")
			}
			if !reflect.DeepEqual(old.Finalizers, w.Finalizers) || !reflect.DeepEqual(old.OwnerReferences, w.OwnerReferences) || !reflect.DeepEqual(old.Labels, w.Labels) || !reflect.DeepEqual(old.Annotations, w.Annotations) {
				return deny("Hook cannot change receipt metadata")
			}
			if old.Spec.Complete {
				if req.UserInfo.Username != contract.ManagerUser(w.Namespace, contract.HookAccount(w.Spec.FleetName)) {
					return deny("Receipt actor does not match Fleet hook identity")
				}
				return webhook.Allowed("Immutable redelivery")
			}
		} else if req.Operation == admissionv1.Create {
			if w.Spec.Complete || !reflect.DeepEqual(w.Status, api.ClaudeWorkOrderStatus{}) || len(w.Finalizers) != 1 || w.Finalizers[0] != contract.OrderFinalizer || len(w.Annotations) != 0 || len(w.Labels) != 0 {
				return deny("New receipt must be incomplete, retained and sanitized")
			}
		}
		if req.UserInfo.Username != contract.ManagerUser(w.Namespace, contract.HookAccount(w.Spec.FleetName)) {
			return deny("Receipt actor does not match Fleet hook identity")
		}
		f := &api.ClaudeRunnerFleet{}
		if err := h.Reader.Get(ctx, client.ObjectKey{Namespace: w.Namespace, Name: w.Spec.FleetName}, f); err != nil {
			return readFailure(err, "Fleet unavailable")
		}
		if string(f.UID) != w.Spec.FleetUID || !contract.Owns(f, w, "ClaudeRunnerFleet") || w.Spec.PoolID != f.Spec.EnvironmentID || !reflect.DeepEqual(w.Spec.Execution, f.Spec.Execution) {
			return deny("Fleet identity or policy mismatch")
		}
		if err := contract.InputsReady(ctx, h.Reader, f, h.now()); err != nil {
			return prerequisiteFailure(err, "Fleet intake prerequisites are missing")
		}
		deadline := contract.OrderExpiry(w)
		if !deadline.After(h.now()) || deadline.After(h.now().Add(time.Duration(w.Spec.Execution.MaxTokenLifetimeSeconds)*time.Second)) {
			return deny("Receipt expiry is outside bounds")
		}
		if w.Spec.Complete {
			s := &corev1.Secret{}
			if err := h.Reader.Get(ctx, client.ObjectKey{Namespace: w.Namespace, Name: w.Spec.CredentialSecretRef.Name}, s); err != nil {
				return readFailure(err, "Credential receipt is unavailable")
			}
			if !controller.CredentialMatches(w, s) {
				return deny("Credential receipt is incomplete or mismatched")
			}
			serverNow := h.now().Add(-time.Duration(w.Spec.ClockOffsetSeconds) * time.Second)
			expiry, err := hook.Expiry(s.Data[contract.CredentialKey], "", serverNow, w.Spec.Execution.MaxTokenLifetimeSeconds)
			if err != nil || !expiry.Equal(w.Spec.ExpiresAt.Time) {
				return deny("Credential expiry does not match the receipt")
			}
		}
		return webhook.Allowed("Durable intake validated")
	case "Secret":
		// Trusted namespace administrators/ESO manage other Secrets. Hook may create exactly its credential.
		if !hookActor {
			return webhook.Allowed("Trusted Secret administrator")
		}
		if req.Operation != admissionv1.Create {
			return deny("Hook may only create an immutable credential")
		}
		s := &corev1.Secret{}
		if json.Unmarshal(req.Object.Raw, s) != nil {
			return deny("Malformed credential object")
		}
		owner := metav1.GetControllerOf(s)
		if owner == nil || owner.Kind != "ClaudeWorkOrder" || owner.APIVersion != api.GroupVersion.String() {
			return deny("Credential requires a receipt owner")
		}
		w := &api.ClaudeWorkOrder{}
		if err := h.Reader.Get(ctx, client.ObjectKey{Namespace: s.Namespace, Name: owner.Name}, w); err != nil {
			return readFailure(err, "Receipt unavailable")
		}
		if req.UserInfo.Username != contract.ManagerUser(w.Namespace, contract.HookAccount(w.Spec.FleetName)) || !w.DeletionTimestamp.IsZero() || w.Spec.Complete || w.Status.Terminal || !controller.CredentialMatches(w, s) {
			return deny("Credential owner, digest or state mismatch")
		}
		return webhook.Allowed("Immutable credential validated")
	default:
		return deny("Unsupported resource")
	}
}
