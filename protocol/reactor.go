package protocol

import (
	"fmt"
	"slices"
)

const WASIReactorProfile = "wasi-preview1-reactor-noio-v1"

// Reactors explicitly initialize once before calling their workload export.
// This profile has no ambient filesystem, environment, argv, stdin or network;
// nonempty stdout/stderr writes are rejected, not silently discarded.
func ValidateReactor(w Workload) error {
	if w.ABI != "wasi-reactor" || w.HostProfile != WASIReactorProfile || w.Initialize != "_initialize" || w.Export == "" || w.Export == "_initialize" || w.Export == "_start" || w.Command != nil || w.Vectors != nil || w.Density != nil || w.Oracle.Kind != "exact_u64" || w.Oracle.Float != nil || w.Oracle.ExpectedTrap != "" || !slices.Contains([]string{"stateless", "fresh_instance_per_sample"}, w.Reset) {
		return fmt.Errorf("unsupported WASI reactor contract: explicit _initialize, scalar oracle and no-I/O Preview 1 host profile required")
	}
	return nil
}

func ValidateReactorRun(p *Preparation, r *RunRequest) error {
	if p == nil || r == nil {
		return fmt.Errorf("reactor preparation and run required")
	}
	if err := ValidateReactor(p.Workload); err != nil {
		return err
	}
	if !slices.Contains([]string{"timing", "memory"}, p.Profile) || r.PhaseBarriers || !slices.Contains([]string{"compile", "instantiate", "app-init", "first-call", "steady", "cold-process", "teardown"}, r.Scenario) {
		return fmt.Errorf("unsupported reactor scenario/profile; phase barriers, counters, code and profiling are not qualified")
	}
	if r.Samples < 1 || r.Samples > 100000 || r.Warmup < 0 || r.Warmup > 100000 || r.Operations < 1 || r.Operations > 1000000 {
		return fmt.Errorf("reactor batch exceeds bounded samples/operations/warmup contract")
	}
	return nil
}
