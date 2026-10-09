// Copyright 2026 biwsantang. SPDX-License-Identifier: Apache-2.0
package contract

import (
	"encoding/json"
	api "github.com/biwsantang/claude-code-cloud-operator/api/v1alpha1"
	"k8s.io/utils/ptr"
	"strings"
	"testing"
)

func TestLifecycleIntakeDoesNotRewriteLegacyPolicy(t *testing.T) {
	// Serialized fields from before lifecycle support: these bytes must remain unchanged.
	legacy := `{"runnerImage":"runtime@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","securityRevision":"v1","proxy":{"url":"http://proxy.proxy.svc:3128","namespace":"proxy","podLabels":{"app":"proxy"},"port":3128},"resources":{},"workspaceSize":"8Gi","maxTokenLifetimeSeconds":86400,"clockMarginSeconds":300,"diagnosticRetentionSeconds":3600}`
	var p api.ExecutionPolicy
	if err := json.Unmarshal([]byte(legacy), &p); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(p)
	if string(b) != legacy || PolicyDigest(p) != Hash([]byte(legacy)) {
		t.Fatal("legacy serialization or digest changed")
	}
	f := &api.ClaudeRunnerFleet{Spec: api.ClaudeRunnerFleetSpec{HookImage: "hook@sha256:" + strings.Repeat("b", 64), Execution: p}}
	accepted := AcceptedExecution(f)
	l := accepted.Lifecycle
	if *l.IdleMinutes != 30 || *l.MaxSessionMinutes != 480 || *l.ShutdownWaitSeconds != 300 || !*l.PromptToSave || *l.PushOutcomeOnRelease {
		t.Fatal("new intake defaults wrong")
	}
	if f.Spec.Execution.Lifecycle != nil || f.Spec.Execution.SessionConfigImage != "" {
		t.Fatal("intake mutated Fleet")
	}
	if PolicyDigest(accepted) == PolicyDigest(p) || NetworkDigest(accepted) != PolicyDigest(p) {
		t.Fatal("receipt must freeze new settings while preserving network selectors")
	}
	if !ExecutionMatchesFleet(accepted, p) || !ExecutionMatchesFleet(p, p) {
		t.Fatal("new/legacy matching failed")
	}
	f.Spec.HookImage = "hook@sha256:" + strings.Repeat("c", 64)
	if accepted.SessionConfigImage == AcceptedExecution(f).SessionConfigImage {
		t.Fatal("accepted helper was not frozen")
	}
	changed := *p.DeepCopy()
	changed.Lifecycle = ResolvedLifecycle(nil)
	changed.Lifecycle.IdleMinutes = ptr.To(int32(15))
	if ExecutionMatchesFleet(accepted, changed) || ExecutionMatchesFleet(p, changed) {
		t.Fatal("changed declared intent matched old receipt")
	}
	before := api.ClaudeWorkOrderSpec{Execution: accepted}
	after := before
	after.Execution = *accepted.DeepCopy()
	after.Execution.Lifecycle.PromptToSave = ptr.To(false)
	if SameReceipt(before, after) {
		t.Fatal("accepted lifecycle mutable")
	}
}

func TestLifecycleBoundsAndExplicitZero(t *testing.T) {
	for _, l := range []*api.SessionLifecycle{
		{IdleMinutes: ptr.To(int32(-1))}, {MaxSessionMinutes: ptr.To(int32(10081))}, {ShutdownWaitSeconds: ptr.To(int32(86401))}, {IdleMinutes: ptr.To(int32(481))}, {MaxSessionMinutes: ptr.To(int32(29))},
	} {
		if ValidateLifecycle(l) == nil {
			t.Fatal("invalid or partially invalid lifecycle accepted", l)
		}
	}
	l := &api.SessionLifecycle{IdleMinutes: ptr.To(int32(0)), MaxSessionMinutes: ptr.To(int32(0)), ShutdownWaitSeconds: ptr.To(int32(0)), PromptToSave: ptr.To(false), PushOutcomeOnRelease: ptr.To(true)}
	r := ResolvedLifecycle(l)
	if ValidateLifecycle(l) != nil || *r.IdleMinutes != 0 || *r.MaxSessionMinutes != 0 || *r.ShutdownWaitSeconds != 0 || *r.PromptToSave || !*r.PushOutcomeOnRelease {
		t.Fatal("explicit zero/false lost")
	}
}
