package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	wago "github.com/wago-org/wago"
	"github.com/wasmbench/wasmbench/adapters/harness"
	"github.com/wasmbench/wasmbench/collectors"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/protocol"
	"os"
	"runtime"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	component "github.com/wago-org/component-model"
	wagoplugin "github.com/wago-org/wago/plugin"
)

var sourceRevision = "unknown"

type adapter struct {
	barrier          func(protocol.PhaseEvent) error
	prep             *protocol.Preparation
	wasm             []byte
	compileConfig    *wago.RuntimeConfig
	compiled         *wago.Compiled
	instance         *wago.Instance
	fn               *wago.WasmFunc
	imports          *wago.Imports
	hostRuntime      *wago.Runtime
	hostIdentity     *wago.HostFuncRef
	componentRuntime *wago.Runtime
	componentRef     *wagoplugin.Ref[component.Service]
	componentCache   *component.CompileCache
}

func (a *adapter) close() {
	if a.instance != nil {
		a.instance.Close()
	}
	if a.compiled != nil {
		a.compiled.Close()
	}
	a.instance = nil
	a.compiled = nil
	a.fn = nil
}
func (a *adapter) closeAll() {
	a.close()
	if a.componentCache != nil {
		_ = a.componentCache.Close(context.Background())
		a.componentCache = nil
	}
	a.componentRef = nil
	if a.componentRuntime != nil {
		_ = a.componentRuntime.Close()
		a.componentRuntime = nil
	}
	if a.hostIdentity != nil {
		a.hostIdentity.Close()
		a.hostIdentity = nil
	}
	if a.hostRuntime != nil {
		a.hostRuntime.Close()
		a.hostRuntime = nil
	}
	a.imports = nil
}
func (a *adapter) instantiate(c *wago.Compiled) (*wago.Instance, error) {
	if a.hostRuntime != nil {
		mod, err := a.hostRuntime.Module(c)
		if err != nil {
			return nil, err
		}
		defer mod.Close()
		return a.hostRuntime.Instantiate(context.Background(), mod, wago.WithImports(a.imports))
	}
	options := wago.InstantiateOptions{Imports: a.imports}
	return wago.Instantiate(c, options)
}
func (a *adapter) initialize(m *wago.Instance) error {
	if name := a.prep.Workload.Initialize; name != "" {
		f, e := m.WasmFunc(name)
		if e != nil {
			return e
		}
		_, e = f.Invoke()
		if e != nil {
			return e
		}
	}
	return a.applyInput(m)
}
func (a *adapter) applyInput(m *wago.Instance) error {
	return a.prep.Workload.Input.Apply(func(name string) ([]uint64, error) {
		f, err := m.WasmFunc(name)
		if err != nil {
			return nil, err
		}
		return f.Invoke()
	}, m.Write)
}
func (a *adapter) fresh() error {
	if a.instance != nil {
		a.instance.Close()
	}
	var e error
	a.instance, e = a.instantiate(a.compiled)
	if e != nil {
		return e
	}
	if e = a.initialize(a.instance); e != nil {
		return e
	}
	a.fn, e = a.instance.WasmFunc(a.prep.Workload.Export)
	return e
}
func (a *adapter) setup() error {
	if a.compiled == nil {
		var e error
		a.compiled, e = wago.Compile(a.compileConfig, a.wasm)
		if e != nil {
			return e
		}
	}
	if a.instance == nil {
		return a.fresh()
	}
	return nil
}
func (a *adapter) verify(m *wago.Instance, v []uint64) error {
	return a.verifyCompiled(a.compiled, m, v)
}
func (a *adapter) verifyCompiled(c *wago.Compiled, m *wago.Instance, v []uint64) error {
	if a.prep.Workload.Oracle.Kind == "float_bits_v1" || a.prep.Workload.Oracle.Float != nil {
		if c == nil {
			return fmt.Errorf("float verification requires compiled signature")
		}
		_, types, err := c.Signature(a.prep.Workload.Export)
		if err != nil {
			return err
		}
		names := make([]string, len(types))
		for i, typ := range types {
			switch typ {
			case wago.ValF32:
				names[i] = "f32"
			case wago.ValF64:
				names[i] = "f64"
			default:
				names[i] = "unsupported"
			}
		}
		if err = a.prep.Workload.Oracle.VerifyFloat(v, names); err != nil {
			return err
		}
	} else if a.prep.Workload.Oracle.Kind != "exact_u64" || !slices.Equal(v, a.prep.Workload.Oracle.Expected) {
		return fmt.Errorf("incorrect result: got %v want %v", v, a.prep.Workload.Oracle.Expected)
	}
	var base uint64
	if name := a.prep.Workload.Oracle.OutputPointerExport; name != "" {
		f, err := m.WasmFunc(name)
		if err != nil {
			return fmt.Errorf("incorrect result: output pointer: %w", err)
		}
		v, err := f.Invoke()
		if err != nil || len(v) != 1 || v[0] > 0xffffffff {
			return fmt.Errorf("incorrect result: invalid output pointer")
		}
		base = v[0]
	}
	for _, check := range a.prep.Workload.Oracle.Memory {
		want, e := hex.DecodeString(check.Hex)
		if e != nil {
			return e
		}
		offset := base + uint64(check.Offset)
		if offset > 0xffffffff || uint64(len(want)) > 0x100000000-offset {
			return fmt.Errorf("incorrect result: output pointer overflow")
		}
		got, ok := m.Read(uint32(offset), uint32(len(want)))
		if !ok || !slices.Equal(got, want) {
			return fmt.Errorf("incorrect result: memory oracle mismatch")
		}
	}
	return nil
}
func (a *adapter) checkModule(c *wago.Compiled) error {
	m, e := a.instantiate(c)
	if e != nil {
		return e
	}
	defer m.Close()
	if e = a.initialize(m); e != nil {
		return e
	}
	if a.prep.Workload.Vectors != nil {
		return a.checkVectors(m)
	}
	f, e := m.WasmFunc(a.prep.Workload.Export)
	if e != nil {
		return e
	}
	v, e := f.Invoke(a.prep.Workload.Args...)
	if e != nil {
		return e
	}
	return a.verifyCompiled(c, m, v)
}
func (a *adapter) run(r *protocol.RunRequest) ([]protocol.Sample, error) {
	if a.prep == nil || r == nil {
		return nil, fmt.Errorf("prepare required")
	}
	if r.Scenario == protocol.HarnessCalibrationScenario {
		if err := protocol.ValidateHarnessCalibration(a.prep, r); err != nil {
			return nil, err
		}
		if err := a.setup(); err != nil {
			return nil, err
		}
		if err := a.checkModule(a.compiled); err != nil {
			return nil, err
		}
		return harness.Run(a.prep, r)
	}
	if a.prep.Workload.Command != nil || a.prep.Workload.ABI == "component" {
		return a.runPluginFeature(r)
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
	if r.Samples < 1 || r.Samples > 100000 || r.Operations < 1 || r.Operations > 1000000 || r.Warmup < 0 {
		return nil, fmt.Errorf("invalid batch")
	}
	if r.Scenario == "app-init" {
		return a.runAppInit(r)
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
	if !slices.Contains([]string{"compile", "instantiate", "first-call", "steady", "trajectory", "teardown", "aot-produce", "aot-load"}, r.Scenario) {
		return nil, fmt.Errorf("unsupported scenario")
	}
	if e := a.setup(); e != nil {
		return nil, e
	}
	var serialized []byte
	if r.Scenario == "aot-load" {
		var e error
		serialized, e = a.compiled.MarshalBinary()
		if e != nil {
			return nil, e
		}
	}
	warmup := 0
	if r.Scenario == "steady" || r.Scenario == "trajectory" {
		warmup = r.Warmup
	}
	out := make([]protocol.Sample, 0, r.Samples+warmup)
	for i := -warmup; i < r.Samples; i++ {
		ops := r.Operations
		if r.Scenario == "first-call" || r.Scenario == "trajectory" || r.Scenario == "teardown" || a.prep.Workload.Reset == "fresh_instance_per_sample" {
			ops = 1
		}
		if r.Scenario == "first-call" || (r.Scenario == "steady" && a.prep.Workload.Reset == "fresh_instance_per_sample") {
			if e := a.fresh(); e != nil {
				return nil, e
			}
		}
		var result []uint64
		if r.Scenario == "teardown" {
			if e := a.setup(); e != nil {
				return nil, e
			}
			v, e := a.fn.Invoke(a.prep.Workload.Args...)
			if e != nil {
				return nil, e
			}
			result = append([]uint64(nil), v...)
			if e = a.verify(a.instance, result); e != nil {
				return nil, e
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
		var elapsed int64
		if r.Scenario == "steady" || r.Scenario == "trajectory" || r.Scenario == "first-call" {
			results := make([][]uint64, ops)
			for j := range results {
				results[j] = make([]uint64, 0, len(a.prep.Workload.Oracle.Expected))
			}
			var e error
			start := time.Now()
			for j := 0; j < ops; j++ {
				v, err := a.fn.Invoke(a.prep.Workload.Args...)
				if err != nil {
					e = err
					break
				}
				// Wago invalidates its instance-owned result slice on the next
				// invocation, including an oracle's output-pointer export.
				results[j] = append(results[j], v...)
			}
			elapsed = time.Since(start).Nanoseconds()
			if e != nil {
				return nil, e
			}
			for _, v := range results {
				if e = a.verify(a.instance, v); e != nil {
					return nil, e
				}
			}
			result = results[len(results)-1]
		} else {
			for j := 0; j < ops; j++ {
				switch r.Scenario {
				case "compile":
					start := time.Now()
					c, e := wago.Compile(a.compileConfig, a.wasm)
					elapsed += time.Since(start).Nanoseconds()
					if e != nil {
						return nil, e
					}
					e = a.checkModule(c)
					c.Close()
					if e != nil {
						return nil, e
					}
				case "instantiate":
					start := time.Now()
					m, e := a.instantiate(a.compiled)
					elapsed += time.Since(start).Nanoseconds()
					if e != nil {
						return nil, e
					}
					if e = a.initialize(m); e == nil {
						var f *wago.WasmFunc
						f, e = m.WasmFunc(a.prep.Workload.Export)
						if e == nil {
							var v []uint64
							v, e = f.Invoke(a.prep.Workload.Args...)
							if e == nil {
								e = a.verify(m, v)
							}
						}
					}
					m.Close()
					if e != nil {
						return nil, e
					}
				case "teardown":
					start := time.Now()
					a.close()
					elapsed += time.Since(start).Nanoseconds()
				case "aot-produce":
					start := time.Now()
					c, e := wago.Compile(a.compileConfig, a.wasm)
					if e != nil {
						return nil, e
					}
					artifact, e := c.MarshalBinary()
					elapsed += time.Since(start).Nanoseconds()
					c.Close()
					if e != nil {
						return nil, e
					}
					// Trust only immutable bytes produced locally by this compile.
					// Round-trip and behavioral verification are outside production timing.
					restored, e := wago.LoadTrustedArtifact(artifact)
					if e != nil {
						return nil, e
					}
					e = a.checkModule(restored)
					restored.Close()
					if e != nil {
						return nil, e
					}
				case "aot-load":
					start := time.Now()
					// serialized was generated by a.compiled.MarshalBinary above;
					// no caller-supplied native artifact is accepted by this adapter.
					c, e := wago.LoadTrustedArtifact(serialized)
					elapsed += time.Since(start).Nanoseconds()
					if e != nil {
						return nil, e
					}
					e = a.checkModule(c)
					c.Close()
					if e != nil {
						return nil, e
					}
				}
			}
		}
		s := protocol.Sample{Index: i + warmup, Warmup: i < 0, ElapsedNS: elapsed, Operations: ops, SampleType: "batch_average", Verified: true, Result: result}
		if ops == 1 {
			s.SampleType = "individual_operation"
		}
		if a.prep.Profile == "memory" {
			runtime.ReadMemStats(&after)
			s.Observations = collectors.GoMemoryObservations(before, after, r.Scenario, "batch_including_untimed_verification_and_release")
			if a.instance != nil && a.instance.Memory() != nil {
				s.Observations = append(s.Observations, protocol.Observation{Metric: "guest.memory.logical", DefinitionVersion: 1, Value: protocol.Value(float64(len(a.instance.Memory().UnsafeBytes()))), Unit: "bytes", Scope: "guest_linear_memory", Phase: r.Scenario, Collector: "wago.Memory", CollectorVersion: sourceRevision, Quality: "engine_reported", Profile: "memory", Status: "available", Denominator: "instance"})
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
	defer a.closeAll()
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 4096), 4<<20)
	enc := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var req protocol.Request
		resp := protocol.Response{Version: 1, Status: "ok"}
		e := json.Unmarshal(scanner.Bytes(), &req)
		resp.ID = req.ID
		if e == nil && req.Version != 1 {
			e = fmt.Errorf("unsupported protocol")
		}
		if e == nil {
			switch req.Method {
			case "describe":
				build := "unknown"
				if info, ok := debug.ReadBuildInfo(); ok {
					build = info.String()
				}
				resp.Description = &protocol.Description{Runtime: "wago", Version: sourceRevision, Backend: "railshot", Embedding: "Go API / Wago WASI and Component Model plugin implementations", Build: build, Configuration: map[string]string{"runtime_config": "Wago defaults; maxModuleBytes is set to the exact prepared artifact size per workload", "compile_module_byte_limit": "exact workload artifact size; Wago's 64 MiB default does not reject larger benchmark artifacts", "module_cache": "disabled", "wasi_preview1": "wago-org/wasi v0.3.1 P1 provider host imports; explicit readonly fixture mount and guest argv/streams", "wasi_preview2": "wago-org/wasi v0.3.1 P2 host options through the Component Model plugin service", "component_model": "wago-org/component-model v0.1.6 loaded through Wago plugin contract", "component_compile_cache": "retained only within a feature timing request", "source_revision": sourceRevision}, Capabilities: map[string]bool{"can_compile_separately": true, "can_instantiate_separately": true, "can_disable_code_cache": true, "can_measure_host_allocations": true, "can_export_native_code": false, "can_snapshot": false, "can_observe_tiers": false, "can_run_commands": true, "can_run_component_commands": true, "can_component_command_lifecycle": true, "can_component_u64_calls_v1": true}, Scenarios: []string{"compile", "instantiate", "first-call", "steady", "trajectory", "teardown", "aot-produce", "aot-load"}, ABIs: []string{"core", "wasi-command", "component"}, Features: []string{"mvp", "bulk-memory", "simd", "wasi-preview1", "wasi-preview2", "component-model"}}
				resp.Description.Scenarios = append(resp.Description.Scenarios, "app-init")
				resp.Description.Scenarios = append(resp.Description.Scenarios, protocol.HarnessCalibrationScenario)
				resp.Description.Configuration["harness_calibration_policy"] = protocol.HarnessCalibrationPolicy
				resp.Description.Configuration["aot_policy"] = "produce: resident Wasm bytes through Compile and MarshalBinary; load only those locally produced immutable bytes via LoadTrustedArtifact and verify behavior outside production timer; load: LoadTrustedArtifact of resident bytes generated by this adapter before sampling, behavior verified outside timer; no arbitrary native artifact ingestion; release outside timers"
				resp.Description.Scenarios = append(resp.Description.Scenarios, "density")
				resp.Description.Capabilities["can_density"] = true
				resp.Description.Capabilities["can_counter_compile"] = true
				resp.Description.Capabilities["can_counter_instantiate"] = true
				resp.Description.Capabilities["can_counter_first-call"] = true
				resp.Description.Capabilities["can_counter_steady"] = true
				resp.Description.Configuration["execution_result_capture"] = "copy each instance-owned Invoke result into preallocated storage before next call; capture cost included in execution window; oracle verification afterward"
				resp.Description.Configuration["counter_steady_policy"] = "stateless exact scalar; fresh compiled module/initialized instance per request retained across explicit warmup and measured batches; every call result copied into preallocated storage inside window and verified afterward; no hidden pre-call or memory snapshots; raw counts per batch include result capture and barrier transport"
				resp.Description.Configuration["counter_first_call_policy"] = "fresh initialized instance per sample; compiled module retained; exactly one requested call, no hidden verification pre-call; setup/input/export resolution before counter window, verification/result copy/release after; no memory instrumentation"
				resp.Description.Configuration["counter_policy"] = "core exact scalar compile/instantiate phase handshakes; one operation, no warmup; no allocator snapshots; perf collected externally over cgroup barrier window including transport/background work; verification and release excluded"
				resp.Description.Configuration["density_policy"] = "Runtime.Compile/Instantiate: fresh simultaneous group; shared_module one runtime/module; separate_engines fresh runtime and compilation per instance; timer includes engine construction, compile, instantiate/start, initialization, input and workload invocation; verification and release excluded; release waits Runtime.CloseContext and Module.Close; no forced GC"
				resp.Description.ValidatorFeatures = &protocol.ValidatorFeaturePolicy{Namespace: "wasmparser/0.251.0", Evidence: "Pinned Wago release implements standardized exception handling, but not the legacy EXCEPTIONS proposal; proposal continuation types are also unavailable", Supported: map[string]bool{"EXCEPTIONS": false, "STACK_SWITCHING": false}}
				resp.Description.Capabilities["can_export_native_code"] = true
				resp.Description.Capabilities["can_verify_float_bits_v1"] = true
				resp.Description.Capabilities["can_float_phases"] = true
				resp.Description.Capabilities["can_float_teardown"] = true
				resp.Description.Capabilities["can_float_trajectory"] = true
				resp.Description.Configuration["native_code_export"] = "raw native image v1; mixed instructions/wrappers/embedded data; function attribution unavailable; maximum 16 MiB"
				resp.Description.Capabilities["can_verify_invocation_traps"] = true
				resp.Description.Capabilities["can_measure_invocation_traps"] = true
				resp.Description.Configuration["teardown_policy"] = "verify each fresh instance before timing instance and compiled-module close; host runtime retained; no forced GC"
				resp.Description.PhaseBarrierScenarios = []string{"compile", "teardown", "app-init", "instantiate", "density", "first-call", "steady"}
				resp.Description.Configuration["instantiate_release_policy"] = "close verified instance; compiled module and host runtime retained; host-runtime Module handle prepared outside instantiation; no forced GC"
				resp.Description.Configuration["app_init_release_policy"] = "close verified instance; compiled module and host runtime retained; no forced GC"
				resp.Description.Capabilities["can_run_vectors"] = true
				resp.Description.Capabilities["can_vector_compile_phases"] = true
				resp.Description.Capabilities["can_vector_instantiate_phases"] = true
				resp.Description.Capabilities["can_vector_first-call_phases"] = true
				resp.Description.Configuration["vector_first_call_phases_policy"] = protocol.VectorFirstCallPhasesPolicy
				resp.Description.Configuration["vector_instantiate_phases_policy"] = "fresh instance; prepared compiled module retained; instantiation includes Wasm start; explicit initialization, ordered vector calls, oracle verification and release outside API window; no forced GC"
				resp.Description.Capabilities["can_vector_teardown_phases"] = true
				resp.Description.PhaseReleasePolicy = "close measured instance and compiled module; host runtime retained; no forced GC"
			case "prepare":
				a.closeAll()
				a.prep = req.Prepare
				a.imports = nil
				if a.prep == nil {
					e = fmt.Errorf("missing prepare")
					break
				}
				if a.prep.Workload.HostProfile == "identity-v1" {
					a.hostRuntime = wago.NewRuntime()
					a.hostIdentity, e = a.hostRuntime.NewHostFuncRef(func(v int32) int32 { return v }, wago.FuncSig{Params: []wago.ValType{wago.ValI32}, Results: []wago.ValType{wago.ValI32}})
					if e != nil {
						break
					}
					a.imports = wago.NewImports().Function("wasmbench", "identity", a.hostIdentity)
				} else if a.prep.Workload.HostProfile == protocol.AssemblyScriptAbortProfile {
					a.hostRuntime = wago.NewRuntime()
					a.hostIdentity, e = a.hostRuntime.NewHostFuncRef(wago.HostCallFunc(func(call wago.HostCall) {
						panic(wago.HostTrap{Err: protocol.AssemblyScriptAbort(uint32(call.I32(0)), uint32(call.I32(1)), uint32(call.I32(2)), uint32(call.I32(3)))})
					}), wago.FuncSig{Params: []wago.ValType{wago.ValI32, wago.ValI32, wago.ValI32, wago.ValI32}})
					if e != nil {
						break
					}
					a.imports = wago.NewImports().Function("env", "abort", a.hostIdentity)
				} else if a.prep.Workload.HostProfile != "" && a.prep.Workload.ABI != "wasi-command" && a.prep.Workload.ABI != "component" {
					e = fmt.Errorf("unsupported host profile")
					break
				}
				switch a.prep.Workload.ABI {
				case "core":
				case "wasi-command":
					if a.prep.Workload.Command == nil {
						e = fmt.Errorf("WASI command ABI requires a command contract")
						break
					}
				case "component":
					a.componentRuntime, e = loadComponentPluginRuntime(&a.componentRef)
					if e != nil {
						break
					}
					a.componentCache = component.NewCompileCache()
				default:
					e = fmt.Errorf("unsupported ABI")
					break
				}
				if e != nil {
					break
				}
				a.wasm, e = os.ReadFile(a.prep.Artifact)
				if e == nil && corpus.Hash(a.wasm) != a.prep.ArtifactSHA256 {
					e = fmt.Errorf("artifact digest mismatch")
				}
				if e == nil {
					// Keep Wago's normal runtime defaults while allowing the source-built
					// 100 MiB Swift-format module; the cap follows the measured artifact.
					a.compileConfig = wago.NewRuntimeConfig().WithMaxModuleBytes(uint64(len(a.wasm)))
				}
			case "run":
				a.barrier = func(event protocol.PhaseEvent) error { return protocol.Barrier(scanner, enc, req.ID, event) }
				resp.Samples, e = a.run(req.Run)
			case "inspect":
				if e = a.setup(); e == nil {
					if a.compiled.CodeSize() <= protocol.MaxCodeImageBytes {
						var image bytes.Buffer
						if _, e = a.compiled.WriteCodeTo(&image); e != nil {
							break
						}
						resp.CodeImage = &protocol.CodeImage{Version: 1, ModuleSHA256: a.prep.ArtifactSHA256, SHA256: corpus.Hash(image.Bytes()), Architecture: runtime.GOARCH, Backend: "railshot", Format: "raw-native-image", SectionKind: "mixed_code_and_embedded_data", Event: "compiled_snapshot", FunctionAttribution: "unavailable", Data: image.Bytes()}
					} else {
						resp.Diagnostics = append(resp.Diagnostics, protocol.Observation{Metric: "native.code_export", DefinitionVersion: 1, Unit: "bytes", Scope: "compiled_module", Phase: "compile", Collector: "wago.Compiled.WriteCodeTo", CollectorVersion: sourceRevision, Quality: "engine_reported", Profile: "code", Status: "unavailable", Reason: "native image exceeds 16 MiB transport budget; no partial export", Denominator: "module"})
					}
					b, err := a.compiled.MarshalBinary()
					e = err
					for metric, v := range map[string]float64{"native.code_image": float64(a.compiled.CodeSize()), "artifact.serialized": float64(len(b))} {
						resp.Diagnostics = append(resp.Diagnostics, protocol.Observation{Metric: metric, DefinitionVersion: 1, Value: protocol.Value(v), Unit: "bytes", Scope: "compiled_module", Phase: "compile", Collector: "wago.Compiled", CollectorVersion: sourceRevision, Quality: "engine_reported", Profile: "code", Status: "available", Denominator: "module"})
					}
				}
			case "close":
				a.close()
			default:
				e = fmt.Errorf("unknown method")
			}
		}
		var unsupported unsupportedRequest
		if errors.As(e, &unsupported) {
			resp.Status = "unsupported"
			resp.Reason = unsupported.Error()
			e = nil
		} else if e != nil && strings.HasPrefix(e.Error(), "unsupported: ") {
			resp.Status = "unsupported"
			resp.Reason = strings.TrimPrefix(e.Error(), "unsupported: ")
			e = nil
		}
		if e != nil {
			resp.Status = "error"
			resp.Reason = e.Error()
		}
		if e = enc.Encode(resp); e != nil {
			return
		}
		if req.Method == "close" {
			return
		}
	}
}
