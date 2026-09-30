package main

import (
	"fmt"
	"runtime"
	"time"

	wago "github.com/wago-org/wago"
	"github.com/wasmbench/wasmbench/collectors"
	"github.com/wasmbench/wasmbench/protocol"
)

func (a *adapter) compilePhases(r *protocol.RunRequest) ([]protocol.Sample, error) {
	if (a.prep.Profile != "memory" && a.prep.Profile != "counters") || r.Scenario != "compile" || a.barrier == nil {
		return nil, fmt.Errorf("unsupported phase barrier request")
	}
	a.close()
	out := make([]protocol.Sample, 0, r.Samples)
	for i := 0; i < r.Samples; i++ {
		if err := a.barrier(protocol.PhaseEvent{SampleIndex: i, Stage: "before_compile"}); err != nil {
			return nil, err
		}
		var before, after runtime.MemStats
		if a.prep.Profile == "memory" {
			runtime.ReadMemStats(&before)
		}
		start := time.Now()
		compiled, err := wago.Compile(nil, a.wasm)
		elapsed := time.Since(start).Nanoseconds()
		if a.prep.Profile == "memory" {
			runtime.ReadMemStats(&after)
		}
		if err != nil {
			return nil, err
		}
		if err = a.barrier(protocol.PhaseEvent{SampleIndex: i, Stage: "compiled"}); err != nil {
			compiled.Close()
			return nil, err
		}
		err = a.checkModule(compiled)
		compiled.Close()
		if err != nil {
			return nil, err
		}
		if err = a.barrier(protocol.PhaseEvent{SampleIndex: i, Stage: "released"}); err != nil {
			return nil, err
		}
		s := protocol.Sample{Index: i, ElapsedNS: elapsed, Operations: 1, SampleType: "individual_operation", Verified: true}
		if a.prep.Profile == "memory" {
			s.Observations = collectors.GoMemoryObservations(before, after, "compile/api_window", "operation_excluding_verification_release_and_barriers")
		}
		out = append(out, s)
	}
	return out, nil
}
