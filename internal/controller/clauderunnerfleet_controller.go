// Copyright 2026 biwsantang. SPDX-License-Identifier: Apache-2.0
package controller

import (
	"context"
	api "github.com/biwsantang/claude-code-cloud-operator/api/v1alpha1"
	"github.com/biwsantang/claude-code-cloud-operator/internal/builders"
	"github.com/biwsantang/claude-code-cloud-operator/internal/contract"
	"github.com/biwsantang/claude-code-cloud-operator/internal/telemetry"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	"reflect"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"time"
)

type ClaudeRunnerFleetReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	AdmissionReady func(context.Context) bool
	Now            func() time.Time
}

func (r *ClaudeRunnerFleetReconciler) now() time.Time {
	if r.Now != nil {
		return r.Now().UTC()
	}
	return time.Now().UTC()
}

// +kubebuilder:rbac:groups="",resources=serviceaccounts;configmaps,verbs=get;list;watch;create;update;patch;delete,namespace=cloud-operator-system
// +kubebuilder:rbac:groups="",resources=secrets,verbs=create,namespace=cloud-operator-system
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete,namespace=cloud-operator-system
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete,namespace=cloud-operator-system
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles;rolebindings,verbs=get;list;watch;create;update;patch;delete,namespace=cloud-operator-system
// +kubebuilder:rbac:groups=runners.biwsantang.github.io,resources=claudeworkorders,verbs=create,namespace=cloud-operator-system

// Reconcile converges this resource and its owned state.
func (r *ClaudeRunnerFleetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	f := &api.ClaudeRunnerFleet{}
	if err := r.Get(ctx, req.NamespacedName, f); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if !f.DeletionTimestamp.IsZero() {
		return r.drain(ctx, f)
	}
	if !controllerutil.ContainsFinalizer(f, contract.FleetFinalizer) {
		controllerutil.AddFinalizer(f, contract.FleetFinalizer)
		return ctrl.Result{RequeueAfter: time.Millisecond}, r.Update(ctx, f)
	}
	before := f.Status.DeepCopy()
	f.Status.ObservedGeneration = f.Generation
	f.Status.PolicyDigest = contract.PolicyDigest(f.Spec.Execution)
	condition := func(kind string, ready bool, reason string) {
		s := metav1.ConditionFalse
		if ready {
			s = metav1.ConditionTrue
		}
		apimeta.SetStatusCondition(&f.Status.Conditions, metav1.Condition{Type: kind, Status: s, Reason: reason, Message: reason, ObservedGeneration: f.Generation})
	}
	valid := contract.ValidateFleet(f) == nil
	condition("Configured", valid, "ConfigurationEvaluated")
	credentials := false
	revision := ""
	s := &corev1.Secret{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: f.Namespace, Name: f.Spec.EnvironmentSecretRef.Name}, s); err == nil {
		credentials = len(s.Data[contract.EnvironmentKey]) > 0
		revision = contract.Hash(s.Data[contract.EnvironmentKey])
	} else if !apierrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}
	f.Status.CredentialRevision = revision
	condition("CredentialsReady", credentials, "CredentialEvaluated")
	network := contract.NetworkApproved(ctx, r.Client, f, r.now()) == nil
	condition("NetworkValidated", network, "NetworkReportEvaluated")
	admission := r.AdmissionReady != nil && r.AdmissionReady(ctx)
	condition("AdmissionReady", admission, "AdmissionEvaluated")
	condition("Suspended", f.Spec.Suspended, "SuspensionEvaluated")
	enabled := valid && credentials && network && admission && !f.Spec.Suspended && contract.InputsReady(ctx, r.Client, f, r.now()) == nil
	if valid {
		claim := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "pool-" + contract.Hash([]byte(f.Spec.EnvironmentID))[:40], Namespace: f.Namespace, OwnerReferences: []metav1.OwnerReference{contract.Owner(f, "ClaudeRunnerFleet")}}, Data: map[string]string{"poolID": f.Spec.EnvironmentID}}
		if err := r.converge(ctx, f, claim); err != nil {
			condition("Ready", false, "PoolClaimConflict")
			condition("Degraded", true, "PoolClaimConflict")
			if !reflect.DeepEqual(before, &f.Status) {
				_ = r.Status().Update(ctx, f)
			}
			return ctrl.Result{}, err
		}
		owner := []metav1.OwnerReference{contract.Owner(f, "ClaudeRunnerFleet")}
		for _, name := range []string{contract.HookAccount(f.Name), f.Name + "-session"} {
			if err := r.converge(ctx, f, &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: f.Namespace, OwnerReferences: owner}, AutomountServiceAccountToken: ptr.To(false)}); err != nil {
				return ctrl.Result{}, err
			}
		}
		role := &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: f.Name + "-hook", Namespace: f.Namespace, OwnerReferences: owner}, Rules: []rbacv1.PolicyRule{{APIGroups: []string{api.GroupVersion.Group}, Resources: []string{"clauderunnerfleets"}, ResourceNames: []string{f.Name}, Verbs: []string{"get"}}, {APIGroups: []string{api.GroupVersion.Group}, Resources: []string{"claudeworkorders"}, Verbs: []string{"get", "create", "update"}}, {APIGroups: []string{""}, Resources: []string{"secrets"}, Verbs: []string{"create"}}}}
		binding := &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: f.Name + "-hook", Namespace: f.Namespace, OwnerReferences: owner}, RoleRef: rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: role.Name}, Subjects: []rbacv1.Subject{{Kind: "ServiceAccount", Namespace: f.Namespace, Name: contract.HookAccount(f.Name)}}}
		for _, o := range []client.Object{role, binding, builders.Network(f), builders.Orchestrator(f, revision, enabled)} {
			if err := r.converge(ctx, f, o); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		// An invalid update cannot leave a previous poller enabled.
		dep := &appsv1.Deployment{}
		if err := r.Get(ctx, client.ObjectKey{Namespace: f.Namespace, Name: f.Name + "-orchestrator"}, dep); err == nil && contract.Owns(f, dep, "ClaudeRunnerFleet") && dep.Spec.Replicas != nil && *dep.Spec.Replicas != 0 {
			dep.Spec.Replicas = ptr.To(int32(0))
			if err := r.Update(ctx, dep); err != nil {
				return ctrl.Result{}, err
			}
		}
	}
	connected := false
	dep := &appsv1.Deployment{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: f.Namespace, Name: f.Name + "-orchestrator"}, dep); err == nil {
		// Recreate prevents old/new credential and lease generations from polling
		// together. Old availability must not make an unready replacement Connected.
		connected = enabled && dep.Status.ObservedGeneration == dep.Generation && dep.Status.UpdatedReplicas > 0 && dep.Status.UpdatedReplicas == dep.Status.Replicas && dep.Status.AvailableReplicas > 0
	}
	condition("Connected", connected, "NativeConnectedReadinessProbe")
	condition("Ready", enabled && connected, "FleetReadinessEvaluated")
	condition("Degraded", !valid || !credentials || !network || !admission || enabled && !connected, "PrerequisitesEvaluated")
	orders := &api.ClaudeWorkOrderList{}
	if err := r.List(ctx, orders, client.InNamespace(f.Namespace)); err != nil {
		return ctrl.Result{}, err
	}
	f.Status.Infrastructure = api.InfrastructureCounts{}
	for _, w := range orders.Items {
		if !contract.Owns(f, &w, "ClaudeRunnerFleet") {
			continue
		}
		switch {
		case w.Status.Terminal:
			f.Status.Infrastructure.Terminal++
		case w.Status.SubmissionUncertain:
			f.Status.Infrastructure.Uncertain++
		case w.Status.Phase == "Running":
			f.Status.Infrastructure.Running++
		default:
			f.Status.Infrastructure.Pending++
		}
	}
	telemetry.Retained.Set(string(f.UID), f.Status.Infrastructure)
	if !reflect.DeepEqual(before, &f.Status) {
		if err := r.Status().Update(ctx, f); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
}

// converge changes only resources already owned by this Fleet; never adopts collisions.
func (r *ClaudeRunnerFleetReconciler) converge(ctx context.Context, f *api.ClaudeRunnerFleet, desired client.Object) error {
	current := desired.DeepCopyObject().(client.Object)
	err := r.Get(ctx, client.ObjectKeyFromObject(desired), current)
	if apierrors.IsNotFound(err) {
		return r.Create(ctx, desired)
	}
	if err != nil {
		return err
	}
	if !contract.Owns(f, current, "ClaudeRunnerFleet") {
		return apierrors.NewConflict(schema.GroupResource{Group: api.GroupVersion.Group, Resource: "owned-resource"}, desired.GetName(), nil)
	}
	before := current.DeepCopyObject()
	switch d := desired.(type) {
	case *corev1.ConfigMap:
		current.(*corev1.ConfigMap).Data = d.Data
	case *corev1.ServiceAccount:
		current.(*corev1.ServiceAccount).AutomountServiceAccountToken = d.AutomountServiceAccountToken
	case *appsv1.Deployment:
		c := current.(*appsv1.Deployment)
		c.Spec.Replicas = d.Spec.Replicas
		c.Spec.Strategy = d.Spec.Strategy
		c.Spec.Template = d.Spec.Template
	case *networkingv1.NetworkPolicy:
		c := current.(*networkingv1.NetworkPolicy)
		c.Spec = d.Spec
		if c.Annotations == nil {
			c.Annotations = map[string]string{}
		}
		c.Annotations[contract.Group+"/declared-policy-digest"] = d.Annotations[contract.Group+"/declared-policy-digest"]
	case *rbacv1.Role:
		current.(*rbacv1.Role).Rules = d.Rules
	case *rbacv1.RoleBinding:
		c := current.(*rbacv1.RoleBinding)
		c.RoleRef = d.RoleRef
		c.Subjects = d.Subjects
	}
	if equality.Semantic.DeepEqual(before, current) {
		return nil
	}
	return r.Update(ctx, current)
}
func (r *ClaudeRunnerFleetReconciler) drain(ctx context.Context, f *api.ClaudeRunnerFleet) (ctrl.Result, error) {
	f.Status.ObservedGeneration = f.Generation
	dep := &appsv1.Deployment{}
	if err := r.Get(ctx, client.ObjectKey{Namespace: f.Namespace, Name: f.Name + "-orchestrator"}, dep); err == nil && contract.Owns(f, dep, "ClaudeRunnerFleet") && dep.Spec.Replicas != nil && *dep.Spec.Replicas != 0 {
		dep.Spec.Replicas = ptr.To(int32(0))
		if err := r.Update(ctx, dep); err != nil {
			return ctrl.Result{}, err
		}
	}
	orders := &api.ClaudeWorkOrderList{}
	if err := r.List(ctx, orders, client.InNamespace(f.Namespace)); err != nil {
		return ctrl.Result{}, err
	}
	waiting := false
	for i := range orders.Items {
		w := &orders.Items[i]
		if !contract.Owns(f, w, "ClaudeRunnerFleet") {
			continue
		}
		waiting = true
		if f.Spec.DeletionPolicy == "Abort" {
			p := &corev1.Pod{}
			if err := r.Get(ctx, client.ObjectKey{Namespace: w.Namespace, Name: w.Name}, p); err == nil && contract.Owns(w, p, "ClaudeWorkOrder") {
				if err := r.Delete(ctx, p, client.Preconditions{UID: &p.UID}); client.IgnoreNotFound(err) != nil {
					return ctrl.Result{}, err
				}
			}
		}
		if w.DeletionTimestamp.IsZero() {
			if err := r.Delete(ctx, w, client.Preconditions{UID: &w.UID}); client.IgnoreNotFound(err) != nil {
				return ctrl.Result{}, err
			}
		}
	}
	if waiting {
		apimeta.SetStatusCondition(&f.Status.Conditions, metav1.Condition{Type: "Ready", Status: metav1.ConditionFalse, Reason: "DrainingOrdersAndRetention", Message: "Waiting for owned execution and retention", ObservedGeneration: f.Generation})
		return ctrl.Result{RequeueAfter: 5 * time.Second}, r.Status().Update(ctx, f)
	}
	telemetry.Retained.Remove(string(f.UID))
	controllerutil.RemoveFinalizer(f, contract.FleetFinalizer)
	return ctrl.Result{}, r.Update(ctx, f)
}
func (r *ClaudeRunnerFleetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).For(&api.ClaudeRunnerFleet{}).Owns(&appsv1.Deployment{}).Owns(&api.ClaudeWorkOrder{}).Complete(r)
}
