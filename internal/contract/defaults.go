// Copyright 2026 biwsantang. SPDX-License-Identifier: Apache-2.0
package contract

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"reflect"

	api "github.com/biwsantang/claude-code-cloud-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// FleetDefaults is non-secret installation configuration. It is copied into the
// poller environment so admission, reconciliation and intake use the same resolver.
// Changes affect new intake only, never accepted WorkOrder snapshots.
type FleetDefaults struct {
	RuntimeImage      string           `json:"runtimeImage,omitempty"`
	OrchestratorImage string           `json:"orchestratorImage,omitempty"`
	HookImage         string           `json:"hookImage,omitempty"`
	SecurityRevision  string           `json:"securityRevision,omitempty"`
	Proxy             *api.ProxyPolicy `json:"proxy,omitempty"`
}

func DecodeDefaults(raw string) (FleetDefaults, error) {
	var d FleetDefaults
	if raw == "" {
		return d, nil
	}
	if len(raw) > 32768 {
		return d, errors.New("installation defaults exceed the size bound")
	}
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&d); err != nil {
		return d, errors.New("installation defaults are malformed")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return d, errors.New("installation defaults contain trailing data")
	}
	return d, d.Validate()
}

func (d FleetDefaults) Validate() error {
	for _, image := range []string{d.RuntimeImage, d.OrchestratorImage, d.HookImage} {
		if image != "" && !digestImage.MatchString(image) {
			return errors.New("installation images must be digest pinned")
		}
	}
	if d.SecurityRevision != "" && !safeID.MatchString(d.SecurityRevision) {
		return errors.New("installation security revision is invalid")
	}
	if d.Proxy != nil {
		return validateProxy(*d.Proxy)
	}
	return nil
}

// ResolveFleet never mutates user configuration. A complete legacy manifest has
// exactly its previous policy digest; no installation default is applied to receipts.
func ResolveFleet(in *api.ClaudeRunnerFleet, d FleetDefaults) *api.ClaudeRunnerFleet {
	f := in.DeepCopy()
	s := &f.Spec
	p := &s.Execution
	if p.RunnerImage == "" {
		p.RunnerImage = d.RuntimeImage
	}
	if s.OrchestratorImage == "" {
		s.OrchestratorImage = d.OrchestratorImage
		if s.OrchestratorImage == "" {
			s.OrchestratorImage = d.RuntimeImage
		}
		if s.OrchestratorImage == "" {
			s.OrchestratorImage = p.RunnerImage
		}
	}
	if s.HookImage == "" {
		s.HookImage = d.HookImage
	}
	if s.NetworkReportRef.Name == "" {
		s.NetworkReportRef.Name = f.Name + "-network-report"
	}
	if s.OrchestratorReplicas == 0 {
		s.OrchestratorReplicas = 2
	}
	if s.HookTimeoutSeconds == 0 {
		s.HookTimeoutSeconds = 15
	}
	if s.SpawnLeaseSeconds == 0 {
		s.SpawnLeaseSeconds = 120
	}
	if s.DeletionPolicy == "" {
		s.DeletionPolicy = "Drain"
	}
	if p.SecurityRevision == "" {
		p.SecurityRevision = d.SecurityRevision
		if p.SecurityRevision == "" {
			p.SecurityRevision = "installation-v1"
		}
	}
	// Proxy identity is atomic: partial overrides must be rejected, not mixed
	// with selectors from another proxy or silently repaired.
	if reflect.DeepEqual(p.Proxy, api.ProxyPolicy{}) && d.Proxy != nil {
		p.Proxy = *d.Proxy.DeepCopy()
	}
	if p.Resources.Requests == nil {
		p.Resources.Requests = corev1.ResourceList{}
	}
	if p.Resources.Limits == nil {
		p.Resources.Limits = corev1.ResourceList{}
	}
	for key, value := range map[corev1.ResourceName]string{corev1.ResourceCPU: "2", corev1.ResourceMemory: "4Gi"} {
		if _, present := p.Resources.Requests[key]; !present {
			p.Resources.Requests[key] = resource.MustParse(value)
		}
	}
	if _, present := p.Resources.Limits[corev1.ResourceMemory]; !present {
		p.Resources.Limits[corev1.ResourceMemory] = resource.MustParse("4Gi")
	}
	if p.WorkspaceSize == "" {
		p.WorkspaceSize = "8Gi"
	}
	if p.MaxTokenLifetimeSeconds == 0 {
		p.MaxTokenLifetimeSeconds = 86400
	}
	if p.ClockMarginSeconds == 0 {
		p.ClockMarginSeconds = 300
	}
	if p.DiagnosticRetentionSeconds == 0 {
		p.DiagnosticRetentionSeconds = 3600
	}
	return f
}
