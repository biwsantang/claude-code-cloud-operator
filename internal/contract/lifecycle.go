// Copyright 2026 biwsantang. SPDX-License-Identifier: Apache-2.0
package contract

import (
	"errors"
	"reflect"

	api "github.com/biwsantang/claude-code-cloud-operator/api/v1alpha1"
	"k8s.io/utils/ptr"
)

// ResolvedLifecycle returns product defaults, without changing a Fleet or a stored receipt.
func ResolvedLifecycle(in *api.SessionLifecycle) *api.SessionLifecycle {
	l := &api.SessionLifecycle{}
	if in != nil {
		l = in.DeepCopy()
	}
	if l.IdleMinutes == nil {
		l.IdleMinutes = ptr.To(int32(30))
	}
	if l.MaxSessionMinutes == nil {
		l.MaxSessionMinutes = ptr.To(int32(480))
	}
	if l.ShutdownWaitSeconds == nil {
		l.ShutdownWaitSeconds = ptr.To(int32(300))
	}
	if l.PromptToSave == nil {
		l.PromptToSave = ptr.To(true)
	}
	if l.PushOutcomeOnRelease == nil {
		l.PushOutcomeOnRelease = ptr.To(false)
	}
	return l
}

// AcceptedExecution freezes lifecycle defaults and the trusted helper image once, at intake.
func AcceptedExecution(f *api.ClaudeRunnerFleet) api.ExecutionPolicy {
	p := *f.Spec.Execution.DeepCopy()
	p.Lifecycle = ResolvedLifecycle(p.Lifecycle)
	p.SessionConfigImage = f.Spec.HookImage
	return p
}

// ExecutionMatchesFleet preserves legacy receipts without silently changing their accepted intent.
// The helper image is an immutable intake snapshot, not a manager-global setting or a later upgrade constraint.
func ExecutionMatchesFleet(accepted, declared api.ExecutionPolicy) bool {
	if accepted.Lifecycle == nil {
		return reflect.DeepEqual(accepted, declared)
	}
	a, d := *accepted.DeepCopy(), *declared.DeepCopy()
	a.SessionConfigImage, d.SessionConfigImage = "", ""
	d.Lifecycle = ResolvedLifecycle(d.Lifecycle)
	return reflect.DeepEqual(a, d)
}

// NetworkDigest excludes only the additive session lifecycle/helper fields. Neither changes
// proxy peers or isolation. Legacy selectors remain identical; receipt digests cover both fields.
func NetworkDigest(p api.ExecutionPolicy) string {
	p.Lifecycle, p.SessionConfigImage = nil, ""
	return PolicyDigest(p)
}

func ValidateLifecycle(l *api.SessionLifecycle) error {
	if l == nil {
		return nil
	}
	r := ResolvedLifecycle(l)
	if *r.IdleMinutes < 0 || *r.IdleMinutes > 10080 || *r.MaxSessionMinutes < 0 || *r.MaxSessionMinutes > 10080 || *r.ShutdownWaitSeconds < 0 || *r.ShutdownWaitSeconds > 86400 {
		return errors.New("lifecycle minutes must be 0..10080 and shutdown seconds 0..86400")
	}
	if *r.IdleMinutes > 0 && *r.MaxSessionMinutes > 0 && *r.IdleMinutes > *r.MaxSessionMinutes {
		return errors.New("positive idleMinutes cannot exceed maxSessionMinutes")
	}
	return nil
}
