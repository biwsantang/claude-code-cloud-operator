// Copyright 2026 biwsantang. SPDX-License-Identifier: Apache-2.0
package builders

import (
	"encoding/json"
	api "github.com/biwsantang/claude-code-cloud-operator/api/v1alpha1"
	"github.com/biwsantang/claude-code-cloud-operator/internal/contract"
	"k8s.io/utils/ptr"
	"strings"
	"testing"
)

func TestSessionIsolation(t *testing.T) {
	w := &api.ClaudeWorkOrder{}
	w.Name = "order-example"
	w.Namespace = "operator"
	w.Spec.FleetName = "fleet"
	w.Spec.PolicyDigest = strings.Repeat("a", 64)
	w.Spec.CredentialSecretRef.Name = "own-credential"
	w.Spec.Execution.WorkspaceSize = "8Gi"
	w.Spec.Execution.RunnerImage = "runtime@sha256:" + strings.Repeat("a", 64)
	p := Runner(w)
	if *p.Spec.AutomountServiceAccountToken || p.Spec.HostNetwork || p.Spec.HostPID || p.Spec.RestartPolicy != "Never" {
		t.Fatal("unsafe Pod identity or restart policy")
	}
	c := p.Spec.Containers[0]
	if !*c.SecurityContext.ReadOnlyRootFilesystem || *c.SecurityContext.AllowPrivilegeEscalation || *c.SecurityContext.RunAsUser == 0 {
		t.Fatal("unsafe security context")
	}
	b, _ := json.Marshal(p)
	for _, bad := range []string{"SELF_HOSTED_RUNNER_ENVIRONMENT_SECRET", "hostPath", "ANTHROPIC_API_KEY", "AWS_"} {
		if strings.Contains(string(b), bad) {
			t.Fatalf("unexpected %s", bad)
		}
	}
	for _, v := range p.Spec.Volumes {
		if v.Secret != nil && v.Secret.SecretName != w.Spec.CredentialSecretRef.Name {
			t.Fatal("foreign credential")
		}
		if v.EmptyDir != nil && v.EmptyDir.SizeLimit == nil {
			t.Fatal("unbounded storage")
		}
	}
	if p.Labels[contract.RoleLabel] != "session" {
		t.Fatal("network selector missing")
	}
}

func TestLifecycleBudgetAndLegacyPod(t *testing.T) {
	w := &api.ClaudeWorkOrder{}
	w.Spec.Execution.WorkspaceSize = "8Gi"
	legacy := Runner(w)
	if *legacy.Spec.TerminationGracePeriodSeconds != 120 || len(legacy.Spec.InitContainers) != 0 || strings.Contains(strings.Join(legacy.Spec.Containers[0].Args, " "), "--release-idle-session-min") {
		t.Fatal("legacy receipt retroactively defaulted")
	}
	w.Spec.Execution.Lifecycle = contract.ResolvedLifecycle(nil)
	w.Spec.Execution.SessionConfigImage = "frozen-hook@sha256:" + strings.Repeat("b", 64)
	p := Runner(w)
	args := strings.Join(p.Spec.Containers[0].Args, " ")
	for _, flag := range []string{"--capacity 1", "--drain-grace-sec 0", "--release-idle-session-min 30", "--kill-session-after-min 480", "--drain-wait-sec 300", "--session-stop-grace-sec 5", "--post-session-hook-timeout-sec 60"} {
		if !strings.Contains(args, flag) {
			t.Fatal("missing", flag)
		}
	}
	if *p.Spec.TerminationGracePeriodSeconds != 420 || strings.Contains(args, "--push-outcome-on-release") || len(p.Spec.InitContainers) != 2 || p.Spec.InitContainers[0].Image != w.Spec.Execution.SessionConfigImage {
		t.Fatal("default lifecycle budget/config wrong")
	}
	w.Spec.Execution.Lifecycle.ShutdownWaitSeconds = ptr.To(int32(86400))
	w.Spec.Execution.Lifecycle.PushOutcomeOnRelease = ptr.To(true)
	w.Spec.Execution.Lifecycle.PromptToSave = ptr.To(false)
	p = Runner(w)
	if *p.Spec.TerminationGracePeriodSeconds != 86540 || len(p.Spec.InitContainers) != 2 || p.Spec.InitContainers[1].Command[3] != "false" || !strings.Contains(strings.Join(p.Spec.Containers[0].Args, " "), "--push-outcome-on-release") {
		t.Fatal("opt-in flags/budget wrong")
	}
}
