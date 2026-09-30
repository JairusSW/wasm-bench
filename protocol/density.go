package protocol

import "fmt"

// DensityContract describes simultaneously held fresh instances. Separate
// engines deliberately include engine construction: compiling repeatedly in
// one engine does not establish that an internal code cache was bypassed.
type DensityContract struct {
	Instances int    `json:"instances"`
	Sharing   string `json:"sharing"`
}

func ValidateDensityWorkload(w Workload) error {
	if w.GuestDensity != nil || w.Checkpoint != nil {
		return fmt.Errorf("density contract cannot include checkpoint/guest density")
	}
	d := w.Density
	if d == nil || d.Instances < 1 || d.Instances > 128 {
		return fmt.Errorf("density requires between 1 and 128 simultaneously held instances")
	}
	if d.Sharing != "shared_module" && d.Sharing != "separate_engines" {
		return fmt.Errorf("unsupported density sharing policy")
	}
	if w.ABI != "core" || w.HostProfile != "" || w.Command != nil || w.Vectors != nil || w.Oracle.Kind != "exact_u64" || w.Oracle.Float != nil || w.Export == "" || w.Reset != "fresh_instance_per_sample" || w.WorkUnit != "instance_group" || w.Units != 1 {
		return fmt.Errorf("density requires fresh core scalar instance groups without host imports")
	}
	return nil
}

func ValidateDensity(p *Preparation, r *RunRequest) error {
	if p == nil || r == nil {
		return fmt.Errorf("missing density preparation or request")
	}
	if err := ValidateDensityWorkload(p.Workload); err != nil {
		return err
	}
	if r.Scenario != "density" || (p.Profile != "timing" && p.Profile != "memory") || (r.PhaseBarriers && p.Profile != "memory") {
		return fmt.Errorf("density requires timing or memory; barriers require memory")
	}
	if r.Samples < 1 || r.Samples > 100000 || r.Operations != 1 || r.Warmup != 0 {
		return fmt.Errorf("density requires 1 operation per sample, no warmup, and 1 to 100000 samples")
	}
	return nil
}

// ValidateDensityCycle preserves density's budgets but names the distinct
// retained-engine/module experiment. Ordinary density adapters must not accept
// this request through ValidateDensity and silently rebuild engines per sample.
func ValidateDensityCycle(p *Preparation, r *RunRequest) error {
	if r == nil || r.Scenario != "density-cycle" {
		return fmt.Errorf("missing density-cycle request")
	}
	copy := *r
	copy.Scenario = "density"
	return ValidateDensity(p, &copy)
}
