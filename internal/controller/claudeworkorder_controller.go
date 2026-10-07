// Copyright 2026 biwsantang. SPDX-License-Identifier: Apache-2.0
package controller

import (
	"context"
	api "github.com/biwsantang/claude-code-cloud-operator/api/v1alpha1"
	"github.com/biwsantang/claude-code-cloud-operator/internal/builders"
	"github.com/biwsantang/claude-code-cloud-operator/internal/contract"
	"github.com/biwsantang/claude-code-cloud-operator/internal/kubeclient"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"time"
)

// Client must be uncached: status conflicts fence concurrent controller invocations.
type ClaudeWorkOrderReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	CreatePod      func(context.Context, *corev1.Pod) error
	AdmissionReady func(context.Context) bool
	Now            func() time.Time
	Recorder       record.EventRecorder
	// AfterFence is a fault injection boundary; production leaves it nil.
	AfterFence func() error
}

func SinglePost(cfg *rest.Config) (func(context.Context, *corev1.Pod) error, error) {
	c := kubeclient.FreshCredentials(cfg)
	c.GroupVersion = &corev1.SchemeGroupVersion
	c.APIPath = "/api"
	c.NegotiatedSerializer = clientgoscheme.Codecs.WithoutConversion()
	c.Timeout = 10 * time.Second
	rc, err := rest.RESTClientFor(c)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, p *corev1.Pod) error {
		return rc.Post().Namespace(p.Namespace).Resource("pods").Body(p).MaxRetries(0).Do(ctx).Into(p)
	}, nil
}
func (r *ClaudeWorkOrderReconciler) now() time.Time {
	if r.Now != nil {
		return r.Now().UTC()
	}
	return time.Now().UTC()
}

// +kubebuilder:rbac:groups=runners.biwsantang.github.io,resources=claudeworkorders;clauderunnerfleets,verbs=get;list;watch;update;patch;delete,namespace=cloud-operator-system
// +kubebuilder:rbac:groups=runners.biwsantang.github.io,resources=claudeworkorders/status;clauderunnerfleets/status,verbs=get;update;patch,namespace=cloud-operator-system
// +kubebuilder:rbac:groups=runners.biwsantang.github.io,resources=claudeworkorders/finalizers;clauderunnerfleets/finalizers,verbs=update,namespace=cloud-operator-system
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;delete,namespace=cloud-operator-system
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;delete,namespace=cloud-operator-system

// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch,namespace=cloud-operator-system

// Reconcile converges this resource and its owned state.
func (r *ClaudeWorkOrderReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	w := &api.ClaudeWorkOrder{}
	if err := r.Get(ctx, req.NamespacedName, w); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	w.Status.Accepted = w.Status.Accepted || w.Spec.Complete
	now := r.now()
	f := &api.ClaudeRunnerFleet{}
	err := r.Get(ctx, client.ObjectKey{Namespace: w.Namespace, Name: w.Spec.FleetName}, f)
	if err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}
	if err == nil && !contract.Owns(f, w, "ClaudeRunnerFleet") {
		return r.save(ctx, w, "Invalid", "FleetOwnershipMismatch", true)
	}
	if !controllerutil.ContainsFinalizer(w, contract.OrderFinalizer) && w.DeletionTimestamp.IsZero() {
		controllerutil.AddFinalizer(w, contract.OrderFinalizer)
		return ctrl.Result{RequeueAfter: time.Millisecond}, r.Update(ctx, w)
	}
	if !w.DeletionTimestamp.IsZero() {
		return r.cleanup(ctx, w, now, true)
	}
	if w.Status.Terminal {
		return r.cleanup(ctx, w, now, false)
	}
	if w.Status.LaunchAttempted {
		return r.observe(ctx, w, now)
	}
	if !contract.OrderExpiry(w).After(now) {
		return r.save(ctx, w, "Expired", "CredentialExpiredBeforeLaunch", true)
	}
	if !w.Spec.Complete {
		return r.save(ctx, w, "Pending", "IntakeIncomplete", false)
	}
	if err := contract.ValidateOrder(w); err != nil {
		return r.save(ctx, w, "Invalid", "InvalidReceipt", true)
	}
	if f.UID == "" || string(f.UID) != w.Spec.FleetUID {
		return r.save(ctx, w, "Invalid", "FleetUnavailable", true)
	}
	if r.AdmissionReady == nil || !r.AdmissionReady(ctx) {
		return r.save(ctx, w, "Accepted", "AdmissionUnavailable", false)
	}
	if err := contract.InputsReady(ctx, r.Client, f, now); err != nil {
		return r.save(ctx, w, "Accepted", "PrerequisitesUnavailable", false)
	}
	if w.Spec.PolicyDigest != contract.PolicyDigest(f.Spec.Execution) {
		return r.save(ctx, w, "Expired", "PolicyChangedBeforeLaunch", true)
	}
	secret := &corev1.Secret{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: w.Namespace, Name: w.Spec.CredentialSecretRef.Name}, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return r.save(ctx, w, "Invalid", "CredentialMissing", true)
		}
		return ctrl.Result{}, err
	}
	if !CredentialMatches(w, secret) {
		return r.save(ctx, w, "Invalid", "CredentialOwnershipMismatch", true)
	}
	w.Status.Accepted = true
	w.Status.ObservedGeneration = w.Generation
	w.Status.LaunchAttempted = true
	w.Status.LaunchAttemptedAt = ptrTime(now)
	w.Status.PodName = w.Name
	w.Status.Phase = "Submitting"
	w.Status.Reason = "LaunchFenced"
	// Only this invocation can submit after a successful resourceVersion-checked status write.
	observations(w)
	if err := r.Status().Update(ctx, w); err != nil {
		return ctrl.Result{}, err
	}
	if r.AfterFence != nil {
		if err := r.AfterFence(); err != nil {
			return ctrl.Result{}, err
		}
	}
	if ctx.Err() != nil {
		return ctrl.Result{}, ctx.Err()
	}
	pod := builders.Runner(w)
	if r.CreatePod == nil {
		return ctrl.Result{}, nil
	} // fail closed; no generic client fallback
	if err := r.CreatePod(ctx, pod); err != nil {
		w.Status.SubmissionUncertain = true
		w.Status.Phase = "Uncertain"
		w.Status.Reason = "CreateOutcomeUnknown"
		// Never expose the API response, which may echo the credential-bearing object.
		observations(w)
		return ctrl.Result{RequeueAfter: time.Second}, r.Status().Update(ctx, w)
	}
	w.Status.PodObserved = true
	w.Status.PodUID = string(pod.UID)
	w.Status.Phase = "Pending"
	w.Status.Reason = "PodSubmitted"
	observations(w)
	return ctrl.Result{RequeueAfter: time.Second}, r.Status().Update(ctx, w)
}
func CredentialMatches(w *api.ClaudeWorkOrder, s *corev1.Secret) bool {
	return s.Namespace == w.Namespace && s.Name == w.Spec.CredentialSecretRef.Name && contract.Owns(w, s, "ClaudeWorkOrder") && s.Immutable != nil && *s.Immutable && len(s.Data) == 1 && contract.Hash(s.Data[contract.CredentialKey]) == w.Spec.TokenDigest
}
func (r *ClaudeWorkOrderReconciler) observe(ctx context.Context, w *api.ClaudeWorkOrder, now time.Time) (ctrl.Result, error) {
	p := &corev1.Pod{}
	err := r.Get(ctx, client.ObjectKey{Namespace: w.Namespace, Name: w.Name}, p)
	if apierrors.IsNotFound(err) {
		if w.Status.PodObserved {
			return r.save(ctx, w, "Lost", "PodDisappeared", true)
		}
		// Keep observing possible late POST completion. No second POST, even across restarts.
		if now.After(contract.CredentialFloor(w)) {
			return r.save(ctx, w, "Uncertain", "PodNeverObserved", true)
		}
		if !w.Status.SubmissionUncertain {
			w.Status.SubmissionUncertain = true
			return r.save(ctx, w, "Uncertain", "CreateOutcomeUnknown", false)
		}
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}
	if err != nil {
		return ctrl.Result{}, err
	}
	if !contract.Owns(w, p, "ClaudeWorkOrder") || (w.Status.PodUID != "" && string(p.UID) != w.Status.PodUID) {
		return r.save(ctx, w, "Invalid", "PodOwnershipOrUIDMismatch", true)
	}
	if !w.Status.PodObserved || w.Status.PodUID == "" {
		w.Status.PodObserved = true
		w.Status.PodUID = string(p.UID)
	}
	w.Status.SubmissionUncertain = false
	switch p.Status.Phase {
	case corev1.PodSucceeded:
		return r.save(ctx, w, "Succeeded", "PodExitedZero", true)
	case corev1.PodFailed:
		return r.save(ctx, w, "Failed", "PodExitedNonzero", true)
	case corev1.PodRunning:
		return r.save(ctx, w, "Running", "InfrastructureRunning", false)
	default:
		if now.After(contract.CredentialFloor(w)) {
			// Re-read before UID-constrained deletion. Startup can still race; do not claim atomicity.
			fresh := &corev1.Pod{}
			if err := r.Get(ctx, client.ObjectKeyFromObject(p), fresh); err != nil {
				return ctrl.Result{}, client.IgnoreNotFound(err)
			}
			if fresh.Status.Phase == corev1.PodRunning {
				return r.save(ctx, w, "Running", "InfrastructureRunning", false)
			}
			if err := r.Delete(ctx, fresh, client.Preconditions{UID: &p.UID, ResourceVersion: &fresh.ResourceVersion}); client.IgnoreNotFound(err) != nil {
				return ctrl.Result{}, err
			}
			return r.save(ctx, w, "Expired", "NeverStartedBeforeExpiry", true)
		}
		return r.save(ctx, w, "Pending", "SchedulingOrStarting", false)
	}
}
func ptrTime(t time.Time) *metav1.Time { x := metav1.NewTime(t); return &x }
func (r *ClaudeWorkOrderReconciler) save(ctx context.Context, w *api.ClaudeWorkOrder, phase, reason string, terminal bool) (ctrl.Result, error) {
	changed := w.Status.Phase != phase || w.Status.Reason != reason || w.Status.Terminal != terminal || w.Status.ObservedGeneration != w.Generation
	w.Status.Phase = phase
	w.Status.Reason = reason
	w.Status.Terminal = terminal
	w.Status.ObservedGeneration = w.Generation
	if terminal && w.Status.FinishedAt == nil {
		w.Status.FinishedAt = ptrTime(r.now())
		floor := contract.CredentialFloor(w)
		diag := r.now().Add(time.Duration(w.Spec.Execution.DiagnosticRetentionSeconds) * time.Second)
		if diag.After(floor) {
			floor = diag
		}
		w.Status.RetainUntil = ptrTime(floor)
		changed = true
	}
	if changed {
		observations(w)
		if err := r.Status().Update(ctx, w); err != nil {
			return ctrl.Result{}, err
		}
		if r.Recorder != nil {
			// Only fixed infrastructure reasons enter event messages; never API errors or tokens.
			r.Recorder.Event(w, corev1.EventTypeNormal, reason, reason)
		}
	}
	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}
func (r *ClaudeWorkOrderReconciler) cleanup(ctx context.Context, w *api.ClaudeWorkOrder, now time.Time, deleting bool) (ctrl.Result, error) {
	// Deletion never kills active execution. Fleet Abort deletes the owned Pod separately.
	p := &corev1.Pod{}
	err := r.Get(ctx, client.ObjectKey{Namespace: w.Namespace, Name: w.Name}, p)
	if err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}
	if err == nil && contract.Owns(w, p, "ClaudeWorkOrder") {
		if w.Status.PodUID != "" && w.Status.PodUID != string(p.UID) {
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		if p.Status.Phase != corev1.PodSucceeded && p.Status.Phase != corev1.PodFailed {
			return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
		}
		if deleting && !w.Status.Terminal {
			return r.save(ctx, w, string(p.Status.Phase), "InfrastructureExitedDuringDrain", true)
		}
		if err := r.Delete(ctx, p, client.Preconditions{UID: &p.UID}); client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
	}
	// Unobserved in-flight requests retain credentials through token validity plus skew.
	floor := contract.CredentialFloor(w)
	if w.Status.LaunchAttempted && !w.Status.PodObserved && now.Before(floor) {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}
	if deleting && !w.Status.Terminal {
		return r.save(ctx, w, "Cancelled", "OrderDrainedOrAborted", true)
	}
	s := &corev1.Secret{}
	err = r.Get(ctx, client.ObjectKey{Namespace: w.Namespace, Name: w.Spec.CredentialSecretRef.Name}, s)
	if err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}
	if err == nil {
		if contract.Owns(w, s, "ClaudeWorkOrder") {
			if err := r.Delete(ctx, s, client.Preconditions{UID: &s.UID}); client.IgnoreNotFound(err) != nil {
				return ctrl.Result{}, err
			}
		}
	}
	if w.Status.RetainUntil != nil && w.Status.RetainUntil.After(floor) {
		floor = w.Status.RetainUntil.Time
	}
	if now.Before(floor) {
		return ctrl.Result{RequeueAfter: time.Second * 10}, nil
	}
	if deleting {
		controllerutil.RemoveFinalizer(w, contract.OrderFinalizer)
		return ctrl.Result{}, r.Update(ctx, w)
	}
	if controllerutil.ContainsFinalizer(w, contract.OrderFinalizer) {
		controllerutil.RemoveFinalizer(w, contract.OrderFinalizer)
		if err := r.Update(ctx, w); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, client.IgnoreNotFound(r.Delete(ctx, w, client.Preconditions{UID: &w.UID}))
}
func (r *ClaudeWorkOrderReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).For(&api.ClaudeWorkOrder{}).Owns(&corev1.Pod{}).Complete(r)
}

func observations(w *api.ClaudeWorkOrder) {
	for kind, value := range map[string]bool{"Accepted": w.Status.Accepted, "LaunchAttempted": w.Status.LaunchAttempted, "PodObserved": w.Status.PodObserved, "Terminal": w.Status.Terminal, "SubmissionUncertain": w.Status.SubmissionUncertain} {
		status := metav1.ConditionFalse
		if value {
			status = metav1.ConditionTrue
		}
		reason := w.Status.Reason
		if reason == "" {
			reason = "ObservationPending"
		}
		apimeta.SetStatusCondition(&w.Status.Conditions, metav1.Condition{Type: kind, Status: status, Reason: reason, Message: reason, ObservedGeneration: w.Generation})
	}
}
