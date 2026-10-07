// Copyright 2026 biwsantang. SPDX-License-Identifier: Apache-2.0
package builders

import (
	"encoding/json"
	api "github.com/biwsantang/claude-code-cloud-operator/api/v1alpha1"
	"github.com/biwsantang/claude-code-cloud-operator/internal/contract"
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
