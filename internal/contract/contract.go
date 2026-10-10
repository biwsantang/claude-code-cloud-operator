// Copyright 2026 biwsantang. SPDX-License-Identifier: Apache-2.0
package contract

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	api "github.com/biwsantang/claude-code-cloud-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"net/url"
	"reflect"
	"regexp"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
	"time"
)

const (
	Group          = "runners.biwsantang.github.io"
	FleetLabel     = Group + "/fleet"
	RevisionLabel  = Group + "/revision"
	RoleLabel      = Group + "/role"
	OrderFinalizer = Group + "/retain"
	FleetFinalizer = Group + "/drain"
	CredentialKey  = "work-order.jwt"
	EnvironmentKey = "environment-secret"
)

var digestImage = regexp.MustCompile(`^[a-zA-Z0-9./:_-]+@sha256:[a-f0-9]{64}$`)
var safeID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,256}$`)

// Keep infrastructure failures distinct from an invalid assertion without exposing API response text.
var ErrPrerequisiteAPI = errors.New("prerequisite API temporarily unavailable")
var ErrIntakePaused = errors.New("fleet intake paused")

func Hash(b []byte) string                      { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func Name(pool, order string) string            { return "order-" + Hash([]byte(pool + "\x00" + order))[:48] }
func PolicyDigest(p api.ExecutionPolicy) string { b, _ := json.Marshal(p); return Hash(b) }
func HookAccount(fleet string) string           { return fleet + "-hook" }
func ManagerUser(namespace, sa string) string   { return "system:serviceaccount:" + namespace + ":" + sa }
func Owns(owner metav1.Object, child metav1.Object, kind string) bool {
	r := metav1.GetControllerOf(child)
	return r != nil && r.UID == owner.GetUID() && r.Name == owner.GetName() && r.Kind == kind && r.APIVersion == api.GroupVersion.String()
}
func Owner(owner metav1.Object, kind string) metav1.OwnerReference {
	r := *metav1.NewControllerRef(owner, api.GroupVersion.WithKind(kind))
	r.BlockOwnerDeletion = nil
	return r
}
func ValidateExecution(p api.ExecutionPolicy) error {
	if err := ValidateLifecycle(p.Lifecycle); err != nil {
		return err
	}
	if p.SessionConfigImage != "" && !digestImage.MatchString(p.SessionConfigImage) {
		return errors.New("session configuration image must be digest pinned")
	}
	if !digestImage.MatchString(p.RunnerImage) || !safeID.MatchString(p.SecurityRevision) {
		return errors.New("execution image or security revision is invalid")
	}
	if err := validateProxy(p.Proxy); err != nil {
		return err
	}
	if !validLabels(p.NodeSelector) {
		return errors.New("placement selectors are invalid")
	}
	for _, toleration := range p.Tolerations {
		if err := validateToleration(toleration); err != nil {
			return err
		}
	}
	if p.HostConfigRef != nil && len(validation.IsDNS1123Subdomain(p.HostConfigRef.Name)) != 0 {
		return errors.New("host configuration reference is invalid")
	}
	if len(p.Resources.Claims) > 0 || len(p.Resources.Requests) != 2 || len(p.Resources.Limits) != 1 {
		return errors.New("resources require CPU/memory requests and memory limit only")
	}
	cpu := p.Resources.Requests[corev1.ResourceCPU]
	mem := p.Resources.Requests[corev1.ResourceMemory]
	lim := p.Resources.Limits[corev1.ResourceMemory]
	if cpu.Sign() <= 0 || mem.Sign() <= 0 || lim.Cmp(mem) < 0 || cpu.Cmp(resource.MustParse("64")) > 0 || lim.Cmp(resource.MustParse("256Gi")) > 0 {
		return errors.New("resource budget is invalid")
	}
	size, err := resource.ParseQuantity(p.WorkspaceSize)
	if err != nil || size.Sign() <= 0 || size.Cmp(resource.MustParse("128Gi")) > 0 {
		return errors.New("workspace must be bounded within 128Gi")
	}
	if p.MaxTokenLifetimeSeconds < 60 || p.MaxTokenLifetimeSeconds > 604800 || p.ClockMarginSeconds < 60 || p.ClockMarginSeconds > 3600 || p.DiagnosticRetentionSeconds < 300 || p.DiagnosticRetentionSeconds > 604800 {
		return errors.New("retention bounds are invalid")
	}
	return nil
}

func validateProxy(p api.ProxyPolicy) error {
	u, err := url.Parse(p.URL)
	if err != nil || u.Scheme != "http" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.Hostname() == "" || u.Port() != fmt.Sprint(p.Port) {
		return errors.New("proxy must be an explicit HTTP host and matching port")
	}
	if p.Namespace == "" || len(p.PodLabels) == 0 || p.Port < 1 || p.Port > 65535 {
		return errors.New("proxy peer selector is required")
	}
	if len(validation.IsDNS1123Label(p.Namespace)) != 0 || !validLabels(p.PodLabels) {
		return errors.New("proxy namespace or placement selectors are invalid")
	}
	// Approved DNS proxy service only; IP literals, localhost and arbitrary hosts are disallowed.
	if !strings.HasSuffix(u.Hostname(), "."+p.Namespace+".svc") && !strings.HasSuffix(u.Hostname(), "."+p.Namespace+".svc.cluster.local") {
		return errors.New("proxy must name a Service in its approved namespace")
	}
	serviceName := strings.Split(u.Hostname(), ".")[0]
	if len(validation.IsDNS1035Label(serviceName)) != 0 || u.Hostname() != serviceName+"."+p.Namespace+".svc" && u.Hostname() != serviceName+"."+p.Namespace+".svc.cluster.local" {
		return errors.New("proxy must name exactly one valid Service")
	}
	return nil
}

func validLabels(labels map[string]string) bool {
	for key, value := range labels {
		if len(validation.IsQualifiedName(key)) != 0 || len(validation.IsValidLabelValue(value)) != 0 {
			return false
		}
	}
	return true
}

// Restrict placement to the portable Equal/Exists contract, independent of feature gates.
func validateToleration(t corev1.Toleration) error {
	invalid := errors.New("placement toleration is invalid")
	if t.Key != "" && len(validation.IsQualifiedName(t.Key)) != 0 || t.Key == "" && t.Operator != corev1.TolerationOpExists {
		return invalid
	}
	switch t.Operator {
	case "", corev1.TolerationOpEqual:
		if len(validation.IsValidLabelValue(t.Value)) != 0 {
			return invalid
		}
	case corev1.TolerationOpExists:
		if t.Value != "" {
			return invalid
		}
	default:
		return invalid
	}
	switch t.Effect {
	case "", corev1.TaintEffectNoSchedule, corev1.TaintEffectPreferNoSchedule, corev1.TaintEffectNoExecute:
	default:
		return invalid
	}
	if t.TolerationSeconds != nil && (t.Effect != corev1.TaintEffectNoExecute || *t.TolerationSeconds < 0) {
		return invalid
	}
	return nil
}
func ValidateFleet(f *api.ClaudeRunnerFleet) error {
	if f.Spec.Execution.SessionConfigImage != "" {
		return errors.New("sessionConfigImage is managed by intake; set the Fleet hookImage")
	}
	if len(f.Name) > 40 || !safeID.MatchString(f.Spec.EnvironmentID) || !digestImage.MatchString(f.Spec.OrchestratorImage) || !digestImage.MatchString(f.Spec.HookImage) {
		return errors.New("fleet identity or images are invalid")
	}
	if f.Spec.EnvironmentSecretRef.Namespace != "" || f.Spec.NetworkReportRef.Namespace != "" {
		return errors.New("cross-namespace references are forbidden")
	}
	if f.Spec.Execution.HostConfigRef != nil && f.Spec.Execution.HostConfigRef.Namespace != "" {
		return errors.New("cross-namespace host config is forbidden")
	}
	if f.Spec.EnvironmentSecretRef.Name == "" || f.Spec.NetworkReportRef.Name == "" || f.Spec.OrchestratorReplicas < 1 || f.Spec.OrchestratorReplicas > 10 {
		return errors.New("required inputs are missing")
	}
	if len(validation.IsDNS1123Subdomain(f.Spec.EnvironmentSecretRef.Name)) != 0 || len(validation.IsDNS1123Subdomain(f.Spec.NetworkReportRef.Name)) != 0 {
		return errors.New("credential or report reference is invalid")
	}
	if f.Spec.HookTimeoutSeconds < 1 || f.Spec.HookTimeoutSeconds > 300 || f.Spec.SpawnLeaseSeconds < 10 || f.Spec.SpawnLeaseSeconds > 3600 || f.Spec.HookTimeoutSeconds+5 >= f.Spec.SpawnLeaseSeconds {
		return errors.New("hook timeout plus grace must be below the common spawn lease")
	}
	if f.Spec.DeletionPolicy != "Drain" && f.Spec.DeletionPolicy != "Abort" {
		return errors.New("invalid deletion policy")
	}
	return ValidateExecution(f.Spec.Execution)
}
func ValidateOrder(w *api.ClaudeWorkOrder) error {
	if w.Spec.Execution.Lifecycle != nil && !digestImage.MatchString(w.Spec.Execution.SessionConfigImage) {
		return errors.New("lifecycle receipts require their frozen session configuration image")
	}
	if w.Spec.ClockOffsetSeconds < -3600 || w.Spec.ClockOffsetSeconds > 3600 {
		return errors.New("receipt clock offset is outside bounds")
	}
	if w.Name != Name(w.Spec.PoolID, w.Spec.OrderID) || !safeID.MatchString(w.Spec.PoolID) || !safeID.MatchString(w.Spec.OrderID) || w.Spec.FleetUID == "" || w.Spec.FleetName == "" || len(w.Spec.TokenDigest) != 64 || w.Spec.CredentialSecretRef.Namespace != "" || w.Spec.CredentialSecretRef.Name != w.Name+"-credential" || w.Spec.PolicyDigest != PolicyDigest(w.Spec.Execution) {
		return errors.New("invalid receipt identity or policy")
	}
	if _, err := hex.DecodeString(w.Spec.TokenDigest); err != nil {
		return errors.New("invalid token digest")
	}
	return ValidateExecution(w.Spec.Execution)
}

// OrderExpiry maps the immutable signed expiry to the synchronized cluster clock domain.
func OrderExpiry(w *api.ClaudeWorkOrder) time.Time {
	return w.Spec.ExpiresAt.Add(time.Duration(w.Spec.ClockOffsetSeconds) * time.Second)
}

// CredentialFloor never shortens retention below either the signed or corrected expiry.
func CredentialFloor(w *api.ClaudeWorkOrder) time.Time {
	expiry := OrderExpiry(w)
	if w.Spec.ExpiresAt.After(expiry) {
		expiry = w.Spec.ExpiresAt.Time
	}
	return expiry.Add(time.Duration(w.Spec.Execution.ClockMarginSeconds) * time.Second)
}
func SameReceipt(a, b api.ClaudeWorkOrderSpec) bool {
	a.Complete = false
	b.Complete = false
	return reflect.DeepEqual(a, b)
}

// Report is an administrator's assertion of externally reproduced network tests, not CNI detection.
// Approval stays bound to the Fleet and policy until revoked or an optional expiry is reached.
type Report struct {
	FleetUID         string    `json:"fleetUID"`
	PolicyDigest     string    `json:"policyDigest"`
	ValidUntil       time.Time `json:"validUntil,omitzero"`
	TestedAt         time.Time `json:"testedAt"`
	DirectDenied     bool      `json:"directDenied"`
	PrivateDenied    bool      `json:"privateDenied"`
	MetadataDenied   bool      `json:"metadataDenied"`
	KubernetesDenied bool      `json:"kubernetesDenied"`
	ProxyAllowed     bool      `json:"proxyAllowed"`
	ProxyDenied      bool      `json:"proxyDenied"`
	FreshPodTested   bool      `json:"freshPodTested"`
	Evidence         string    `json:"evidence"`
}

func NetworkApproved(ctx context.Context, c client.Reader, f *api.ClaudeRunnerFleet, now time.Time) error {
	cm := &corev1.ConfigMap{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: f.Namespace, Name: f.Spec.NetworkReportRef.Name}, cm); err != nil {
		if !apierrors.IsNotFound(err) {
			return ErrPrerequisiteAPI
		}
		return errors.New("network report unavailable")
	}
	var r Report
	if err := json.Unmarshal([]byte(cm.Data["report.json"]), &r); err != nil {
		return errors.New("network report malformed")
	}
	if r.FleetUID != string(f.UID) || r.PolicyDigest != PolicyDigest(f.Spec.Execution) || r.TestedAt.IsZero() || r.TestedAt.After(now.Add(time.Minute)) || strings.TrimSpace(r.Evidence) == "" || !r.DirectDenied || !r.PrivateDenied || !r.MetadataDenied || !r.KubernetesDenied || !r.ProxyAllowed || !r.ProxyDenied || !r.FreshPodTested {
		return errors.New("network report does not match the policy and required tests")
	}
	if !r.ValidUntil.IsZero() && (!r.ValidUntil.After(now) || !r.ValidUntil.After(r.TestedAt)) {
		return errors.New("network report has expired or has an invalid expiry")
	}
	return nil
}
func InputsReady(ctx context.Context, c client.Reader, f *api.ClaudeRunnerFleet, now time.Time) error {
	if err := ValidateFleet(f); err != nil {
		return err
	}
	if f.Spec.Suspended || !f.DeletionTimestamp.IsZero() {
		return ErrIntakePaused
	}
	s := &corev1.Secret{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: f.Namespace, Name: f.Spec.EnvironmentSecretRef.Name}, s); err != nil {
		if !apierrors.IsNotFound(err) {
			return ErrPrerequisiteAPI
		}
		return errors.New("environment credential unavailable")
	}
	if len(s.Data[EnvironmentKey]) == 0 {
		return errors.New("environment credential unavailable")
	}
	if err := NetworkApproved(ctx, c, f, now); err != nil {
		return err
	}
	if f.Spec.Execution.HostConfigRef != nil {
		cm := &corev1.ConfigMap{}
		if err := c.Get(ctx, client.ObjectKey{Namespace: f.Namespace, Name: f.Spec.Execution.HostConfigRef.Name}, cm); err != nil {
			if !apierrors.IsNotFound(err) {
				return ErrPrerequisiteAPI
			}
			return errors.New("host configuration unavailable")
		}
		if cm.Immutable == nil || !*cm.Immutable {
			return errors.New("host configuration must exist and be immutable")
		}
	}
	return nil
}
