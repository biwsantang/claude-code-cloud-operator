// Copyright 2026 biwsantang. SPDX-License-Identifier: Apache-2.0
package admission

import (
	"fmt"
	registration "k8s.io/api/admissionregistration/v1"
	"slices"
)

// ConfigurationMatches rejects a fail-open or selectively disabled installation before intake is enabled.
func ConfigurationMatches(c *registration.ValidatingWebhookConfiguration, namespace string) bool {
	if len(c.Webhooks) != 2 {
		return false
	}
	seen := map[string]bool{}
	for _, w := range c.Webhooks {
		if seen[w.Name] {
			return false
		}
		seen[w.Name] = true
		if w.FailurePolicy == nil || *w.FailurePolicy != registration.Fail || w.MatchPolicy == nil || *w.MatchPolicy != registration.Equivalent || w.SideEffects == nil || *w.SideEffects != registration.SideEffectClassNone || len(w.ClientConfig.CABundle) == 0 || w.ClientConfig.Service == nil || w.ClientConfig.Service.Name != "cloud-operator-webhook" || w.ClientConfig.Service.Namespace != namespace || w.ClientConfig.Service.Path == nil || *w.ClientConfig.Service.Path != "/validate" || w.ClientConfig.Service.Port == nil || *w.ClientConfig.Service.Port != 443 || w.NamespaceSelector == nil || len(w.NamespaceSelector.MatchLabels) != 1 || w.NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"] != namespace || len(w.NamespaceSelector.MatchExpressions) != 0 || w.ObjectSelector != nil && (len(w.ObjectSelector.MatchLabels) != 0 || len(w.ObjectSelector.MatchExpressions) != 0) || len(w.Rules) != 1 {
			return false
		}
		r := w.Rules[0]
		if len(r.Operations) != 3 || !slices.Contains(r.Operations, registration.Create) || !slices.Contains(r.Operations, registration.Update) || !slices.Contains(r.Operations, registration.Delete) || r.Scope == nil || *r.Scope != registration.NamespacedScope || len(r.APIGroups) != 1 || len(r.APIVersions) != 1 {
			return false
		}
		switch w.Name {
		case "receipts.runners.biwsantang.github.io":
			if len(w.MatchConditions) != 0 || r.APIGroups[0] != "runners.biwsantang.github.io" || r.APIVersions[0] != "v1alpha1" || len(r.Resources) != 4 {
				return false
			}
			for _, resource := range []string{"clauderunnerfleets", "clauderunnerfleets/status", "claudeworkorders", "claudeworkorders/status"} {
				if !slices.Contains(r.Resources, resource) {
					return false
				}
			}
		case "credentials.runners.biwsantang.github.io":
			expression := fmt.Sprintf("request.userInfo.username.startsWith('system:serviceaccount:%s:') && request.userInfo.username.endsWith('-hook')", namespace)
			if len(w.MatchConditions) != 1 || w.MatchConditions[0].Name != "hook-service-accounts" || w.MatchConditions[0].Expression != expression || r.APIGroups[0] != "" || r.APIVersions[0] != "v1" || len(r.Resources) != 1 || r.Resources[0] != "secrets" {
				return false
			}
		default:
			return false
		}
	}
	return true
}
