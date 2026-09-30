package agent

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/wasmbench/wasmbench/protocol"
)

type accountingMetric struct {
	key, metric, unit string
	scale             float64
}

var cpuAccountingMetrics = []accountingMetric{
	{"usage_usec", "time.cpu.total", "ns", 1000},
	{"user_usec", "time.cpu.user", "ns", 1000},
	{"system_usec", "time.cpu.system", "ns", 1000},
	{"nr_periods", "cgroup.cpu.periods", "count", 1},
	{"nr_throttled", "cgroup.cpu.throttled_periods", "count", 1},
	{"throttled_usec", "cgroup.cpu.throttled_time", "ns", 1000},
}

// Fields overlap (e.g. kernel includes slab); never sum these into a total.
var memoryAccountingMetrics = []accountingMetric{
	{"anon", "cgroup.memory.anon", "bytes", 1},
	{"file", "cgroup.memory.file", "bytes", 1},
	{"kernel", "cgroup.memory.kernel", "bytes", 1},
	{"sock", "cgroup.memory.sock", "bytes", 1},
	{"pagetables", "cgroup.memory.pagetables", "bytes", 1},
	{"slab", "cgroup.memory.slab", "bytes", 1},
}

func parseAccounting(data []byte) (map[string]uint64, error) {
	values := map[string]uint64{}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 2 {
			return nil, fmt.Errorf("malformed accounting row")
		}
		if _, exists := values[fields[0]]; exists {
			return nil, fmt.Errorf("duplicate accounting field %s", fields[0])
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid accounting field %s: %w", fields[0], err)
		}
		values[fields[0]] = value
	}
	return values, nil
}

func accountingObservations(specs []accountingMetric, before, after map[string]uint64, delta bool, phase, source, reason string) []protocol.Observation {
	var observations []protocol.Observation
	for _, spec := range specs {
		quality, denominator := "boundary_snapshot_only", "cgroup"
		if delta {
			quality = "kernel_accounted_delta"
			denominator = "diagnostic_operation_including_barrier_transport"
		}
		o := protocol.Observation{Metric: spec.metric, DefinitionVersion: 1, Unit: spec.unit, Scope: "adapter_cgroup", Phase: phase, Collector: source, CollectorVersion: "1", Quality: quality, Profile: "memory", Status: "unavailable", Reason: reason, Denominator: denominator}
		if strings.HasPrefix(spec.metric, "time.cpu.") {
			o.Scope = "adapter_cgroup_process_tree"
		}
		if strings.HasPrefix(spec.metric, "cgroup.cpu.") {
			o.Scope = "adapter_cgroup_local_bandwidth"
		}
		end, exists := after[spec.key]
		if reason == "" {
			switch {
			case !exists:
				o.Reason = "accounting field " + spec.key + " absent"
			case !delta:
				o.Value = protocol.Value(float64(end) * spec.scale)
				o.Status = "available"
			default:
				start, exists := before[spec.key]
				if !exists {
					o.Reason = "starting accounting field " + spec.key + " absent"
				} else if end < start {
					o.Reason = "accounting counter regressed"
				} else {
					o.Value = protocol.Value(float64(end-start) * spec.scale)
					o.Status = "available"
				}
			}
		}
		observations = append(observations, o)
	}
	return observations
}

func unavailablePhaseAccounting(reason string) []protocol.Observation {
	return accountingObservations(cpuAccountingMetrics, nil, nil, true, "compile/barrier_window", "cgroup_v2_cpu.stat", reason)
}

func unavailablePhaseCollection(stage, reason string) []protocol.Observation {
	scenario, _, end := phaseWindow(stage)
	out := accountingObservations(memoryAccountingMetrics, nil, nil, false, scenario+"/"+stage, "cgroup_v2_memory.stat", reason)
	if end {
		peak := phasePeakObservation(reason)
		peak.Phase = scenario + "/barrier_window"
		out = append(out, peak)
		out = append(out, accountingObservations(cpuAccountingMetrics, nil, nil, true, scenario+"/barrier_window", "cgroup_v2_cpu.stat", reason)...)
	}
	return out
}
