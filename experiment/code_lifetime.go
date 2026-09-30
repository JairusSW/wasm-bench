package experiment

import (
	"fmt"
	"github.com/wasmbench/wasmbench/protocol"
	"slices"
)

func qualifiedCodeLifetimeRuntime(d *protocol.Description) bool {
	return d != nil && d.Runtime == "wasmtime" && d.Version == "46.0.1" && (d.Backend == "cranelift" || d.Backend == "winch") && d.Capabilities["can_code_lifetime"] && d.Configuration["code_lifetime"] == "wasmtime-executable-publication-v1" && d.Configuration["parallel_compilation"] == "false" && d.Configuration["cache"] == "disabled" && d.Configuration["code_lifetime_release"] == "drop_module_handles_then_store_then_engine"
}

func validateCodeLifetimeResult(root string, w protocol.Workload, l *protocol.CodeLifetime, image *protocol.CodeImage) error {
	if l == nil {
		return fmt.Errorf("missing code lifetime evidence")
	}
	if err := l.Validate(w.SHA256, image); err != nil {
		return err
	}
	if !slices.Equal([]uint64(l.BeforeDrop), []uint64(w.Oracle.Expected)) || !slices.Equal([]uint64(l.AfterDrop), []uint64(w.Oracle.Expected)) {
		return fmt.Errorf("code lifetime differs from exact workload result")
	}
	// Independently establish that every defined function is attributed; runtime
	// counts alone are not proof of complete Wasm input coverage.
	for i, f := range image.Functions {
		if f.WasmIndex != uint32(i) {
			return fmt.Errorf("code lifetime requires complete import-free function coverage")
		}
	}
	return validateMaterializationInput(root, w.SHA256, &protocol.CompileMaterialization{DefinedFunctions: uint32(len(image.Functions)), ImportedFunctions: 0})
}

func ValidateCodeLifetimeEvidence(root string, b Bundle) error {
	for _, t := range b.Trials {
		if t.CodeLifetime == nil {
			if t.Scenario == "code-lifetime" && t.Block >= 0 && t.Status == "ok" {
				return fmt.Errorf("trial %s omitted code lifetime evidence", t.ID)
			}
			continue
		}
		if t.Scenario != "code-lifetime" || t.Profile != "code" || t.Block < 0 || t.Status != "ok" || len(t.Samples) != 0 || len(t.AdapterSamples) != 0 || len(t.PhaseEvents) != 0 {
			return fmt.Errorf("code lifetime outside successful diagnostic trial")
		}
		var w protocol.Workload
		var d *protocol.Description
		for _, candidate := range b.Manifest.Lock.Workloads {
			if candidate.ID == t.Workload {
				w = candidate
				break
			}
		}
		for _, r := range b.Manifest.Lock.Runtimes {
			if r.ID == t.Runtime {
				d = r.Description
				break
			}
		}
		o := b.Manifest.Lock.Options
		if !qualifiedCodeLifetimeRuntime(d) || o.Profile != t.Profile || b.Manifest.Host.OS != "linux" || b.Manifest.Host.Arch != t.CodeLifetime.Architecture || b.Manifest.Host.PageSize <= 0 || uint64(b.Manifest.Host.PageSize) != t.CodeLifetime.PageSize || d.Backend != t.CodeLifetime.Backend {
			return fmt.Errorf("code lifetime differs from locked runtime, host or profile")
		}
		r := protocol.RunRequest{Scenario: t.Scenario, Samples: o.Samples, Operations: o.Operations, Warmup: o.Warmup, PhaseBarriers: o.PhaseBarriers, SustainedDurationNS: int64(o.SustainedDuration), SustainedPostCollection: o.SustainedPostCollection}
		if err := protocol.ValidateCodeLifetimeRun(&protocol.Preparation{Profile: t.Profile, Workload: w}, &r); err != nil {
			return err
		}
		admitted := false
		for _, a := range b.Admission {
			if a.SHA256 == w.SHA256 && a.Status == "validated" && a.ReportPath == "validation/"+w.SHA256+".json" {
				admitted = true
				break
			}
		}
		if !admitted {
			return fmt.Errorf("code lifetime omitted independent input admission")
		}
		if err := validateCodeLifetimeResult(root, w, t.CodeLifetime, t.CodeImage); err != nil {
			return err
		}
	}
	return nil
}
