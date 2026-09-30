package protocol

import "testing"

func materializedFixture() CodeImage {
	c := codeFixture()
	c.Version, c.Backend, c.FunctionAttribution = 3, "cranelift", "engine_reported"
	c.Functions = []CodeFunction{{WasmIndex: 1, Length: 2, Tier: "cranelift"}, {WasmIndex: 2, Offset: 2, Length: 2, Tier: "cranelift"}}
	c.Materialization = &CompileMaterialization{Version: 1, Collector: "Wasmtime/Module::functions", CollectorVersion: "46.0.1", Parser: "wasmparser/0.251.0", Scope: "all_defined_function_ranges_at_synchronous_compile_return", Completion: "documented_synchronous_api_with_verified_native_coverage", Quality: "engine_reported", ImportedFunctions: 1, DefinedFunctions: 2, CompileElapsedNS: 10}
	return c
}

func TestMaterializedImageRequiresCompleteDefinedCoverage(t *testing.T) {
	c := materializedFixture()
	if err := c.Validate(c.ModuleSHA256); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"missing", "parser", "collector", "scope", "completion", "quality", "count", "import", "duplicate", "gap", "timer", "legacy", "budget", "overflow"} {
		t.Run(mode, func(t *testing.T) {
			c := materializedFixture()
			switch mode {
			case "missing":
				c.Materialization = nil
			case "parser":
				c.Materialization.Parser = "unknown"
			case "collector":
				c.Materialization.CollectorVersion = "50.0.0"
			case "scope":
				c.Materialization.Scope = "exported_functions_only"
			case "completion":
				c.Materialization.Completion = "inferred"
			case "quality":
				c.Materialization.Quality = "exact"
			case "count":
				c.Materialization.DefinedFunctions = 1
			case "import":
				c.Functions[0].WasmIndex = 0
			case "duplicate":
				c.Functions[1].WasmIndex = 1
			case "gap":
				c.Functions[1].WasmIndex = 3
			case "timer":
				c.Materialization.CompileElapsedNS = -1
			case "legacy":
				c.Version = 2
			case "budget":
				c.Materialization.DefinedFunctions = 10001
			case "overflow":
				c.Materialization.ImportedFunctions = ^uint32(0)
			}
			if c.Validate(c.ModuleSHA256) == nil {
				t.Fatal("invalid materialization accepted")
			}
		})
	}
}

func TestMaterializedRunRequiresDedicatedSingleCompile(t *testing.T) {
	p := Preparation{Profile: "code", Workload: Workload{ABI: "core", Reset: "stateless", Export: "run", Oracle: Oracle{Kind: "exact_u64"}}}
	r := RunRequest{Scenario: "compile-materialized", Samples: 1, Operations: 1}
	if err := ValidateMaterializedRun(&p, &r); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"timing", "warmup", "batch", "samples", "barrier", "abi", "reset", "oracle", "initializer"} {
		t.Run(mode, func(t *testing.T) {
			p, r := p, r
			switch mode {
			case "timing":
				p.Profile = "timing"
			case "warmup":
				r.Warmup = 1
			case "batch":
				r.Operations = 2
			case "samples":
				r.Samples = 2
			case "barrier":
				r.PhaseBarriers = true
			case "abi":
				p.Workload.ABI = "wasi-command"
			case "reset":
				p.Workload.Reset = "reuse"
			case "oracle":
				p.Workload.Oracle.Kind = "expected_trap"
			case "initializer":
				p.Workload.Initialize = "init"
			}
			if ValidateMaterializedRun(&p, &r) == nil {
				t.Fatal("unsupported boundary accepted")
			}
		})
	}
}
