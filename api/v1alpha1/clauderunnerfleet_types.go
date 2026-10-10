// Copyright 2026 biwsantang. SPDX-License-Identifier: Apache-2.0
package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// LocalReference cannot name resources outside the Fleet namespace.
type LocalReference struct {
	// Namespace exists only to explicitly reject cross-namespace input.
	// +kubebuilder:validation:MaxLength=0
	Namespace string `json:"namespace,omitempty"`
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Name string `json:"name"`
}

type ProxyPolicy struct {
	// +kubebuilder:validation:Pattern=`^http://[a-z0-9.-]+:[0-9]+$`
	URL string `json:"url"`
	// +kubebuilder:validation:MinLength=1
	Namespace string `json:"namespace"`
	// +kubebuilder:validation:MinProperties=1
	PodLabels map[string]string `json:"podLabels"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`
}

// SessionLifecycle controls native session retention, independently of credential expiry.
// Omitted fields resolve once at new-order intake; never default stored receipts.
// +kubebuilder:validation:XValidation:rule="!has(self.idleMinutes) || !has(self.maxSessionMinutes) || self.idleMinutes == 0 || self.maxSessionMinutes == 0 || self.idleMinutes <= self.maxSessionMinutes",message="Positive idleMinutes cannot exceed maxSessionMinutes"
type SessionLifecycle struct {
	// IdleMinutes releases idle sessions using the native runner. New-order default: 30; 0 disables.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=10080
	IdleMinutes *int32 `json:"idleMinutes,omitempty"`
	// MaxSessionMinutes starts native release at the age threshold, followed by native grace. Default: 480; 0 disables.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=10080
	MaxSessionMinutes *int32 `json:"maxSessionMinutes,omitempty"`
	// ShutdownWaitSeconds lets an in-flight turn finish after SIGTERM. New-order default: 300; 0 skips the wait.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=86400
	ShutdownWaitSeconds *int32 `json:"shutdownWaitSeconds,omitempty"`
	// PromptToSave installs a once-per-turn reminder, never an unconditional git commit/push. New-order default: true.
	PromptToSave *bool `json:"promptToSave,omitempty"`
	// PushOutcomeOnRelease opts into native best-effort push of committed outcome refs. Restrict claude/* push access first. Default: false.
	PushOutcomeOnRelease *bool `json:"pushOutcomeOnRelease,omitempty"`
}

// ExecutionPolicy is deliberately narrower than PodSpec. Fleets may omit installation defaults.
// WorkOrders require the complete resolved policy and must never inherit defaults. All accepted inputs are frozen in a receipt.
type ExecutionPolicy struct {
	// +kubebuilder:validation:Pattern=`^[a-zA-Z0-9./:_-]+@sha256:[a-f0-9]{64}$`
	RunnerImage string `json:"runnerImage,omitempty,omitzero"`
	// +kubebuilder:validation:MinLength=1
	SecurityRevision string                      `json:"securityRevision,omitempty,omitzero"`
	Proxy            ProxyPolicy                 `json:"proxy,omitempty,omitzero"`
	Resources        corev1.ResourceRequirements `json:"resources,omitempty"`
	NodeSelector     map[string]string           `json:"nodeSelector,omitempty"`
	Tolerations      []corev1.Toleration         `json:"tolerations,omitempty"`
	HostConfigRef    *LocalReference             `json:"hostConfigRef,omitempty"`
	// +kubebuilder:default="8Gi"
	WorkspaceSize string `json:"workspaceSize,omitempty"`
	// +kubebuilder:default=86400
	// +kubebuilder:validation:Minimum=60
	// +kubebuilder:validation:Maximum=604800
	MaxTokenLifetimeSeconds int64 `json:"maxTokenLifetimeSeconds,omitempty"`
	// +kubebuilder:default=300
	// +kubebuilder:validation:Minimum=60
	// +kubebuilder:validation:Maximum=3600
	ClockMarginSeconds int64 `json:"clockMarginSeconds,omitempty"`
	// +kubebuilder:default=3600
	// +kubebuilder:validation:Minimum=300
	// +kubebuilder:validation:Maximum=604800
	DiagnosticRetentionSeconds int64 `json:"diagnosticRetentionSeconds,omitempty"`
	// Lifecycle is resolved and frozen only for newly accepted orders. Missing on a legacy receipt preserves legacy behavior.
	Lifecycle *SessionLifecycle `json:"lifecycle,omitempty"`
	// SessionConfigImage is stamped from the Fleet hook image at intake; do not set it on Fleets.
	// It pins the materializer/save helper for the accepted order, independently of later hook upgrades.
	// +kubebuilder:validation:Pattern=`^[a-zA-Z0-9./:_-]+@sha256:[a-f0-9]{64}$`
	SessionConfigImage string `json:"sessionConfigImage,omitempty"`
}

type ClaudeRunnerFleetSpec struct {
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	EnvironmentID        string         `json:"environmentID"`
	EnvironmentSecretRef LocalReference `json:"environmentSecretRef"`
	// +kubebuilder:validation:Pattern=`^[a-zA-Z0-9./:_-]+@sha256:[a-f0-9]{64}$`
	OrchestratorImage string `json:"orchestratorImage,omitempty,omitzero"`
	// +kubebuilder:validation:Pattern=`^[a-zA-Z0-9./:_-]+@sha256:[a-f0-9]{64}$`
	HookImage string `json:"hookImage,omitempty,omitzero"`
	// +kubebuilder:default=true
	Suspended bool `json:"suspended"`
	// OrchestratorReplicas controls polling availability, never session concurrency.
	// +kubebuilder:default=2
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=10
	OrchestratorReplicas int32 `json:"orchestratorReplicas"`
	// +kubebuilder:default=15
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=300
	HookTimeoutSeconds int32 `json:"hookTimeoutSeconds"`
	// +kubebuilder:default=120
	// +kubebuilder:validation:Minimum=10
	// +kubebuilder:validation:Maximum=3600
	SpawnLeaseSeconds int32           `json:"spawnLeaseSeconds"`
	NetworkReportRef  LocalReference  `json:"networkReportRef,omitempty,omitzero"`
	Execution         ExecutionPolicy `json:"execution,omitempty,omitzero"`
	// +kubebuilder:default=Drain
	// +kubebuilder:validation:Enum=Drain;Abort
	DeletionPolicy string `json:"deletionPolicy"`
}

type InfrastructureCounts struct {
	Pending   int32 `json:"pending"`
	Running   int32 `json:"running"`
	Terminal  int32 `json:"terminal"`
	Uncertain int32 `json:"uncertain"`
}

// FleetConfiguration exposes non-secret effective installation defaults without rewriting spec.
type FleetConfiguration struct {
	OrchestratorImage string          `json:"orchestratorImage"`
	HookImage         string          `json:"hookImage"`
	NetworkReportRef  LocalReference  `json:"networkReportRef"`
	Execution         ExecutionPolicy `json:"execution"`
}

type ClaudeRunnerFleetStatus struct {
	EffectiveConfiguration *FleetConfiguration  `json:"effectiveConfiguration,omitempty"`
	ObservedGeneration     int64                `json:"observedGeneration,omitempty"`
	PolicyDigest           string               `json:"policyDigest,omitempty"`
	CredentialRevision     string               `json:"credentialRevision,omitempty"`
	Infrastructure         InfrastructureCounts `json:"infrastructure,omitempty"`
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:printcolumn:name="Suspended",type=boolean,JSONPath=`.spec.suspended`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
type ClaudeRunnerFleet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ClaudeRunnerFleetSpec   `json:"spec"`
	Status            ClaudeRunnerFleetStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type ClaudeRunnerFleetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClaudeRunnerFleet `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(GroupVersion, &ClaudeRunnerFleet{}, &ClaudeRunnerFleetList{})
		return nil
	})
}
