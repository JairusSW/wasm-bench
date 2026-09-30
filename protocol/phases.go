package protocol

// PhaseStages defines ordered boundary handshakes, not timed API subphases.
// Compile retains its original three-stage protocol; teardown starts only
// after workload verification and has no intervening execution phase.
// App-init snapshots initialization before input/verification, then releases
// the verified instance while retaining the compiled module and engine.
func PhaseStages(scenario string) []string {
	if IsContinuationScenario(scenario) {
		return []string{"before_" + scenario, scenario + "_completed", scenario + "_released"}
	}
	if IsCheckpointScenario(scenario) {
		return []string{"before_" + scenario, scenario + "_returned", scenario + "_released"}
	}
	switch scenario {
	case "guest-density":
		return []string{"before_guest_density", "guest_density_ready", "guest_density_released"}
	case "steady":
		return []string{"before_steady_batch", "steady_batch_returned", "steady_batch_verified"}
	case "first-call":
		return []string{"before_first_call", "first_call_returned", "first_call_released"}
	case "density-cycle":
		return []string{"before_density_cycle", "density_cycle_ready", "density_cycle_released"}
	case "density":
		return []string{"before_density", "density_ready", "density_released"}
	case "instantiate":
		return []string{"before_instantiate", "instantiated", "instance_released"}
	case "compile":
		return []string{"before_compile", "compiled", "released"}
	case "teardown":
		return []string{"before_teardown", "torn_down"}
	case "app-init":
		return []string{"before_app_init", "app_initialized", "app_released"}
	default:
		return nil
	}
}
