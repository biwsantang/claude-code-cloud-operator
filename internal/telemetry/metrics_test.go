// Copyright 2026 biwsantang. SPDX-License-Identifier: Apache-2.0
package telemetry

import (
	api "github.com/biwsantang/claude-code-cloud-operator/api/v1alpha1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"strings"
	"testing"
)

func TestCountsDoNotExposeIdentity(t *testing.T) {
	c := NewCounts()
	c.Set("synthetic-private-uid", api.InfrastructureCounts{Terminal: 2})
	registry := prometheus.NewRegistry()
	registry.MustRegister(c)
	expected := `# HELP claude_operator_orders Retained order observations; infrastructure state only
# TYPE claude_operator_orders gauge
claude_operator_orders{phase="pending"} 0
claude_operator_orders{phase="running"} 0
claude_operator_orders{phase="terminal"} 2
claude_operator_orders{phase="uncertain"} 0
`
	if err := testutil.GatherAndCompare(registry, strings.NewReader(expected)); err != nil {
		t.Fatal(err)
	}
	c.Remove("synthetic-private-uid")
	if testutil.CollectAndCount(c) != 4 {
		t.Fatal("fixed phase observations lost")
	}
}
