package protocol

import "fmt"

// ValidateTrajectory requires a repeatable scalar contract. Resetting instances,
// commands and vector sequences represent different experiments.
func ValidateTrajectory(p *Preparation, r *RunRequest) error {
	if p == nil || r == nil {
		return fmt.Errorf("missing trajectory preparation")
	}
	w := p.Workload
	if r.Scenario != "trajectory" || p.Profile != "timing" || r.PhaseBarriers || w.ABI != "core" || w.Reset != "stateless" || (w.Oracle.Kind != "exact_u64" && w.Oracle.Kind != "float_bits_v1") || w.Command != nil || w.Vectors != nil || w.Export == "" {
		return fmt.Errorf("unsupported trajectory contract: requires timing, stateless core scalar calls without phase barriers")
	}
	if w.Oracle.Kind == "float_bits_v1" {
		if err := ValidateFloatWorkload(w); err != nil {
			return err
		}
	}
	if r.Samples < 1 || r.Samples > 100000 || r.Warmup < 0 || r.Warmup > 100000 || r.Operations < 1 || r.Operations > 1000000 {
		return fmt.Errorf("invalid trajectory batch")
	}
	return nil
}
