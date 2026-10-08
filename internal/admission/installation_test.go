// Copyright 2026 biwsantang. SPDX-License-Identifier: Apache-2.0
package admission

import (
	registration "k8s.io/api/admissionregistration/v1"
	"k8s.io/utils/ptr"
	"os"
	"sigs.k8s.io/yaml"
	"testing"
)

func TestInstallationScope(t *testing.T) {
	b, err := os.ReadFile("../../config/webhook/manifests.yaml")
	if err != nil {
		t.Fatal(err)
	}
	c := &registration.ValidatingWebhookConfiguration{}
	if yaml.Unmarshal(b, c) != nil {
		t.Fatal("invalid generated webhook configuration")
	}
	for i := range c.Webhooks {
		c.Webhooks[i].ClientConfig.CABundle = []byte("synthetic-ca")
		c.Webhooks[i].Rules[0].Scope = ptr.To(registration.NamespacedScope)
	}
	if !ConfigurationMatches(c, "cloud-operator-system") {
		t.Fatal("source configuration failed readiness validation")
	}
	for _, mutate := range []func(*registration.ValidatingWebhookConfiguration){
		func(c *registration.ValidatingWebhookConfiguration) { c.Webhooks[1].MatchConditions = nil },
		func(c *registration.ValidatingWebhookConfiguration) {
			c.Webhooks[0].Rules[0].Resources = []string{"clauderunnerfleets"}
		},
		func(c *registration.ValidatingWebhookConfiguration) {
			c.Webhooks[0].FailurePolicy = ptr.To(registration.Ignore)
		},
		func(c *registration.ValidatingWebhookConfiguration) {
			c.Webhooks[0].ClientConfig.Service.Namespace = "other"
		},
	} {
		bad := c.DeepCopy()
		mutate(bad)
		if ConfigurationMatches(bad, "cloud-operator-system") {
			t.Fatal("incomplete or bypassable installation accepted")
		}
	}
}
