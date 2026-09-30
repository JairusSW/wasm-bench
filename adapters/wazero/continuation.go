package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sync"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/wasmbench/wasmbench/corpus"
)

// These clocks have distinct boundaries. Restore is NOT an embedding API return
// clock: Restore never returns normally. Its interval ends at entry to the first
// host marker reached by resumed guest code and includes that transition.
type continuationMeasurement struct {
	Verified                             bool
	Depth                                int
	ArtifactSHA256                       string
	CreateNS, RestoreResumptionNS        int64
	FirstWriteNS, PostRestoreExecutionNS int64
	StackResult, PostRestoreResult       uint64
	MemoryAfterRestore, MemoryAfterWrite [32]byte
	GlobalAfterRestore, GlobalAfterWrite uint64
}

// continuationExperiment owns compiled/host modules, but not the caller's engine.
// Each measure owns a fresh guest instance and one snapshot used exactly once in
// its original active invocation. Close and measure are serialized; the caller
// must not close the engine concurrently. The batch adapter and controller gate
// canonical artifacts and the qualified compiler build; this module itself
// does not make a whole-instance/COW snapshot or physical-reclamation claim.
type continuationExperiment struct {
	mu       sync.Mutex
	runtime  wazero.Runtime
	compiled wazero.CompiledModule
	host     api.Module
	depth    int
	digest   string
	active   *continuationCapture
}

type continuationCapture struct {
	scenario                    string
	window                      func(string, bool) error
	snapshot                    experimental.Snapshot
	created, restoring, resumed bool
	createNS, restoreNS         int64
	restoreStart                time.Time
}

func (s *continuationCapture) boundary(stage string, start bool) error {
	if s.window != nil && s.scenario == stage {
		return s.window(stage, start)
	}
	return nil
}

func newContinuationExperiment(ctx context.Context, r wazero.Runtime, depth int) (*continuationExperiment, error) {
	b, err := corpus.ContinuationModule(depth)
	if err != nil {
		return nil, err
	}
	e := &continuationExperiment{runtime: r, depth: depth, digest: corpus.Hash(b)}
	builder := r.NewHostModuleBuilder("wasmbench_continuation_v1")
	builder.NewFunctionBuilder().WithFunc(func(ctx context.Context) uint32 {
		s := e.active
		if s == nil || s.created {
			panic("invalid continuation capture order")
		}
		snapshotter := experimental.GetSnapshotter(ctx)
		if err := s.boundary("continuation-create", true); err != nil {
			panic(err)
		}
		start := time.Now()
		s.snapshot = snapshotter.Snapshot()
		s.createNS = time.Since(start).Nanoseconds()
		s.created = true
		if err := s.boundary("continuation-create", false); err != nil {
			panic(err)
		}
		return 0
	}).Export("capture")
	builder.NewFunctionBuilder().WithFunc(func() {
		s := e.active
		if s == nil || !s.created || s.restoring {
			panic("invalid continuation restore order")
		}
		s.restoring = true
		ret := []uint64{1}
		if err := s.boundary("continuation-resume", true); err != nil {
			panic(err)
		}
		s.restoreStart = time.Now()
		s.snapshot.Restore(ret)
		panic("continuation restore returned normally")
	}).Export("restore")
	builder.NewFunctionBuilder().WithFunc(func() {
		// Capture the end timestamp before checking bookkeeping.
		end := time.Now()
		s := e.active
		if s == nil || !s.restoring || s.resumed {
			panic("invalid continuation resume order")
		}
		s.restoreNS = end.Sub(s.restoreStart).Nanoseconds()
		s.resumed = true
		if err := s.boundary("continuation-resume", false); err != nil {
			panic(err)
		}
	}).Export("resumed")
	e.host, err = builder.Instantiate(ctx)
	if err != nil {
		return nil, err
	}
	e.compiled, err = r.CompileModule(ctx, b)
	if err != nil {
		e.host.Close(ctx)
		return nil, err
	}
	return e, nil
}

func (e *continuationExperiment) close(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.compiled == nil {
		return nil
	}
	err := e.compiled.Close(ctx)
	e.compiled = nil
	if hostErr := e.host.Close(ctx); err == nil {
		err = hostErr
	}
	return err
}

func continuationState(m api.Module, written bool) ([32]byte, uint64, error) {
	var zero [32]byte
	if m.Memory() == nil || m.Memory().Size() != 65536 {
		return zero, 0, fmt.Errorf("incorrect result: continuation memory size")
	}
	data, ok := m.Memory().Read(0, 65536)
	if !ok {
		return zero, 0, fmt.Errorf("incorrect result: unreadable continuation memory")
	}
	for i, v := range data {
		want := byte(0)
		if i == 0 {
			want = 11
		}
		if written && i == 65535 {
			want = 22
		}
		if v != want {
			return zero, 0, fmt.Errorf("incorrect result: continuation memory byte %d", i)
		}
	}
	g := m.ExportedGlobal("state")
	want := uint64(99)
	if written {
		want = 100
	}
	if g == nil || g.Get() != want {
		return zero, 0, fmt.Errorf("incorrect result: continuation global")
	}
	return sha256.Sum256(data), g.Get(), nil
}

func (e *continuationExperiment) measure(ctx context.Context) (out continuationMeasurement, err error) {
	return e.measureStage(ctx, "", nil)
}

func (e *continuationExperiment) measureStage(ctx context.Context, scenario string, window func(string, bool) error) (out continuationMeasurement, err error) {
	// Failed correctness, cancellation or release produces no publishable clocks.
	defer func() {
		if err != nil {
			out = continuationMeasurement{}
		}
	}()
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.compiled == nil {
		return out, fmt.Errorf("continuation experiment closed")
	}
	m, err := e.runtime.InstantiateModule(ctx, e.compiled, wazero.NewModuleConfig().WithName("").WithStartFunctions())
	if err != nil {
		return out, err
	}
	defer func() {
		if closeErr := m.Close(ctx); err == nil {
			err = closeErr
		}
	}()
	s := &continuationCapture{scenario: scenario, window: window}
	e.active = s
	defer func() { e.active = nil }()
	values, err := m.ExportedFunction("run").Call(experimental.WithSnapshotter(ctx))
	e.active = nil
	s.snapshot = nil
	if err != nil {
		return out, err
	}
	want := uint64(7 + e.depth*(e.depth+1)/2)
	if len(values) != 1 || values[0] != want || !s.created || !s.restoring || !s.resumed {
		return out, fmt.Errorf("incorrect result: native continuation stack: got %v want %d; capture=%t restore=%t resume=%t", values, want, s.created, s.restoring, s.resumed)
	}
	out.Depth, out.ArtifactSHA256, out.StackResult = e.depth, e.digest, values[0]
	out.CreateNS, out.RestoreResumptionNS = s.createNS, s.restoreNS
	out.MemoryAfterRestore, out.GlobalAfterRestore, err = continuationState(m, false)
	if err != nil {
		return out, err
	}
	// Resolve exports outside these embedding API windows. Allocations performed
	// internally by Call remain part of the declared embedding operation.
	write, execute := m.ExportedFunction("first_write"), m.ExportedFunction("benchmark")
	if err = s.boundary("continuation-first-write", true); err != nil {
		return out, err
	}
	start := time.Now()
	values, err = write.Call(ctx)
	out.FirstWriteNS = time.Since(start).Nanoseconds()
	if err != nil {
		return out, err
	}
	if err = s.boundary("continuation-first-write", false); err != nil {
		return out, err
	}
	if len(values) != 0 {
		return out, fmt.Errorf("incorrect result: continuation write result")
	}
	out.MemoryAfterWrite, out.GlobalAfterWrite, err = continuationState(m, true)
	if err != nil {
		return out, err
	}
	if err = s.boundary("continuation-execute", true); err != nil {
		return out, err
	}
	start = time.Now()
	values, err = execute.Call(ctx)
	out.PostRestoreExecutionNS = time.Since(start).Nanoseconds()
	if err != nil {
		return out, err
	}
	if err = s.boundary("continuation-execute", false); err != nil {
		return out, err
	}
	if len(values) != 1 || values[0] != 133 {
		return out, fmt.Errorf("incorrect result: continuation execution checksum")
	}
	out.PostRestoreResult = values[0]
	out.Verified = true
	return out, nil
}
