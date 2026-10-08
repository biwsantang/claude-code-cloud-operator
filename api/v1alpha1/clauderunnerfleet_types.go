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

// ExecutionPolicy is deliberately narrower than PodSpec. All accepted inputs are frozen in a receipt.
type ExecutionPolicy struct {
	// +kubebuilder:validation:Pattern=`^[a-zA-Z0-9./:_-]+@sha256:[a-f0-9]{64}$`
	RunnerImage string `json:"runnerImage"`
	// +kubebuilder:validation:MinLength=1
	SecurityRevision string                      `json:"securityRevision"`
	Proxy            ProxyPolicy                 `json:"proxy"`
	Resources        corev1.ResourceRequirements `json:"resources"`
	NodeSelector     map[string]string           `json:"nodeSelector,omitempty"`
	Tolerations      []corev1.Toleration         `json:"tolerations,omitempty"`
	HostConfigRef    *LocalReference             `json:"hostConfigRef,omitempty"`
	// +kubebuilder:default="8Gi"
	WorkspaceSize string `json:"workspaceSize"`
	// +kubebuilder:default=86400
	// +kubebuilder:validation:Minimum=60
	// +kubebuilder:validation:Maximum=604800
	MaxTokenLifetimeSeconds int64 `json:"maxTokenLifetimeSeconds"`
	// +kubebuilder:default=300
	// +kubebuilder:validation:Minimum=60
	// +kubebuilder:validation:Maximum=3600
	ClockMarginSeconds int64 `json:"clockMarginSeconds"`
	// +kubebuilder:default=3600
	// +kubebuilder:validation:Minimum=300
	// +kubebuilder:validation:Maximum=604800
	DiagnosticRetentionSeconds int64 `json:"diagnosticRetentionSeconds"`
}

type ClaudeRunnerFleetSpec struct {
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	EnvironmentID        string         `json:"environmentID"`
	EnvironmentSecretRef LocalReference `json:"environmentSecretRef"`
	// +kubebuilder:validation:Pattern=`^[a-zA-Z0-9./:_-]+@sha256:[a-f0-9]{64}$`
	OrchestratorImage string `json:"orchestratorImage"`
	// +kubebuilder:validation:Pattern=`^[a-zA-Z0-9./:_-]+@sha256:[a-f0-9]{64}$`
	HookImage string `json:"hookImage"`
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
	NetworkReportRef  LocalReference  `json:"networkReportRef"`
	Execution         ExecutionPolicy `json:"execution"`
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

type ClaudeRunnerFleetStatus struct {
	ObservedGeneration int64                `json:"observedGeneration,omitempty"`
	PolicyDigest       string               `json:"policyDigest,omitempty"`
	CredentialRevision string               `json:"credentialRevision,omitempty"`
	Infrastructure     InfrastructureCounts `json:"infrastructure,omitempty"`
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
