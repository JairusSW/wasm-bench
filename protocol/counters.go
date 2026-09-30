package protocol

import "fmt"

// ValidateCounterRun is the initial adapter contract for core scalar compile
// instantiation, first-invocation and steady diagnostics. It does not imply host PMU availability.
func ValidateCounterRun(p *Preparation, r *RunRequest) error {
	if p == nil || r == nil || p.Profile != "counters" {
		return fmt.Errorf("counters preparation required")
	}
	w := p.Workload
	if w.ABI != "core" || w.Command != nil || w.Vectors != nil || w.Density != nil || w.Oracle.Kind != "exact_u64" || (w.Reset != "stateless" && w.Reset != "fresh_instance_per_sample") || w.Export == "" {
		return fmt.Errorf("counters require a core scalar exact oracle")
	}
	if r.Scenario != "compile" && r.Scenario != "instantiate" && r.Scenario != "first-call" && r.Scenario != "steady" {
		return fmt.Errorf("unsupported counter scenario")
	}
	if r.Scenario == "steady" && w.Reset != "stateless" {
		return fmt.Errorf("steady counters require stateless repeated invocations")
	}
	_, err := CounterSampleCount(r)
	return err
}

// CounterSampleCount includes retained warmup batches. Non-steady lifecycle
// windows remain individual operations with no warmup.
func CounterSampleCount(r *RunRequest) (int, error) {
	if r == nil || !r.PhaseBarriers || r.Samples < 1 || r.Samples > 100000 || r.Operations < 1 || r.Operations > 1000000 || r.Warmup < 0 || r.Warmup > 100000 {
		return 0, fmt.Errorf("counter batch requires phase barriers and bounded samples, operations and warmup")
	}
	if r.Scenario != "steady" && (r.Operations != 1 || r.Warmup != 0) {
		return 0, fmt.Errorf("non-steady counters require one operation and no warmup")
	}
	return r.Samples + r.Warmup, nil
}
