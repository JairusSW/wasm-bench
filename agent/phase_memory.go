package agent

import "github.com/wasmbench/wasmbench/protocol"

func (c *Client) PhaseMemory(stage string) []protocol.Observation { return c.cgroup.phaseMemory(stage) }

func phaseWindow(stage string) (scenario string, start, end bool) {
	for _, scenario := range append(append(protocol.CheckpointScenarios(), protocol.ContinuationScenarios()...), "guest-density") {
		for index, candidate := range protocol.PhaseStages(scenario) {
			if stage == candidate {
				return scenario, index == 0, index == 1
			}
		}
	}
	switch stage {
	case "compile_release_entry", "compile_release_completed":
		return "compile", false, false
	case "instantiate_release_entry", "instantiate_release_completed":
		return "instantiate", false, false
	case "first_call_release_entry", "first_call_release_completed":
		return "first-call", false, false
	case "steady_release_entry", "steady_release_decision_completed":
		return "steady", false, false
	case "before_steady_batch":
		return "steady", true, false
	case "steady_batch_returned":
		return "steady", false, true
	case "steady_batch_verified":
		return "steady", false, false
	case "before_first_call":
		return "first-call", true, false
	case "first_call_returned":
		return "first-call", false, true
	case "first_call_released":
		return "first-call", false, false
	case "before_density_cycle":
		return "density-cycle", true, false
	case "density_cycle_ready":
		return "density-cycle", false, true
	case "density_cycle_released":
		return "density-cycle", false, false
	case "before_density":
		return "density", true, false
	case "density_ready":
		return "density", false, true
	case "density_released":
		return "density", false, false
	case "before_instantiate":
		return "instantiate", true, false
	case "instantiated":
		return "instantiate", false, true
	case "instance_released":
		return "instantiate", false, false
	case "before_app_init":
		return "app-init", true, false
	case "app_initialized":
		return "app-init", false, true
	case "app_released":
		return "app-init", false, false
	case "before_teardown":
		return "teardown", true, false
	case "torn_down":
		return "teardown", false, true
	default:
		return "compile", stage == "before_compile", stage == "compiled"
	}
}

func phasePeakObservation(reason string) protocol.Observation {
	return protocol.Observation{Metric: "cgroup.memory.phase_peak", DefinitionVersion: 1, Unit: "bytes", Scope: "adapter_cgroup", Phase: "compile/barrier_window", Collector: "cgroup_v2_same_fd", CollectorVersion: "1", Quality: "kernel_accounted_peak", Profile: "memory", Status: "unavailable", Reason: reason, Denominator: "diagnostic_operation_including_barrier_transport"}
}
