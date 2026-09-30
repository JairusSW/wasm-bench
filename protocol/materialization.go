package protocol

import "fmt"

// CompileMaterialization qualifies one synchronous Wasmtime compile return.
// Function ranges can include padding/constants. This is neither an executed
// tier observation nor a code creation/retirement or reclamation event.
type CompileMaterialization struct {
	Version           int    `json:"version"`
	Collector         string `json:"collector"`
	CollectorVersion  string `json:"collector_version"`
	Parser            string `json:"parser"`
	Scope             string `json:"scope"`
	Completion        string `json:"completion"`
	Quality           string `json:"quality"`
	ImportedFunctions uint32 `json:"imported_functions"`
	DefinedFunctions  uint32 `json:"defined_functions"`
	CompileElapsedNS  int64  `json:"compile_elapsed_ns"`
}

func ValidateMaterializedRun(p *Preparation, r *RunRequest) error {
	if p == nil || r == nil || p.Profile != "code" || r.Scenario != "compile-materialized" || r.Samples != 1 || r.Operations != 1 || r.Warmup != 0 || r.PhaseBarriers {
		return fmt.Errorf("materialized compile requires a code pass, one compile per launch, no warmup or phase barriers")
	}
	w := p.Workload
	if w.ABI != "core" || w.Reset != "stateless" || w.Export == "" || w.Oracle.Kind != "exact_u64" || w.Oracle.Float != nil || w.Command != nil || w.Vectors != nil || w.Density != nil || w.Checkpoint != nil || w.GuestDensity != nil || w.Input != nil || len(w.Oracle.Memory) != 0 || w.Initialize != "" {
		return fmt.Errorf("materialized compile requires a stateless exact-result core workload without compound fixtures")
	}
	return nil
}

func (m CompileMaterialization) Validate(image CodeImage) error {
	if m.Version != 1 || m.Collector != "Wasmtime/Module::functions" || m.CollectorVersion != "46.0.1" || m.Parser != "wasmparser/0.251.0" || m.Scope != "all_defined_function_ranges_at_synchronous_compile_return" || m.Completion != "documented_synchronous_api_with_verified_native_coverage" || m.Quality != "engine_reported" || m.CompileElapsedNS < 0 || m.DefinedFunctions > 10000 || uint64(m.ImportedFunctions)+uint64(m.DefinedFunctions) > 1<<32 || uint64(m.DefinedFunctions) != uint64(len(image.Functions)) {
		return fmt.Errorf("invalid compile materialization identity or coverage")
	}
	for _, f := range image.Functions {
		if uint64(f.WasmIndex) < uint64(m.ImportedFunctions) || uint64(f.WasmIndex) >= uint64(m.ImportedFunctions)+uint64(m.DefinedFunctions) {
			return fmt.Errorf("materialized range does not cover a defined function")
		}
	}
	return nil
}
