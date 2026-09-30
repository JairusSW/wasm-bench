package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/wasmbench/wasmbench/corpus"
)

func TestNativeContinuationMeasurements(t *testing.T) {
	for _, depth := range []int{0, 1, 8, 32, 128} {
		t.Run(fmt.Sprintf("compiler/depth=%d", depth), func(t *testing.T) {
			a := &adapter{}
			r := a.newEngine()
			defer r.Close(ctx)
			e, err := newContinuationExperiment(ctx, r, depth)
			if err != nil {
				t.Fatal(err)
			}
			defer e.close(ctx)
			artifact, _ := corpus.ContinuationModule(depth)
			restored := make([]byte, 65536)
			restored[0] = 11
			restoredHash := sha256.Sum256(restored)
			restored[65535] = 22
			writtenHash := sha256.Sum256(restored)
			// Repeated measurements must use fresh instances and active captures,
			// not restore a snapshot retained from the previous invocation.
			for i := 0; i < 3; i++ {
				m, err := e.measure(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if !m.Verified || m.Depth != depth || m.ArtifactSHA256 != corpus.Hash(artifact) || m.StackResult != uint64(7+depth*(depth+1)/2) || m.PostRestoreResult != 133 {
					t.Fatalf("wrong identity/stack/checksum: %+v", m)
				}
				if m.MemoryAfterRestore != restoredHash || m.MemoryAfterWrite != writtenHash || m.GlobalAfterRestore != 99 || m.GlobalAfterWrite != 100 {
					t.Fatal("native stack capture misrepresented as memory/global restoration")
				}
				for _, ns := range []int64{m.CreateNS, m.RestoreResumptionNS, m.FirstWriteNS, m.PostRestoreExecutionNS} {
					if ns < 0 {
						t.Fatal("negative stage clock")
					}
				}
				if e.active != nil {
					t.Fatal("snapshot retained beyond original invocation")
				}
			}
			if err := e.close(ctx); err != nil {
				t.Fatal(err)
			}
			if _, err := e.measure(ctx); err == nil {
				t.Fatal("measurement after close")
			}
		})
	}
}

func TestNativeContinuationRejectsNonStackCorruption(t *testing.T) {
	a := &adapter{}
	r := a.newEngine()
	e, err := newContinuationExperiment(ctx, r, 8)
	if err != nil {
		t.Fatal(err)
	}
	// Replace only the host marker, leaving the actual engine snapshot path
	// intact. A checksum of just the two known write locations would miss this.
	if err := e.host.Close(ctx); err != nil {
		t.Fatal(err)
	}
	builder := r.NewHostModuleBuilder("wasmbench_continuation_v1")
	builder.NewFunctionBuilder().WithFunc(func(c context.Context) uint32 {
		s := e.active
		s.snapshot = experimental.GetSnapshotter(c).Snapshot()
		s.created = true
		return 0
	}).Export("capture")
	builder.NewFunctionBuilder().WithFunc(func() {
		e.active.restoring = true
		e.active.snapshot.Restore([]uint64{1})
	}).Export("restore")
	builder.NewFunctionBuilder().WithFunc(func(_ context.Context, m api.Module) {
		e.active.resumed = true
		m.Memory().WriteByte(1234, 42)
	}).Export("resumed")
	e.host, err = builder.Instantiate(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := e.measure(ctx); err == nil || !strings.Contains(err.Error(), "memory byte 1234") || result != (continuationMeasurement{}) {
		t.Fatalf("corrupted full-memory oracle did not reject changed byte: %v", err)
	}
	e.close(ctx)
	r.Close(ctx)
}

func TestNativeContinuationCannotRestoreAcrossInstances(t *testing.T) {
	for _, interpreter := range []bool{false, true} {
		a := &adapter{interpreter: interpreter}
		r := a.newEngine()
		var saved experimental.Snapshot
		builder := r.NewHostModuleBuilder("wasmbench_continuation_v1")
		builder.NewFunctionBuilder().WithFunc(func(c context.Context) uint32 {
			if saved == nil {
				saved = experimental.GetSnapshotter(c).Snapshot()
				return 1 // Exit normally without invoking restore.
			}
			saved.Restore([]uint64{1}) // Restore from the previous run is invalid.
			return 0
		}).Export("capture")
		builder.NewFunctionBuilder().WithFunc(func() {}).Export("restore")
		builder.NewFunctionBuilder().WithFunc(func() {}).Export("resumed")
		if _, err := builder.Instantiate(ctx); err != nil {
			t.Fatal(err)
		}
		b, _ := corpus.ContinuationModule(8)
		m, err := r.InstantiateWithConfig(ctx, b, wazero.NewModuleConfig().WithStartFunctions())
		if err != nil {
			t.Fatal(err)
		}
		c := experimental.WithSnapshotter(ctx)
		if _, err := m.ExportedFunction("run").Call(c); err != nil {
			t.Fatal(err)
		}
		if _, err := m.ExportedFunction("first_write").Call(c); err != nil {
			t.Fatal(err)
		}
		// A DIFFERENT exported function call engine must refuse the old capture.
		// The runtime API does not guarantee invalidation on same-function reentry;
		// our experiment forbids all later-invocation reuse regardless.
		// Force a different call engine by invoking a host capture through a fresh
		// instance of the identical compiled module.
		m2, err := r.InstantiateWithConfig(ctx, b, wazero.NewModuleConfig().WithName("").WithStartFunctions())
		if err != nil {
			t.Fatal(err)
		}
		func() {
			// Wazevo propagates an invalid restore as a panic, whereas the
			// interpreter may convert it into a guest-call error.
			defer func() {
				if p := recover(); p != nil && !strings.Contains(fmt.Sprint(p), "unhandled snapshot restore") {
					panic(p)
				}
			}()
			if _, err := m2.ExportedFunction("run").Call(c); err == nil {
				t.Fatal("cross-instance restoration accepted")
			}
		}()
		r.Close(ctx)
	}
}

func TestNativeContinuationInterpreterFailsResumptionOracle(t *testing.T) {
	// Wazero 1.12.0's interpreter exposes the experimental interface, but it
	// does not satisfy this fixture's guest-resumption contract. Preserve that
	// distinction: method availability is not correctness qualification.
	for _, depth := range []int{0, 1, 8, 32, 128} {
		r := (&adapter{interpreter: true}).newEngine()
		e, err := newContinuationExperiment(ctx, r, depth)
		if err != nil {
			t.Fatal(err)
		}
		if result, err := e.measure(ctx); err == nil || !strings.Contains(err.Error(), "resume=false") || result != (continuationMeasurement{}) {
			t.Fatalf("depth %d: interpreter must not qualify without guest resumption: %v", depth, err)
		}
		e.close(ctx)
		r.Close(ctx)
	}
}

func TestNativeContinuationValidationAndCancellation(t *testing.T) {
	r := (&adapter{}).newEngine()
	defer r.Close(ctx)
	for _, depth := range []int{-1, 129} {
		if _, err := newContinuationExperiment(ctx, r, depth); err == nil {
			t.Fatal("invalid depth admitted")
		}
	}
	e, err := newContinuationExperiment(ctx, r, 8)
	if err != nil {
		t.Fatal(err)
	}
	defer e.close(ctx)
	c, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := e.measure(c); err == nil {
		t.Fatal("cancelled measurement admitted")
	}
	if e.active != nil {
		t.Fatal("capture retained on cancelled measurement")
	}
}
