package agent

import (
	"github.com/wasmbench/wasmbench/metrics"
	"testing"
)

func TestAccountingParser(t *testing.T) {
	values, err := parseAccounting([]byte("system_usec 2\nunknown_future_field 9\nusage_usec 7\nuser_usec 5\n"))
	if err != nil || values["usage_usec"] != 7 {
		t.Fatal(values, err)
	}
	for _, input := range []string{"usage_usec -1", "usage_usec NaN", "usage_usec 1\nusage_usec 2", "usage_usec", "usage_usec 18446744073709551616", "usage_usec 1 extra"} {
		if _, err := parseAccounting([]byte(input)); err == nil {
			t.Fatal("accepted malformed accounting", input)
		}
	}
}

func TestAccountingDeltaAndMissing(t *testing.T) {
	before := map[string]uint64{"usage_usec": 100, "user_usec": 70, "system_usec": 30, "nr_periods": 4, "nr_throttled": 2}
	after := map[string]uint64{"usage_usec": 150, "user_usec": 70, "system_usec": 20, "nr_periods": 5, "nr_throttled": 2}
	observations := accountingObservations(cpuAccountingMetrics, before, after, true, "compile/barrier_window", "cgroup_v2_cpu.stat", "")
	for _, o := range observations {
		switch o.Metric {
		case "time.cpu.total":
			if o.Value == nil || *o.Value != 50000 || o.Scope != "adapter_cgroup_process_tree" {
				t.Fatal(o)
			}
		case "time.cpu.user", "cgroup.cpu.throttled_periods":
			if o.Value == nil || *o.Value != 0 {
				t.Fatal("lost real zero", o)
			}
		case "time.cpu.system", "cgroup.cpu.throttled_time":
			if o.Value != nil || o.Status != "unavailable" || o.Reason == "" {
				t.Fatal("fabricated counter", o)
			}
		case "cgroup.cpu.periods":
			if o.Value == nil || *o.Value != 1 || o.Scope != "adapter_cgroup_local_bandwidth" {
				t.Fatal(o)
			}
		}
	}
	for _, o := range accountingObservations(cpuAccountingMetrics, before, after, true, "compile/barrier_window", "cgroup_v2_cpu.stat", "permission denied") {
		if o.Value != nil || o.Reason != "permission denied" {
			t.Fatal(o)
		}
	}
}

func TestAccountingRegistryCoverage(t *testing.T) {
	for _, specs := range [][]accountingMetric{cpuAccountingMetrics, memoryAccountingMetrics} {
		for _, spec := range specs {
			found := false
			for _, definition := range metrics.Registry {
				if definition.Name == spec.metric && definition.Unit == spec.unit {
					found = true
				}
			}
			if !found {
				t.Fatal("missing definition", spec)
			}
		}
	}
}
