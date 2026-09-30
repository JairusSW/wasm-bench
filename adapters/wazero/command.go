package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/sys"
	"github.com/wasmbench/wasmbench/collectors"
	"github.com/wasmbench/wasmbench/protocol"
)

type commandOutput struct {
	bytes.Buffer
	limit    uint64
	overflow bool
}

func (w *commandOutput) Write(p []byte) (int, error) {
	if uint64(len(p)) > w.limit-uint64(w.Len()) {
		w.overflow = true
		return 0, fmt.Errorf("command output budget exceeded")
	}
	return w.Buffer.Write(p)
}

type zeroRandom struct{}

func (zeroRandom) Read(p []byte) (int, error) { clear(p); return len(p), nil }

func (a *adapter) runCommand(r *protocol.RunRequest) ([]protocol.Sample, error) {
	w := a.prep.Workload
	if err := protocol.ValidateCommand(w); err != nil {
		return nil, err
	}
	if (r.PhaseBarriers && (!slices.Contains([]string{"compile", "instantiate", "first-call", "teardown"}, r.Scenario) || a.prep.Profile != "memory" || a.barrier == nil)) || !slices.Contains([]string{"timing", "memory"}, a.prep.Profile) || !slices.Contains([]string{"compile", "instantiate", "first-call", "steady", "teardown"}, r.Scenario) || r.Samples > 100000 || r.Warmup > 100000 {
		return nil, fmt.Errorf("unsupported command scenario/profile")
	}
	c := w.Command
	// Only newly created private fixture storage is writable by the host adapter.
	// No guest receives the source checkout or ambient host filesystem.
	dir, err := os.MkdirTemp("", "wasmbench-command-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	for name, file := range c.Files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return nil, err
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0400)
		if err != nil {
			return nil, err
		}
		err = protocol.CopyCommandFile(f, file, "")
		closeErr := f.Close()
		if err != nil {
			return nil, err
		}
		if closeErr != nil {
			return nil, closeErr
		}
	}
	var engine wazero.Runtime
	var emscripten *emscriptenHost
	if r.Scenario != "teardown" {
		engine = a.newEngine()
		defer engine.Close(ctx)
		if w.ABI == "emscripten" {
			emscripten = &emscriptenHost{}
		}
		if err := instantiateCommandHost(ctx, engine, w.ABI, emscripten); err != nil {
			return nil, err
		}
	}
	var shared wazero.CompiledModule
	if r.Scenario != "compile" && r.Scenario != "teardown" {
		shared, err = engine.CompileModule(ctx, a.wasm)
		if err != nil {
			return nil, err
		}
		defer shared.Close(ctx)
	}
	warmup := 0
	if r.Scenario == "steady" {
		warmup = r.Warmup
	}
	out := make([]protocol.Sample, 0, r.Samples+warmup)
	for i := 0; i < r.Samples+warmup; i++ {
		sample, err := func() (protocol.Sample, error) {
			engine := engine
			sampleEmscripten := emscripten
			if r.Scenario == "teardown" {
				engine = a.newEngine()
				defer engine.Close(ctx)
				if w.ABI == "emscripten" {
					sampleEmscripten = &emscriptenHost{}
				}
				if err := instantiateCommandHost(ctx, engine, w.ABI, sampleEmscripten); err != nil {
					return protocol.Sample{}, err
				}
			}
			if r.PhaseBarriers && r.Scenario == "compile" {
				if err := a.barrier(protocol.PhaseEvent{SampleIndex: i, Stage: "before_compile"}); err != nil {
					return protocol.Sample{}, err
				}
			}
			var finish func() []protocol.Observation
			var observations []protocol.Observation
			if a.prep.Profile == "memory" && r.Scenario != "teardown" {
				if r.PhaseBarriers && r.Scenario == "compile" {
					finish = collectors.GoMemoryWindow("compile/api_window", "operation_excluding_verification_release_and_barriers")
				} else if !r.PhaseBarriers {
					finish = collectors.GoMemoryWindow(r.Scenario+"/command_lifecycle", "command_including_instance_setup_capture_verification_release")
				}
			}
			compiled := shared
			var elapsed int64
			if compiled == nil {
				start := time.Now()
				compiled, err = engine.CompileModule(ctx, a.wasm)
				elapsed = time.Since(start).Nanoseconds()
				if err != nil {
					return protocol.Sample{}, err
				}
				defer compiled.Close(ctx)
			}
			if r.PhaseBarriers && r.Scenario == "compile" {
				observations = finish()
				finish = nil
				if err := a.barrier(protocol.PhaseEvent{SampleIndex: i, Stage: "compiled"}); err != nil {
					return protocol.Sample{}, err
				}
			}
			stdout, stderr := &commandOutput{limit: c.OutputLimit}, &commandOutput{limit: c.OutputLimit}
			var stdin io.Reader = bytes.NewReader(c.Stdin)
			var stdinFile *os.File
			if c.StdinFile != "" {
				f, err := os.Open(filepath.Join(dir, filepath.FromSlash(c.StdinFile)))
				if err != nil {
					return protocol.Sample{}, err
				}
				defer f.Close()
				stdinFile = f
				stdin = f
			}
			cfg := wazero.NewModuleConfig().WithName("").WithStartFunctions().WithArgs(c.Argv...).WithStdin(stdin).WithStdout(stdout).WithStderr(stderr).WithRandSource(zeroRandom{})
			if w.ABI == "emscripten" {
				cfg = cfg.WithWalltime(func() (int64, int32) { return 1, 1 }, 1).WithNanotime(func() int64 { return 1 }, 1)
			}
			if len(c.Files) > 0 && w.ABI != "emscripten" {
				cfg = cfg.WithFSConfig(wazero.NewFSConfig().WithReadOnlyDirMount(dir, "/"))
			}
			if r.PhaseBarriers && r.Scenario == "instantiate" {
				if err := a.barrier(protocol.PhaseEvent{SampleIndex: i, Stage: "before_instantiate"}); err != nil {
					return protocol.Sample{}, err
				}
				finish = collectors.GoMemoryWindow("instantiate/api_window", "operation_excluding_verification_release_and_barriers")
			}
			start := time.Now()
			instance, err := engine.InstantiateModule(ctx, compiled, cfg)
			if r.Scenario == "instantiate" {
				elapsed = time.Since(start).Nanoseconds()
			}
			if err != nil {
				return protocol.Sample{}, err
			}
			defer instance.Close(ctx)
			if r.PhaseBarriers && r.Scenario == "instantiate" {
				observations = finish()
				finish = nil
				if err := a.barrier(protocol.PhaseEvent{SampleIndex: i, Stage: "instantiated"}); err != nil {
					return protocol.Sample{}, err
				}
			}
			var function api.Function
			var emscriptenArgv uint32
			if w.ABI == "emscripten" {
				function, emscriptenArgv, err = prepareEmscriptenMain(ctx, instance, c.Argv)
				if err != nil && sampleEmscripten != nil && sampleEmscripten.assertion != "" {
					return protocol.Sample{}, fmt.Errorf("%s: %w", sampleEmscripten.assertion, err)
				}
				if err != nil {
					return protocol.Sample{}, err
				}
			} else {
				function = instance.ExportedFunction("_start")
				if function == nil || len(function.Definition().ParamTypes()) != 0 || len(function.Definition().ResultTypes()) != 0 {
					return protocol.Sample{}, fmt.Errorf("command _start must have no parameters or results")
				}
			}
			if r.PhaseBarriers && r.Scenario == "first-call" {
				if err := a.barrier(protocol.PhaseEvent{SampleIndex: i, Stage: "before_first_call"}); err != nil {
					return protocol.Sample{}, err
				}
				finish = collectors.GoMemoryWindow("first-call/api_window", "operation_excluding_verification_release_and_barriers")
			}
			start = time.Now()
			var results []uint64
			if w.ABI == "emscripten" {
				results, err = function.Call(ctx, uint64(len(c.Argv)), uint64(emscriptenArgv))
			} else {
				results, err = function.Call(ctx)
			}
			if r.Scenario == "first-call" || r.Scenario == "steady" {
				elapsed = time.Since(start).Nanoseconds()
			}
			if r.PhaseBarriers && r.Scenario == "first-call" {
				observations = finish()
				finish = nil
				if barrierErr := a.barrier(protocol.PhaseEvent{SampleIndex: i, Stage: "first_call_returned"}); barrierErr != nil {
					return protocol.Sample{}, barrierErr
				}
			}
			var code uint32
			if err == nil && len(results) == 1 {
				code = uint32(results[0])
			}
			if err != nil {
				var exit *sys.ExitError
				if !errors.As(err, &exit) {
					return protocol.Sample{}, err
				}
				code = exit.ExitCode()
			}
			if stdout.overflow || stderr.overflow {
				return protocol.Sample{}, fmt.Errorf("command output budget exceeded")
			}
			stdoutBytes := stdout.Bytes()
			stdoutDigest := protocol.CommandDigest(stdoutBytes)
			stdoutOracleDigest := ""
			if c.StdoutNormalize != "" {
				normalized, err := protocol.NormalizeCommandStdout(c.StdoutNormalize, stdoutBytes)
				if err != nil {
					return protocol.Sample{}, err
				}
				stdoutOracleDigest = protocol.CommandDigest(normalized)
			}
			result := protocol.CommandResult{ExitCode: code, StdoutSHA256: stdoutDigest, StdoutOracleSHA256: stdoutOracleDigest, StderrSHA256: protocol.CommandDigest(stderr.Bytes()), StdoutBytes: uint64(stdout.Len()), StderrBytes: uint64(stderr.Len())}
			if err := c.Verify(result); err != nil {
				return protocol.Sample{}, err
			}
			if r.PhaseBarriers && r.Scenario == "teardown" {
				if err := a.barrier(protocol.PhaseEvent{SampleIndex: i, Stage: "before_teardown"}); err != nil {
					return protocol.Sample{}, err
				}
			}
			if r.Scenario == "teardown" && a.prep.Profile == "memory" {
				finish = collectors.GoMemoryWindow("teardown/api_window", "remaining_command_resources_excluding_execution_verification_fixture_cleanup")
			}
			releaseStart := time.Now()
			if err := instance.Close(ctx); err != nil {
				return protocol.Sample{}, err
			}
			if shared == nil {
				if err := compiled.Close(ctx); err != nil {
					return protocol.Sample{}, err
				}
			}
			if stdinFile != nil {
				if err := stdinFile.Close(); err != nil {
					return protocol.Sample{}, err
				}
			}
			if r.Scenario == "teardown" {
				if err := engine.Close(ctx); err != nil {
					return protocol.Sample{}, err
				}
				elapsed = time.Since(releaseStart).Nanoseconds()
				if finish != nil {
					observations = finish()
					finish = nil
				}
			}
			if r.PhaseBarriers {
				stage := "released"
				if r.Scenario == "teardown" {
					stage = "torn_down"
				} else if r.Scenario == "instantiate" {
					stage = "instance_released"
				} else if r.Scenario == "first-call" {
					stage = "first_call_released"
				}
				if err := a.barrier(protocol.PhaseEvent{SampleIndex: i, Stage: stage}); err != nil {
					return protocol.Sample{}, err
				}
			}
			s := protocol.Sample{Index: i, Warmup: i < warmup, ElapsedNS: elapsed, Operations: 1, SampleType: "individual_operation", Verified: true, CommandResult: &result}
			s.Observations = observations
			if finish != nil {
				s.Observations = finish()
			}
			return s, nil
		}()
		if err != nil {
			return out, err
		}
		out = append(out, sample)
	}
	return out, nil
}
