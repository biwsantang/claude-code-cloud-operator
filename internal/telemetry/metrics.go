// Copyright 2026 biwsantang. SPDX-License-Identifier: Apache-2.0
package telemetry

import (
	api "github.com/biwsantang/claude-code-cloud-operator/api/v1alpha1"
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sync"
)

// Internal keys identify Fleets; exported labels are fixed infrastructure phases only.
type Counts struct {
	mu     sync.Mutex
	fleets map[string]api.InfrastructureCounts
	desc   *prometheus.Desc
}

func NewCounts() *Counts {
	return &Counts{fleets: map[string]api.InfrastructureCounts{}, desc: prometheus.NewDesc("claude_operator_orders", "Retained order observations; infrastructure state only", []string{"phase"}, nil)}
}
func (c *Counts) Set(uid string, v api.InfrastructureCounts) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fleets[uid] = v
}
func (c *Counts) Remove(uid string)                   { c.mu.Lock(); defer c.mu.Unlock(); delete(c.fleets, uid) }
func (c *Counts) Describe(ch chan<- *prometheus.Desc) { ch <- c.desc }
func (c *Counts) Collect(ch chan<- prometheus.Metric) {
	c.mu.Lock()
	defer c.mu.Unlock()
	total := api.InfrastructureCounts{}
	for _, v := range c.fleets {
		total.Pending += v.Pending
		total.Running += v.Running
		total.Terminal += v.Terminal
		total.Uncertain += v.Uncertain
	}
	for phase, value := range map[string]int32{"pending": total.Pending, "running": total.Running, "terminal": total.Terminal, "uncertain": total.Uncertain} {
		ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, float64(value), phase)
	}
}

var Retained = NewCounts()

func init() { metrics.Registry.MustRegister(Retained) }
