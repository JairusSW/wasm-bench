package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	component "github.com/wago-org/component-model"
	wago "github.com/wago-org/wago"
	wagoplugin "github.com/wago-org/wago/plugin"
	"github.com/wago-org/wasi/p1"
	"github.com/wago-org/wasi/p2"
	"github.com/wasmbench/wasmbench/protocol"
)

type unsupportedRequest string

func (e unsupportedRequest) Error() string { return string(e) }

type componentConsumer struct {
	ref **wagoplugin.Ref[component.Service]
}

func (c componentConsumer) Register(reg *wago.Registrar) error {
	ref, err := wagoplugin.Require(reg, component.Contract)
	*c.ref = ref
	return err
}

func loadComponentPluginRuntime(ref **wagoplugin.Ref[component.Service]) (*wago.Runtime, error) {
	cm := component.Provider()
	consumerDef := wago.PluginDefinition{
		ID: "github.com/wasmbench/wasmbench/component-consumer", Name: "wasm-bench component consumer", Version: "1.0.0",
		Stability:  wago.Experimental,
		Provenance: wago.PluginProvenance{Repository: "https://github.com/wasmbench/wasmbench", License: "Apache-2.0"},
		Requires:   []wago.PluginRequirement{{ID: component.PluginID, Version: "^0.1.0"}},
		Consumes:   []wago.ContractRequirement{{ID: component.Contract.ID(), Major: component.Contract.Major(), Mode: wago.ContractRequired}},
	}
	consumer := wago.PluginProvider{Definition: consumerDef, New: func() wago.Plugin { return componentConsumer{ref: ref} }}
	providers := []wago.PluginProvider{cm, consumer}
	set := wago.PluginSet{Providers: providers}
	for i, provider := range providers {
		digest, err := wago.DefinitionDigest(provider.Definition)
		if err != nil {
			return nil, err
		}
		sel := wago.PluginSelection{ID: provider.Definition.ID, DefinitionDigest: digest, Direct: i == 1, Dependencies: map[string]string{}}
		for _, dependency := range provider.Definition.Requires {
			sel.Dependencies[dependency.ID] = dependency.Version
		}
		for _, authority := range provider.Definition.Authorities {
			sel.Grants = append(sel.Grants, wago.AuthorityGrant{Name: authority.Name, Scope: authority.Scope})
		}
		for _, required := range provider.Definition.Consumes {
			for _, candidate := range providers {
				for _, offered := range candidate.Definition.Provides {
					if offered.ID == required.ID && offered.Major == required.Major {
						sel.Contracts = append(sel.Contracts, wago.ContractBinding{ID: required.ID, Major: required.Major, Providers: []string{candidate.Definition.ID}})
					}
				}
			}
		}
		set.Selections = append(set.Selections, sel)
	}
	rt := wago.NewRuntime()
	if err := rt.LoadPlugins(context.Background(), set); err != nil {
		_ = rt.Close()
		return nil, err
	}
	return rt, nil
}

type boundedBuffer struct {
	bytes.Buffer
	limit uint64
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if uint64(b.Len())+uint64(len(p)) > b.limit {
		return 0, fmt.Errorf("WASI output exceeds %d bytes", b.limit)
	}
	return b.Buffer.Write(p)
}

func commandStdin(c *protocol.CommandContract) ([]byte, error) {
	if c.StdinFile == "" {
		return append([]byte(nil), c.Stdin...), nil
	}
	file, ok := c.Files[c.StdinFile]
	if !ok {
		return nil, fmt.Errorf("stdin fixture %q is missing", c.StdinFile)
	}
	var out bytes.Buffer
	if err := protocol.CopyCommandFile(&out, file, ""); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func removeCommandFiles(dir string) error {
	_ = filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			_ = os.Chmod(path, 0700)
		}
		return nil
	})
	return os.RemoveAll(dir)
}

func stageCommandFiles(c *protocol.CommandContract) (string, []p1.Preopen, []p2.Preopen, error) {
	dir, err := os.MkdirTemp("", "wasmbench-wago-wasi-")
	if err != nil {
		return "", nil, nil, err
	}
	for name, file := range c.Files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			_ = removeCommandFiles(dir)
			return "", nil, nil, err
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0400)
		if err != nil {
			_ = removeCommandFiles(dir)
			return "", nil, nil, err
		}
		err = protocol.CopyCommandFile(f, file, "")
		closeErr := f.Close()
		if err != nil || closeErr != nil {
			_ = removeCommandFiles(dir)
			if err != nil {
				return "", nil, nil, err
			}
			return "", nil, nil, closeErr
		}
	}
	// The WASI preopen uses guest credentials that may not match the host
	// process owner. Fixtures stay immutable while remaining readable there.
	if err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		mode := os.FileMode(0444)
		if entry.IsDir() {
			mode = 0555
		}
		return os.Chmod(path, mode)
	}); err != nil {
		_ = removeCommandFiles(dir)
		return "", nil, nil, err
	}
	return dir, []p1.Preopen{{GuestPath: "/", HostPath: dir, Read: true}}, []p2.Preopen{{GuestPath: "/", HostPath: dir, Read: true}}, nil
}

func commandDigests(c *protocol.CommandContract, exit uint32, stdout, stderr []byte) (protocol.CommandResult, error) {
	result := protocol.CommandResult{ExitCode: exit, StdoutSHA256: protocol.CommandDigest(stdout), StderrSHA256: protocol.CommandDigest(stderr), StdoutBytes: uint64(len(stdout)), StderrBytes: uint64(len(stderr))}
	if c.StdoutNormalize != "" {
		normalized, err := protocol.NormalizeCommandStdout(c.StdoutNormalize, stdout)
		if err != nil {
			return protocol.CommandResult{}, err
		}
		h := sha256.Sum256(normalized)
		result.StdoutOracleSHA256 = hex.EncodeToString(h[:])
	}
	if err := c.Verify(result); err != nil {
		return protocol.CommandResult{}, err
	}
	return result, nil
}

func wasmExit(err error) (uint32, error) {
	if err == nil {
		return 0, nil
	}
	var exit *wago.ExitError
	if errors.As(err, &exit) {
		if exit.Code < 0 {
			return 0, fmt.Errorf("negative WASI exit code %d", exit.Code)
		}
		return uint32(exit.Code), nil
	}
	return 0, err
}

func (a *adapter) runWasiP1One(scenario string, c *protocol.CommandContract, mounts []p1.Preopen, stdin []byte) (int64, protocol.CommandResult, error) {
	compiled := a.compiled
	compileNS := int64(0)
	freshCompile := scenario == "compile" || compiled == nil
	if freshCompile {
		start := time.Now()
		var err error
		compiled, err = wago.Compile(a.compileConfig, a.wasm)
		compileNS = time.Since(start).Nanoseconds()
		if err != nil {
			return 0, protocol.CommandResult{}, err
		}
		if scenario != "compile" {
			a.compiled = compiled
		} else {
			defer compiled.Close()
		}
	}
	stdout, stderr := &boundedBuffer{limit: c.OutputLimit}, &boundedBuffer{limit: c.OutputLimit}
	imports := p1.Imports(p1.Config{Stdin: bytes.NewReader(stdin), Stdout: stdout, Stderr: stderr, Args: append([]string(nil), c.Argv...), Mounts: mounts})
	if scenario == "compile" {
		instance, err := wago.Instantiate(compiled, wago.InstantiateOptions{Imports: imports})
		if err != nil {
			return 0, protocol.CommandResult{}, err
		}
		defer instance.Close()
		_, invokeErr := instance.Invoke("_start")
		exit, err := wasmExit(invokeErr)
		if err != nil {
			return 0, protocol.CommandResult{}, err
		}
		result, err := commandDigests(c, exit, stdout.Bytes(), stderr.Bytes())
		return compileNS, result, err
	}
	start := time.Now()
	instance, err := wago.Instantiate(compiled, wago.InstantiateOptions{Imports: imports})
	instantiateNS := time.Since(start).Nanoseconds()
	if err != nil {
		return 0, protocol.CommandResult{}, err
	}
	defer instance.Close()
	if scenario == "instantiate" {
		_, invokeErr := instance.Invoke("_start")
		exit, err := wasmExit(invokeErr)
		if err != nil {
			return 0, protocol.CommandResult{}, err
		}
		result, err := commandDigests(c, exit, stdout.Bytes(), stderr.Bytes())
		return instantiateNS, result, err
	}
	if scenario != "first-call" && scenario != "steady" {
		return 0, protocol.CommandResult{}, unsupportedRequest("WASI Preview 1 supports compile, instantiate, first-call and steady timing only")
	}
	start = time.Now()
	_, invokeErr := instance.Invoke("_start")
	elapsed := time.Since(start).Nanoseconds()
	exit, err := wasmExit(invokeErr)
	if err != nil {
		return 0, protocol.CommandResult{}, err
	}
	result, err := commandDigests(c, exit, stdout.Bytes(), stderr.Bytes())
	return elapsed, result, err
}

func (a *adapter) runComponentOne(scenario string, w protocol.Workload, mounts []p2.Preopen, stdin []byte) (int64, protocol.Values, protocol.CommandResult, error) {
	if a.componentRef == nil || a.componentCache == nil {
		return 0, nil, protocol.CommandResult{}, fmt.Errorf("Component Model plugin runtime is not loaded")
	}
	if w.Oracle.Kind == "component_compile_only" {
		return 0, nil, protocol.CommandResult{}, unsupportedRequest("Component Model compile-only workload has no invocation oracle")
	}
	if scenario == "compile" {
		return 0, nil, protocol.CommandResult{}, unsupportedRequest("Wago's public Component Model plugin API does not expose standalone compilation; compile latency is not measured")
	}
	var stdout, stderr *boundedBuffer
	var args []string
	var cfg p2.Config
	if w.Command != nil {
		stdout, stderr = &boundedBuffer{limit: w.Command.OutputLimit}, &boundedBuffer{limit: w.Command.OutputLimit}
		args = append([]string(nil), w.Command.Argv...)
		cfg = p2.Config{Stdin: p2.NewInputStream(bytes.NewReader(stdin)), Stdout: p2.NewOutputStream(stdout), Stderr: p2.NewOutputStream(stderr), Args: args, Mounts: mounts}
	}
	opts := append(p2.Options(cfg), component.WithCompileCache(a.componentCache))
	ctx := context.Background()
	var elapsed int64
	var value protocol.Values
	var result protocol.CommandResult
	instantiateStarted := time.Now()
	err := a.componentRef.With(func(service component.Service) error {
		return service.WithInstance(ctx, a.wasm, func(in *component.Instance) error {
			if w.Command == nil {
				if scenario == "instantiate" {
					elapsed = time.Since(instantiateStarted).Nanoseconds()
					return nil
				}
				if scenario != "first-call" && scenario != "steady" {
					return unsupportedRequest("Wago Component Model supports instantiation and invocation timings; standalone compilation is unavailable")
				}
				started := time.Now()
				args := make([]component.Value, len(w.Args))
				for i, arg := range w.Args {
					args[i] = arg
				}
				got, err := in.Call(ctx, w.Export, args...)
				elapsed = time.Since(started).Nanoseconds()
				if err != nil {
					return err
				}
				value = make(protocol.Values, len(got))
				for i, v := range got {
					n, ok := v.(uint64)
					if !ok {
						return fmt.Errorf("Component Model result %d has type %T, want u64", i, v)
					}
					value[i] = n
				}
				if w.Oracle.Kind != "component_compile_only" && !slices.Equal(value, w.Oracle.Expected) {
					return fmt.Errorf("component result mismatch: got %v want %v", value, w.Oracle.Expected)
				}
				return nil
			}
			var runExport string
			for _, name := range in.InstanceExports() {
				if strings.HasPrefix(name, "wasi:cli/run@") {
					runExport = name
					break
				}
			}
			if runExport == "" {
				return fmt.Errorf("WASI Preview 2 component does not export wasi:cli/run")
			}
			if scenario == "instantiate" {
				elapsed = time.Since(instantiateStarted).Nanoseconds()
				return nil
			}
			if scenario != "first-call" && scenario != "steady" {
				return unsupportedRequest("WASI Preview 2 supports instantiation and command invocation timings; standalone compilation is unavailable")
			}
			started := time.Now()
			got, err := in.CallExport(ctx, runExport, "run")
			elapsed = time.Since(started).Nanoseconds()
			if err != nil {
				return err
			}
			exit := uint32(0)
			if len(got) != 1 {
				return fmt.Errorf("WASI Preview 2 run returned %d values", len(got))
			}
			rv, ok := got[0].(component.ResultValue)
			if !ok {
				return fmt.Errorf("WASI Preview 2 run returned %T, want result", got[0])
			}
			if rv.IsErr {
				exit = 1
			}
			result, err = commandDigests(w.Command, exit, stdout.Bytes(), stderr.Bytes())
			return err
		}, opts...)
	})
	if err != nil {
		return 0, nil, protocol.CommandResult{}, err
	}
	return elapsed, value, result, nil
}

func (a *adapter) runPluginFeature(r *protocol.RunRequest) ([]protocol.Sample, error) {
	w := a.prep.Workload
	if a.prep.Profile != "timing" || r.PhaseBarriers || r.Operations != 1 || r.Samples < 1 || r.Samples > 100000 || r.Warmup < 0 || r.Warmup > 100000 {
		return nil, unsupportedRequest("Wago plugin feature adapter supports unbarriered timing with one operation")
	}
	if w.ABI == "wasi-command" {
		if err := protocol.ValidateCommand(w); err != nil {
			return nil, err
		}
		stdin, err := commandStdin(w.Command)
		if err != nil {
			return nil, err
		}
		dir, p1Mounts, _, err := stageCommandFiles(w.Command)
		if err != nil {
			return nil, err
		}
		defer func() { _ = removeCommandFiles(dir) }()
		out := make([]protocol.Sample, 0, r.Samples+r.Warmup)
		for i := 0; i < r.Samples+r.Warmup; i++ {
			elapsed, result, err := a.runWasiP1One(r.Scenario, w.Command, p1Mounts, stdin)
			if err != nil {
				return nil, err
			}
			out = append(out, protocol.Sample{Index: i, Warmup: i < r.Warmup, ElapsedNS: elapsed, Operations: 1, SampleType: "individual_operation", Verified: true, CommandResult: &result})
		}
		return out, nil
	}
	if w.ABI == "component" {
		if w.Command == nil && w.Oracle.Kind != "exact_u64" && w.Oracle.Kind != "component_compile_only" {
			return nil, unsupportedRequest("unsupported Component Model oracle")
		}
		var mounts []p2.Preopen
		var stdin []byte
		var dir string
		if w.Command != nil {
			if err := protocol.ValidateCommand(w); err != nil {
				return nil, err
			}
			var err error
			stdin, err = commandStdin(w.Command)
			if err != nil {
				return nil, err
			}
			var p1Mounts []p1.Preopen
			dir, p1Mounts, mounts, err = stageCommandFiles(w.Command)
			_ = p1Mounts
			if err != nil {
				return nil, err
			}
			defer func() { _ = removeCommandFiles(dir) }()
		}
		if r.Scenario == "instantiate" {
			// WithInstance combines component decoding/core compilation with
			// instance creation. Prime its per-request compile cache on an
			// isolated fixture so the measured sample covers instantiation only.
			warmMounts := mounts
			if w.Command != nil {
				warmDir, _, stagedMounts, err := stageCommandFiles(w.Command)
				if err != nil {
					return nil, err
				}
				defer func() { _ = removeCommandFiles(warmDir) }()
				warmMounts = stagedMounts
			}
			if _, _, _, err := a.runComponentOne("instantiate", w, warmMounts, stdin); err != nil {
				return nil, err
			}
		}
		out := make([]protocol.Sample, 0, r.Samples+r.Warmup)
		for i := 0; i < r.Samples+r.Warmup; i++ {
			elapsed, value, command, err := a.runComponentOne(r.Scenario, w, mounts, stdin)
			if err != nil {
				return nil, err
			}
			sample := protocol.Sample{Index: i, Warmup: i < r.Warmup, ElapsedNS: elapsed, Operations: 1, SampleType: "individual_operation", Verified: true, Result: value}
			if w.Command != nil {
				sample.CommandResult = &command
			}
			out = append(out, sample)
		}
		return out, nil
	}
	return nil, unsupportedRequest("unsupported Wago plugin workload ABI")
}
