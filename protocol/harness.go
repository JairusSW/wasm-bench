package protocol

import "fmt"

const HarnessCalibrationScenario = "harness-calibration"
const HarnessCalibrationPolicy = "empty-local-harness-v1: timing only; resident stateless core scalar workload checked before sampling; preallocated uint64-equivalent slots; timed local loop writes iteration+1 to every slot without Wasm/embedding calls; every slot verified after timer; no warmup, barriers or memory instrumentation; sample result is iteration count, not guest output; no automatic subtraction or performance scoring"

func ValidateHarnessCalibration(p *Preparation, r *RunRequest) error {
	if p == nil || r == nil {
		return fmt.Errorf("missing harness calibration preparation")
	}
	w := p.Workload
	if r.Scenario != HarnessCalibrationScenario || p.Profile != "timing" || r.PhaseBarriers || r.Warmup != 0 || w.ABI != "core" || w.Reset != "stateless" || w.Oracle.Kind != "exact_u64" || w.Oracle.Float != nil || w.Command != nil || w.Vectors != nil || w.Density != nil || w.Checkpoint != nil || w.Continuation != nil || w.GuestDensity != nil || w.ProcessSnapshot != nil || w.SnapshotDensity != nil || w.Export == "" {
		return fmt.Errorf("unsupported harness calibration contract: timing, stateless core scalar, no warmup or instrumentation")
	}
	if r.Samples < 1 || r.Samples > 100000 || r.Operations < 1 || r.Operations > 1000000 {
		return fmt.Errorf("invalid harness calibration budget")
	}
	return nil
}
