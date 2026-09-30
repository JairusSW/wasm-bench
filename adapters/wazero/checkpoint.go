package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"runtime"
	"time"

	"github.com/tetratelabs/wazero/api"
	"github.com/wasmbench/wasmbench/collectors"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/protocol"
)

type guestCheckpoint struct {
	memory []byte
	state  uint32
}

func checkpointState(m api.Module, size uint32) ([]byte, api.MutableGlobal, error) {
	if !hasMemory(m) || m.Memory().Size() != size {
		return nil, nil, fmt.Errorf("incorrect checkpoint memory size")
	}
	data, ok := m.Memory().Read(0, size)
	g, mutable := m.ExportedGlobal("state").(api.MutableGlobal)
	if !ok || !mutable || g.Type() != api.ValueTypeI32 {
		return nil, nil, fmt.Errorf("missing checkpoint memory or mutable i32")
	}
	return data, g, nil
}

func saveGuestCheckpoint(data []byte, g api.MutableGlobal) guestCheckpoint {
	return guestCheckpoint{memory: append([]byte(nil), data...), state: uint32(g.Get())}
}

func restoreGuestCheckpoint(m api.Module, g api.MutableGlobal, saved guestCheckpoint) error {
	if !m.Memory().Write(0, saved.memory) {
		return fmt.Errorf("checkpoint restore memory write failed")
	}
	g.Set(uint64(saved.state))
	return nil
}

func checkpointDigest(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

func (a *adapter) runCheckpoint(r *protocol.RunRequest) ([]protocol.Sample, error) {
	if err := protocol.ValidateCheckpoint(a.prep, r); err != nil {
		return nil, err
	}
	canonical, err := corpus.CheckpointModule(a.prep.Workload.Checkpoint.Pages)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(a.wasm, canonical) {
		return nil, fmt.Errorf("guest checkpoint requires exact generated module bytes; arbitrary mutable state is not supported")
	}
	if r.PhaseBarriers && a.barrier == nil {
		return nil, fmt.Errorf("checkpoint barrier handler unavailable")
	}
	if err := a.setupCompiled(); err != nil {
		return nil, err
	}
	out := make([]protocol.Sample, 0, r.Samples)
	for i := 0; i < r.Samples; i++ {
		s, err := a.checkpointSample(r, i)
		if err != nil {
			return out, err
		}
		out = append(out, s)
	}
	return out, nil
}

func (a *adapter) checkpointSample(r *protocol.RunRequest, index int) (protocol.Sample, error) {
	empty := protocol.Sample{}
	w := a.prep.Workload
	size := w.Checkpoint.Pages * 65536
	source, err := a.instantiate()
	if err != nil {
		return empty, err
	}
	defer source.Close(ctx)
	if err := a.initialize(source); err != nil {
		return empty, err
	}
	sourceData, sourceGlobal, err := checkpointState(source, size)
	if err != nil {
		return empty, err
	}
	expected := protocol.CheckpointMemory(w.Checkpoint.Pages, false)
	if !bytes.Equal(sourceData, expected) || sourceGlobal.Get() != 42 {
		return empty, fmt.Errorf("incorrect checkpoint initial state")
	}
	var saved guestCheckpoint
	var target api.Module
	var targetData []byte
	var targetGlobal api.MutableGlobal
	var fn api.Function
	var result []uint64
	defer func() {
		if target != nil {
			_ = target.Close(ctx)
		}
	}()
	makeTarget := func() error {
		var err error
		target, err = a.instantiate()
		if err != nil {
			return err
		}
		targetData, targetGlobal, err = checkpointState(target, size)
		return err
	}
	mutateSource := func() error {
		if _, err := source.ExportedFunction("first_write").Call(ctx); err != nil {
			return err
		}
		if sourceData[len(sourceData)-1] != 11 || sourceGlobal.Get() != 43 || !bytes.Equal(saved.memory, expected) || saved.state != 42 {
			return fmt.Errorf("checkpoint aliases source or source mutation failed")
		}
		return nil
	}
	verifyRestored := func() error {
		if !bytes.Equal(targetData, expected) || targetGlobal.Get() != 42 {
			return fmt.Errorf("incorrect restored full guest state")
		}
		return nil
	}
	if r.Scenario != "checkpoint-create" {
		saved = saveGuestCheckpoint(sourceData, sourceGlobal)
		if err := mutateSource(); err != nil {
			return empty, err
		}
		if err := makeTarget(); err != nil {
			return empty, err
		}
		if r.Scenario != "checkpoint-restore" {
			if err := restoreGuestCheckpoint(target, targetGlobal, saved); err != nil {
				return empty, err
			}
			if err := verifyRestored(); err != nil {
				return empty, err
			}
			name := "benchmark"
			if r.Scenario == "checkpoint-first-write" {
				name = "first_write"
			}
			fn = target.ExportedFunction(name)
		}
	}
	stages := protocol.PhaseStages(r.Scenario)
	barrier := func(stage string) error {
		if r.PhaseBarriers {
			return a.barrier(protocol.PhaseEvent{SampleIndex: index, Stage: stage})
		}
		return nil
	}
	if err := barrier(stages[0]); err != nil {
		return empty, err
	}
	var finish func() []protocol.Observation
	if a.prep.Profile == "memory" {
		finish = collectors.GoMemoryWindow(r.Scenario+"/operation_window", "single_guest_checkpoint_operation_excluding_setup_verification_release")
	}
	start := time.Now()
	switch r.Scenario {
	case "checkpoint-create":
		saved = saveGuestCheckpoint(sourceData, sourceGlobal)
	case "checkpoint-restore":
		err = restoreGuestCheckpoint(target, targetGlobal, saved)
	default:
		result, err = fn.Call(ctx)
	}
	elapsed := time.Since(start).Nanoseconds()
	var obs []protocol.Observation
	if finish != nil {
		obs = finish()
	}
	if err != nil {
		return empty, err
	}
	if err := barrier(stages[1]); err != nil {
		return empty, err
	}
	if r.Scenario == "checkpoint-create" {
		if err := mutateSource(); err != nil {
			return empty, err
		}
		if err := makeTarget(); err != nil {
			return empty, err
		}
		if err := restoreGuestCheckpoint(target, targetGlobal, saved); err != nil {
			return empty, err
		}
	}
	written := r.Scenario == "checkpoint-first-write"
	if !written {
		if err := verifyRestored(); err != nil {
			return empty, err
		}
	} else {
		if !bytes.Equal(targetData, protocol.CheckpointMemory(w.Checkpoint.Pages, true)) || targetGlobal.Get() != 43 {
			return empty, fmt.Errorf("incorrect first-write state")
		}
	}
	if r.Scenario != "checkpoint-execute" {
		result, err = target.ExportedFunction("benchmark").Call(ctx)
		if err != nil {
			return empty, err
		}
	}
	evidence := &protocol.CheckpointResult{Mode: protocol.GuestCheckpointMode, PayloadBytes: uint64(size) + 4, SavedMemorySHA256: checkpointDigest(saved.memory), RestoredMemorySHA256: checkpointDigest(targetData), SavedGlobal: saved.state, RestoredGlobal: uint32(targetGlobal.Get()), SourceIndependent: bytes.Equal(saved.memory, expected)}
	s := protocol.Sample{Index: index, ElapsedNS: elapsed, Operations: 1, SampleType: "individual_operation", Verified: true, Result: result, CheckpointResult: evidence}
	if err := protocol.VerifyCheckpointSample(w, r.Scenario, s); err != nil {
		return empty, err
	}
	if a.prep.Profile == "memory" {
		obs = append(obs, protocol.Observation{Metric: "checkpoint.payload_bytes", DefinitionVersion: 1, Value: protocol.Value(float64(evidence.PayloadBytes)), Unit: "bytes", Scope: "guest_state_checkpoint", Phase: stages[1], Collector: "wasmbench.eager_guest_checkpoint", CollectorVersion: "1", Quality: "exact", Profile: "memory", Status: "available", Denominator: "one_checkpoint"})
	}
	s.Observations = obs
	if err := target.Close(ctx); err != nil {
		return empty, err
	}
	if err := source.Close(ctx); err != nil {
		return empty, err
	}
	// Keep the eagerly copied payload live until verification, then release the
	// reference. No forced GC or claim that allocator pages have been reclaimed.
	runtime.KeepAlive(saved)
	saved.memory = nil
	targetData = nil
	sourceData = nil
	expected = nil
	if err := barrier(stages[2]); err != nil {
		return empty, err
	}
	return s, nil
}
