// Copyright 2026 biwsantang. SPDX-License-Identifier: Apache-2.0
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// +kubebuilder:validation:XValidation:rule="self.fleetName == oldSelf.fleetName && self.fleetUID == oldSelf.fleetUID && self.poolID == oldSelf.poolID && self.orderID == oldSelf.orderID && self.tokenDigest == oldSelf.tokenDigest && self.expiresAt == oldSelf.expiresAt && self.clockOffsetSeconds == oldSelf.clockOffsetSeconds && self.credentialSecretRef == oldSelf.credentialSecretRef && self.policyDigest == oldSelf.policyDigest",message="Receipt identity and accepted policy are immutable"
// +kubebuilder:validation:XValidation:rule="!oldSelf.complete || self.complete",message="Complete receipts cannot be reopened"
type ClaudeWorkOrderSpec struct {
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	FleetName string `json:"fleetName"`
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	FleetUID string `json:"fleetUID"`
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	PoolID string `json:"poolID"`
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	OrderID string `json:"orderID"`
	// +kubebuilder:validation:Pattern=`^[a-f0-9]{64}$`
	TokenDigest string      `json:"tokenDigest"`
	ExpiresAt   metav1.Time `json:"expiresAt"`
	// ClockOffsetSeconds is local minus vendor time measured from the native poll's HTTP Date.
	// It corrects the signed expiry into the synchronized cluster clock domain; redelivery cannot change it.
	// +kubebuilder:default=0
	// +kubebuilder:validation:Minimum=-3600
	// +kubebuilder:validation:Maximum=3600
	ClockOffsetSeconds  int64          `json:"clockOffsetSeconds"`
	CredentialSecretRef LocalReference `json:"credentialSecretRef"`
	// +kubebuilder:validation:Pattern=`^[a-f0-9]{64}$`
	PolicyDigest string          `json:"policyDigest"`
	Execution    ExecutionPolicy `json:"execution"`
	Complete     bool            `json:"complete"`
}

type ClaudeWorkOrderStatus struct {
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	Accepted           bool  `json:"accepted,omitempty"`
	// LaunchAttempted is a monotonic fence, persisted before the sole Pod POST.
	LaunchAttempted     bool         `json:"launchAttempted,omitempty"`
	LaunchAttemptedAt   *metav1.Time `json:"launchAttemptedAt,omitempty"`
	PodName             string       `json:"podName,omitempty"`
	PodUID              string       `json:"podUID,omitempty"`
	PodObserved         bool         `json:"podObserved,omitempty"`
	Terminal            bool         `json:"terminal,omitempty"`
	SubmissionUncertain bool         `json:"submissionUncertain,omitempty"`
	Phase               string       `json:"phase,omitempty"`
	Reason              string       `json:"reason,omitempty"`
	FinishedAt          *metav1.Time `json:"finishedAt,omitempty"`
	RetainUntil         *metav1.Time `json:"retainUntil,omitempty"`
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Fenced",type=boolean,JSONPath=`.status.launchAttempted`
type ClaudeWorkOrder struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ClaudeWorkOrderSpec   `json:"spec"`
	Status            ClaudeWorkOrderStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type ClaudeWorkOrderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClaudeWorkOrder `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(GroupVersion, &ClaudeWorkOrder{}, &ClaudeWorkOrderList{})
		return nil
	})
}
