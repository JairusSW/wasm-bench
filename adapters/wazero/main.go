// Standalone embedding adapter; dependency graph is separate from the controller.
package main

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"slices"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/wasmbench/wasmbench/adapters/harness"
	"github.com/wasmbench/wasmbench/collectors"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/protocol"
)

var ctx = context.Background()

type adapter struct {
	prep        *protocol.Preparation
	wasm        []byte
	engine      wazero.Runtime
	compiled    wazero.CompiledModule
	instance    api.Module
	fn          api.Function
	interpreter bool
	barrier     func(protocol.PhaseEvent) error
}

func (a *adapter) newEngine() wazero.Runtime {
	c := wazero.NewRuntimeConfigCompiler()
	if a.interpreter {
		c = wazero.NewRuntimeConfigInterpreter()
	}
	return wazero.NewRuntimeWithConfig(ctx, c.WithCoreFeatures(api.CoreFeaturesV2).WithCloseOnContextDone(true))
}

// Verify the newly constructed engine, not a separately prepared runtime.
// Compilation, host registration, execution and release are outside init timing.
func (a *adapter) checkEngine(engine wazero.Runtime) error {
	probe := &adapter{prep: a.prep, wasm: a.wasm, engine: engine, interpreter: a.interpreter}
	if err := probe.registerHostImports(); err != nil {
		return err
	}
	compiled, err := engine.CompileModule(ctx, a.wasm)
	if err != nil {
		return err
	}
	defer compiled.Close(ctx)
	probe.compiled = compiled
	instance, err := probe.instantiate()
	if err != nil {
		return err
	}
	defer instance.Close(ctx)
	return probe.checkInstance(instance)
}
func (a *adapter) close() {
	if a.engine != nil {
		a.engine.Close(ctx)
	}
	a.engine = nil
	a.compiled = nil
	a.instance = nil
	a.fn = nil
}
func (a *adapter) instantiate() (api.Module, error) {
	return a.engine.InstantiateModule(ctx, a.compiled, wazero.NewModuleConfig().WithName("").WithStartFunctions().WithStdout(os.Stderr).WithStderr(os.Stderr))
}
func (a *adapter) prepare(p *protocol.Preparation) error {
	a.close()
	if p == nil {
		return fmt.Errorf("missing preparation")
	}
	if p.Workload.ABI != "core" && p.Workload.ABI != "wasi-command" && p.Workload.ABI != "wasi-reactor" && p.Workload.ABI != "emscripten" {
		return fmt.Errorf("unsupported ABI")
	}
	if p.Workload.ABI == "wasi-command" || p.Workload.Command != nil {
		if err := protocol.ValidateCommand(p.Workload); err != nil {
			return err
		}
	}
	if p.Workload.ABI == "wasi-reactor" {
		if err := protocol.ValidateReactor(p.Workload); err != nil {
			return err
		}
	}
	if p.Workload.Reset != "stateless" && p.Workload.Reset != "fresh_instance_per_sample" {
		return fmt.Errorf("unsupported reset policy")
	}
	b, err := os.ReadFile(p.Artifact)
	if err != nil {
		return err
	}
	if corpus.Hash(b) != p.ArtifactSHA256 {
		return fmt.Errorf("artifact digest mismatch")
	}
	a.prep = p
	a.wasm = b
	return nil
}
func (a *adapter) setupCompiled() error {
	if a.engine == nil {
		a.engine = a.newEngine()
		if err := a.registerHostImports(); err != nil {
			return err
		}
	}
	var err error
	if a.compiled == nil {
		a.compiled, err = a.engine.CompileModule(ctx, a.wasm)
		if err != nil {
			return err
		}
	}
	return nil
}
func (a *adapter) setup() error {
	if err := a.setupCompiled(); err != nil {
		return err
	}
	var err error
	if a.instance == nil {
		a.instance, err = a.instantiate()
		if err != nil {
			return err
		}
		if err = a.initialize(a.instance); err != nil {
			return err
		}
		a.fn = a.instance.ExportedFunction(a.prep.Workload.Export)
		if a.fn == nil {
			return fmt.Errorf("missing export")
		}
	}
	return nil
}
func (a *adapter) verify(result []uint64) bool {
	return a.verifyInstance(a.instance, result)
}
func (a *adapter) verifyInstance(instance api.Module, result []uint64) bool {
	if a.prep.Workload.Oracle.Kind == "float_bits_v1" || a.prep.Workload.Oracle.Float != nil {
		fn := instance.ExportedFunction(a.prep.Workload.Export)
		if fn == nil {
			return false
		}
		types := fn.Definition().ResultTypes()
		names := make([]string, len(types))
		for i, typ := range types {
			switch typ {
			case api.ValueTypeF32:
				names[i] = "f32"
			case api.ValueTypeF64:
				names[i] = "f64"
			default:
				names[i] = "unsupported"
			}
		}
		if a.prep.Workload.Oracle.VerifyFloat(result, names) != nil {
			return false
		}
	} else if a.prep.Workload.Oracle.Kind != "exact_u64" || !slices.Equal(result, a.prep.Workload.Oracle.Expected) {
		return false
	}
	var base uint64
	if name := a.prep.Workload.Oracle.OutputPointerExport; name != "" {
		f := instance.ExportedFunction(name)
		if f == nil {
			return false
		}
		v, err := f.Call(ctx)
		if err != nil || len(v) != 1 || v[0] > 0xffffffff {
			return false
		}
		base = v[0]
	}
	for _, check := range a.prep.Workload.Oracle.Memory {
		want, e := hex.DecodeString(check.Hex)
		if e != nil || !hasMemory(instance) {
			return false
		}
		offset := base + uint64(check.Offset)
		if offset > 0xffffffff || uint64(len(want)) > 0x100000000-offset {
			return false
		}
		got, ok := instance.Memory().Read(uint32(offset), uint32(len(want)))
		if !ok || !slices.Equal(got, want) {
			return false
		}
	}
	return true
}
func (a *adapter) checkInstance(m api.Module) error {
	if err := a.initialize(m); err != nil {
		return err
	}
	if a.prep.Workload.Vectors != nil {
		return a.checkVectors(m)
	}
	f := m.ExportedFunction(a.prep.Workload.Export)
	if f == nil {
		return fmt.Errorf("missing export")
	}
	result, err := f.Call(ctx, a.prep.Workload.Args...)
	if err != nil {
		return err
	}
	if !a.verifyInstance(m, result) {
		return fmt.Errorf("incorrect result")
	}
	return nil
}
func (a *adapter) initialize(m api.Module) error {
	if name := a.prep.Workload.Initialize; name != "" {
		f := m.ExportedFunction(name)
		if f == nil {
			return fmt.Errorf("missing initialization export %s", name)
		}
		_, e := f.Call(ctx)
		if e != nil {
			return e
		}
	}
	return a.applyInput(m)
}
func (a *adapter) applyInput(m api.Module) error {
	return a.prep.Workload.Input.Apply(func(name string) ([]uint64, error) {
		f := m.ExportedFunction(name)
		if f == nil {
			return nil, fmt.Errorf("missing input pointer export %s", name)
		}
		return f.Call(ctx)
	}, func(offset uint32, data []byte) bool { return hasMemory(m) && m.Memory().Write(offset, data) })
}
func (a *adapter) run(r *protocol.RunRequest) ([]protocol.Sample, error) {
	if a.prep == nil || r == nil {
		return nil, fmt.Errorf("prepare required")
	}
	if r.Scenario == protocol.HarnessCalibrationScenario {
		if err := protocol.ValidateHarnessCalibration(a.prep, r); err != nil {
			return nil, err
		}
		engine := a.newEngine()
		defer engine.Close(ctx)
		if err := a.checkEngine(engine); err != nil {
			return nil, err
		}
		return harness.Run(a.prep, r)
	}
	if a.prep.Workload.Continuation != nil || protocol.IsContinuationScenario(r.Scenario) {
		return a.runContinuation(r)
	}
	if r.Scenario == "sustained" {
		return a.runSustained(r)
	}
	if a.prep.Workload.GuestDensity != nil || r.Scenario == "guest-density" {
		return a.runGuestDensity(r)
	}
	if a.prep.Workload.Checkpoint != nil || protocol.IsCheckpointScenario(r.Scenario) {
		return a.runCheckpoint(r)
	}
	if a.prep.Workload.ABI == "wasi-reactor" {
		return a.runReactor(r)
	}
	if a.prep.Profile == "counters" {
		if err := protocol.ValidateCounterRun(a.prep, r); err != nil {
			return nil, err
		}
		if r.Scenario == "first-call" {
			return a.firstCallCounters(r)
		}
		if r.Scenario == "steady" {
			return a.steadyCounters(r)
		}
	}
	if r.Scenario == "density" || a.prep.Workload.Density != nil {
		return a.runDensity(r)
	}
	if a.prep.Workload.Oracle.Kind == "float_bits_v1" {
		if err := protocol.ValidateFloatRun(a.prep, r); err != nil {
			return nil, err
		}
	}
	if r.Scenario == "trajectory" {
		if err := protocol.ValidateTrajectory(a.prep, r); err != nil {
			return nil, err
		}
		a.close()
	}
	if a.prep.Workload.Oracle.Kind == "expected_trap" {
		return a.runTraps(r)
	}
	if r.Samples < 1 || r.Operations < 1 || r.Warmup < 0 {
		return nil, fmt.Errorf("invalid batch")
	}
	if r.Scenario == "app-init" {
		return a.runAppInit(r)
	}
	if a.prep.Workload.Command != nil {
		return a.runCommand(r)
	}
	if a.prep.Workload.Vectors != nil && (r.Scenario == "instantiate" || r.Scenario == "first-call") && r.PhaseBarriers {
		return a.runVectors(r)
	}
	if r.Scenario == "instantiate" && r.PhaseBarriers {
		return a.instantiatePhases(r)
	}
	if r.PhaseBarriers && r.Scenario != "teardown" {
		return a.compilePhases(r)
	}
	if r.PhaseBarriers && (a.prep.Profile != "memory" || a.barrier == nil) {
		return nil, fmt.Errorf("unsupported teardown barriers")
	}
	if a.prep.Workload.Vectors != nil {
		return a.runVectors(r)
	}
	if !slices.Contains([]string{"engine-init", "compile", "instantiate", "first-call", "steady", "trajectory", "teardown"}, r.Scenario) {
		return nil, fmt.Errorf("unsupported scenario")
	}
	if r.Scenario != "engine-init" && r.Scenario != "compile" {
		if err := a.setup(); err != nil {
			return nil, err
		}
	}
	// Validate in this process outside measurement as well. Controller uses a separate
	// sacrificial process before launching measured trials.
	if a.fn != nil && r.Scenario != "first-call" && r.Scenario != "trajectory" && r.Scenario != "teardown" {
		v, e := a.fn.Call(ctx, a.prep.Workload.Args...)
		if e != nil {
			return nil, e
		}
		if !a.verify(v) {
			return nil, fmt.Errorf("incorrect result")
		}
	}
	warmup := r.Warmup
	if r.Scenario != "steady" && r.Scenario != "trajectory" {
		warmup = 0
	}
	out := make([]protocol.Sample, 0, r.Samples+warmup)
	for i := -warmup; i < r.Samples; i++ {
		ops := r.Operations
		if r.Scenario == "first-call" || r.Scenario == "trajectory" || r.Scenario == "teardown" || a.prep.Workload.Reset == "fresh_instance_per_sample" {
			ops = 1
		}
		var result []uint64
		var err error
		var elapsed int64
		verified := true
		if r.Scenario == "first-call" || (r.Scenario == "steady" && a.prep.Workload.Reset == "fresh_instance_per_sample") {
			if a.instance != nil {
				a.instance.Close(ctx)
			}
			a.instance, err = a.instantiate()
			if err != nil {
				return nil, err
			}
			if err = a.initialize(a.instance); err != nil {
				return nil, err
			}
			a.fn = a.instance.ExportedFunction(a.prep.Workload.Export)
		}
		if r.Scenario == "teardown" {
			if err := a.setup(); err != nil {
				return nil, err
			}
			result, err = a.fn.Call(ctx, a.prep.Workload.Args...)
			if err != nil {
				return nil, err
			}
			if !a.verify(result) {
				return nil, fmt.Errorf("incorrect result before teardown")
			}
		}
		var before, after runtime.MemStats
		if r.PhaseBarriers {
			if err := a.barrier(protocol.PhaseEvent{SampleIndex: i, Stage: "before_teardown"}); err != nil {
				return nil, err
			}
		}
		if a.prep.Profile == "memory" {
			runtime.ReadMemStats(&before)
		}
		// For compile/instantiate, each release is outside its timed region.
		if r.Scenario == "steady" || r.Scenario == "trajectory" || r.Scenario == "first-call" {
			results := make([][]uint64, ops)
			start := time.Now()
			for j := 0; j < ops; j++ {
				results[j], err = a.fn.Call(ctx, a.prep.Workload.Args...)
				if err != nil {
					break
				}
			}
			elapsed = time.Since(start).Nanoseconds()
			if err == nil {
				for _, v := range results {
					verified = verified && a.verify(v)
				}
				result = results[len(results)-1]
			}
		} else {
			for j := 0; j < ops; j++ {
				switch r.Scenario {
				case "engine-init":
					start := time.Now()
					e := a.newEngine()
					elapsed += time.Since(start).Nanoseconds()
					err = a.checkEngine(e)
					e.Close(ctx)
				case "compile":
					// Wazero retains compiled code in the runtime after a module
					// handle closes. Give every sample its own empty runtime so
					// measured calls cannot hit the per-runtime module cache.
					a.close()
					a.engine = a.newEngine()
					if e := a.registerHostImports(); e != nil {
						return nil, e
					}
					start := time.Now()
					m, e := a.engine.CompileModule(ctx, a.wasm)
					elapsed += time.Since(start).Nanoseconds()
					err = e
					if m != nil {
						instance, e := a.engine.InstantiateModule(ctx, m, wazero.NewModuleConfig().WithName("").WithStartFunctions().WithStdout(os.Stderr).WithStderr(os.Stderr))
						err = e
						if instance != nil {
							err = a.checkInstance(instance)
							instance.Close(ctx)
						}
						m.Close(ctx)
					}
				case "instantiate":
					start := time.Now()
					m, e := a.instantiate()
					elapsed += time.Since(start).Nanoseconds()
					err = e
					if m != nil {
						err = a.checkInstance(m)
						m.Close(ctx)
					}
				case "teardown":
					start := time.Now()
					a.close()
					elapsed += time.Since(start).Nanoseconds()
				}
				if err != nil {
					break
				}
			}
		}
		if err != nil {
			return nil, err
		}
		if !verified {
			return nil, fmt.Errorf("incorrect result")
		}
		s := protocol.Sample{Index: i + warmup, Warmup: i < 0, ElapsedNS: elapsed, Operations: ops, SampleType: "batch_average", Verified: verified, Result: result}
		if ops == 1 {
			s.SampleType = "individual_operation"
		}
		if a.prep.Profile == "memory" {
			runtime.ReadMemStats(&after)
			window := "batch_including_outside_timer_releases"
			if r.Scenario == "engine-init" {
				window = "batch_including_untimed_usability_verification_and_release"
			}
			s.Observations = collectors.GoMemoryObservations(before, after, r.Scenario, window)
			if hasMemory(a.instance) {
				s.Observations = append(s.Observations, protocol.Observation{Metric: "guest.memory.logical", DefinitionVersion: 1, Value: protocol.Value(float64(a.instance.Memory().Size())), Unit: "bytes", Scope: "guest_linear_memory", Phase: r.Scenario, Collector: "wazero.Memory.Size", CollectorVersion: "1.12.0", Quality: "engine_reported", Profile: "memory", Status: "available", Denominator: "instance"})
			}
		}
		if r.PhaseBarriers {
			if err := a.barrier(protocol.PhaseEvent{SampleIndex: i, Stage: "torn_down"}); err != nil {
				return nil, err
			}
		}
		out = append(out, s)
	}
	return out, nil
}
func main() {
	a := &adapter{}
	for _, arg := range os.Args[1:] {
		if arg == "--interpreter" {
			a.interpreter = true
		}
	}
	defer a.close()
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 4096), 4<<20)
	enc := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var req protocol.Request
		resp := protocol.Response{Version: 1, Status: "ok"}
		err := json.Unmarshal(scanner.Bytes(), &req)
		resp.ID = req.ID
		if err == nil && req.Version != 1 {
			err = fmt.Errorf("unsupported protocol version")
		}
		if err == nil {
			switch req.Method {
			case "describe":
				backend := "compiler"
				if a.interpreter {
					backend = "interpreter"
				}
				build := "unknown"
				if info, ok := debug.ReadBuildInfo(); ok {
					build = info.String()
				}
				resp.Description = &protocol.Description{Runtime: "wazero", Version: "1.12.0", Backend: backend, Embedding: "Go API", Build: build, Configuration: map[string]string{"compilation_cache": "disabled", "compile_policy": "fresh uncached module per operation; empty runtime prepared outside timer; verification, module close and instance close outside timer; no untimed retained module", "context_termination": "enabled", "start_functions": "explicit_disabled", "backend": backend}, Capabilities: map[string]bool{"can_compile_separately": true, "can_instantiate_separately": true, "can_disable_code_cache": true, "can_measure_host_allocations": true, "can_export_native_code": !a.interpreter, "can_observe_tiers": false, "can_snapshot": false}, Scenarios: []string{"engine-init", "compile", "instantiate", "first-call", "steady", "trajectory", "teardown"}, ABIs: []string{"core"}, Features: []string{"mvp", "bulk-memory", "simd", "reference-types", "multi-value"}}
				resp.Description.Scenarios = append(resp.Description.Scenarios, "app-init")
				resp.Description.Scenarios = append(resp.Description.Scenarios, protocol.HarnessCalibrationScenario)
				resp.Description.Configuration["harness_calibration_policy"] = protocol.HarnessCalibrationPolicy
				resp.Description.Configuration["engine_init_policy"] = "time configured runtime construction only; register imports, compile resident workload bytes, instantiate and verify behavior using each measured runtime after timer stop; release outside timer; Go allocation snapshots include untimed usability verification and release; no forced reclamation"
				if !a.interpreter {
					resp.Description.Capabilities["can_native_continuation"] = true
					resp.Description.Scenarios = append(resp.Description.Scenarios, protocol.ContinuationScenarios()...)
					resp.Description.Configuration["native_continuation_protocol"] = protocol.NativeContinuationMode
					resp.Description.Configuration["native_continuation_scope"] = "execution stack only; no memory/global/whole-instance/COW restoration; one capture restored once in its active invocation; restore clock ends at resumed guest host marker; fresh instances; module/engine retained; no forced GC"
				}
				resp.Description.Scenarios = append(resp.Description.Scenarios, "density")
				resp.Description.Capabilities["can_density"] = true
				resp.Description.Capabilities["can_sustained_execution"] = true
				resp.Description.Capabilities["can_sustained_post_collection"] = true
				resp.Description.Configuration["sustained_post_collection_policy"] = "optional memory-only diagnostic: one explicit runtime.GC after logical engine/module/instance close and reference drop; separate monotonic bracket includes allocator snapshots; adapter and sample evidence remain alive; Go heap only, not physical reclamation or leak qualification"
				resp.Description.Scenarios = append(resp.Description.Scenarios, "sustained")
				resp.Description.Configuration["sustained_policy"] = "fixed batch count on one fresh initialized retained instance/engine/module; invocation one retained; only declared warmup; all results verified outside timers; monotonic session clocks; target is cumulative non-warmup API time, not wall/service throughput; memory snapshots bracket operations, excluding verification; final engine/module/instance close outside timers with no forced GC or physical-reclamation claim"
				resp.Description.Capabilities["can_guest_checkpoint"] = true
				resp.Description.Capabilities["can_guest_density"] = true
				resp.Description.Scenarios = append(resp.Description.Scenarios, "guest-density")
				resp.Description.Configuration["guest_density_policy"] = "exact fixed memory plus scalar fixture; fresh simultaneous groups sharing one retained engine/module; fresh_initialize fills all memory or eager_guest_restore copies a shared independent guest-state payload; optional first write changes one byte and scalar; source initialization/save/isolation and compile excluded; timer includes instantiate/start and initialize or restore and optional write; all instance checksums/full state verification excluded; no whole-instance snapshot, COW, untouched-page, forced GC or reclamation claim"
				resp.Description.Scenarios = append(resp.Description.Scenarios, protocol.CheckpointScenarios()...)
				resp.Description.Configuration["guest_checkpoint_policy"] = "eager-memory-scalar-v1 exact generated fixture only: entire fixed memory and one mutable i32; no imports/tables/passive segments/GC objects/active stack; creation includes allocation and copy; restore copies into a separately instantiated target (instantiation excluded); first write is ordinary eager-copy guest call, NOT COW; execution is one full-memory checksum call; setup and verification excluded; source mutated after save to verify independence; engine/module retained, source/target fresh each sample; no forced GC or whole-instance snapshot claim"
				resp.Description.Capabilities["can_counter_compile"] = true
				resp.Description.Capabilities["can_counter_instantiate"] = true
				resp.Description.Capabilities["can_counter_first-call"] = true
				resp.Description.Capabilities["can_counter_steady"] = true
				resp.Description.Capabilities["can_profile_go_cpu_steady"] = a.interpreter
				resp.Description.Configuration["cpu_profile_policy"] = "interpreter only; sampled Go process CPU over entire run request including setup, warmup and verification; no guest-only attribution or native stack-unwinding claim; raw runtime/pprof gzip; 16 MiB cap; no forced GC"
				resp.Description.Configuration["counter_steady_policy"] = "stateless exact scalar; fresh engine/module/initialized instance per request retained across explicit warmup and measured batches; every call result verified after each batch; no hidden pre-call or memory snapshots; result slice container prepared before barrier; embedding Call allocations and barrier transport included; raw counts per batch, not per invocation"
				resp.Description.Configuration["counter_first_call_policy"] = "fresh initialized instance per sample; engine/module retained; exactly one requested call, no hidden verification pre-call; setup/input/export resolution before counter window, verification/result copy/release after; no memory instrumentation"
				resp.Description.Configuration["counter_policy"] = "core exact scalar compile/instantiate phase handshakes; one operation, no warmup; no allocator snapshots; perf collected externally over cgroup barrier window including transport/background work; verification and release excluded"
				resp.Description.Configuration["density_policy"] = "fresh simultaneous group; shared_module uses one engine/module; separate_engines compiles once per fresh engine; timer includes engine construction, compile, instantiate/start, initialization, input and workload invocation; verification and release excluded; no forced GC"
				resp.Description.Capabilities["can_verify_invocation_traps"] = true
				resp.Description.Capabilities["can_verify_float_bits_v1"] = true
				resp.Description.Capabilities["can_float_phases"] = true
				resp.Description.Capabilities["can_float_teardown"] = true
				resp.Description.Capabilities["can_float_trajectory"] = true
				resp.Description.Capabilities["can_measure_invocation_traps"] = true
				resp.Description.Configuration["core_features"] = "api.CoreFeaturesV2"
				resp.Description.ValidatorFeatures = validatorFeaturePolicy()
				resp.Description.Configuration["teardown_policy"] = "verify each fresh instance before timing runtime close (instance/module/engine); no forced GC"
				resp.Description.Configuration["command_teardown_policy"] = "after verified command termination close remaining instance/module/stdin/runtime resources; proc_exit may already close instance during execution; fixture cleanup and capture-buffer reclamation excluded; no forced GC"
				resp.Description.PhaseBarrierScenarios = []string{"compile", "teardown", "app-init", "instantiate", "density", "first-call", "steady"}
				resp.Description.PhaseBarrierScenarios = append(resp.Description.PhaseBarrierScenarios, "guest-density")
				resp.Description.PhaseBarrierScenarios = append(resp.Description.PhaseBarrierScenarios, protocol.CheckpointScenarios()...)
				if !a.interpreter {
					resp.Description.PhaseBarrierScenarios = append(resp.Description.PhaseBarrierScenarios, protocol.ContinuationScenarios()...)
				}
				resp.Description.Configuration["instantiate_release_policy"] = "close verified instance; compiled module and engine retained; no forced GC"
				resp.Description.Configuration["app_init_release_policy"] = "close verified instance; compiled module and engine retained; no forced GC"
				resp.Description.Capabilities["can_run_vectors"] = true
				resp.Description.ABIs = append(resp.Description.ABIs, "wasi-command")
				resp.Description.ABIs = append(resp.Description.ABIs, "emscripten")
				resp.Description.ABIs = append(resp.Description.ABIs, "wasi-reactor")
				resp.Description.Capabilities["can_run_reactors"] = true
				resp.Description.Configuration["wasi_reactor_host"] = "wazero Preview 1 1.12.0; no preopens, argv, env or network; empty stdin; zero random; default synthetic clocks; nonempty stdout/stderr writes rejected; _initialize exactly once per fresh instance before input/calls; Wasm start included in instantiation; steady stateless instance retained without hidden pre-call"
				resp.Description.Capabilities["can_run_commands"] = true
				resp.Description.Configuration["emscripten_host"] = "emscripten-stdio-v1; bounded stdout/stderr and pinned stdin; deterministic clocks/randomness; filesystem syscalls fail closed; argv marshaled through stackAlloc; fresh instance per sample"
				resp.Description.Capabilities["can_command_compile_phases"] = true
				resp.Description.Capabilities["can_command_instantiate_phases"] = true
				resp.Description.Capabilities["can_command_first-call_phases"] = true
				resp.Description.Configuration["command_lifecycle_phases_policy"] = "compile/instantiate/first-call memory API windows; prepared imports and capture buffers excluded; instantiate includes Wasm start but excludes Emscripten constructors/argv; first-call includes bounded capture and host calls; output/exit verification before release; fresh instance each sample; module/engine retained except compile module; no forced reclamation"
				resp.Description.Configuration["wasi_host"] = "wazero-wasi_snapshot_preview1-1.12.0; readonly fixture root; empty env; zero random; default synthetic clocks; fresh stdin and instance; bounded stream capture included in call"
				resp.Description.Capabilities["can_vector_compile_phases"] = true
				resp.Description.Capabilities["can_vector_instantiate_phases"] = true
				resp.Description.Capabilities["can_vector_first-call_phases"] = true
				resp.Description.Configuration["vector_first_call_phases_policy"] = protocol.VectorFirstCallPhasesPolicy
				resp.Description.Configuration["vector_instantiate_phases_policy"] = "fresh instance; prepared module/engine retained; instantiation includes Wasm start; explicit initialization, ordered vector calls, oracle verification and release outside API window; no forced GC"
				resp.Description.Capabilities["can_vector_teardown_phases"] = true
				resp.Description.Capabilities["can_command_teardown_phases"] = true
				if !a.interpreter {
					resp.Description.Configuration["code_export"] = "separate code-profile compilation with an empty temporary file cache; pinned Wazevo 1.12.0 native segment and CRC; excludes serialized metadata, separate shared helpers and entry preambles; no instruction-only attribution; timing compilation cache remains disabled"
				}
				resp.Description.PhaseReleasePolicy = "close measured instance and compiled module; engine retained; no forced GC"
				if !a.interpreter {
					resp.Description.PhaseReleasePolicy += "; native continuation stages discard capture after active invocation and close verified instance; shared compiled module/engine retained until batch close"
				}
			case "prepare":
				err = a.prepare(req.Prepare)
			case "run":
				a.barrier = func(event protocol.PhaseEvent) error { return protocol.Barrier(scanner, enc, req.ID, event) }
				if a.prep != nil && a.prep.Profile == "profiling" {
					resp.Samples, resp.CPUProfile, err = a.runProfile(req.Run)
				} else {
					resp.Samples, err = a.run(req.Run)
				}
			case "inspect":
				resp.CodeImage, resp.Diagnostics, err = a.inspectCode()
			case "close":
				a.close()
			default:
				err = fmt.Errorf("unknown method %q", req.Method)
			}
		}
		if err != nil {
			resp.Status = "error"
			resp.Reason = err.Error()
		}
		if e := enc.Encode(resp); e != nil {
			return
		}
		if req.Method == "close" {
			return
		}
	}
}
