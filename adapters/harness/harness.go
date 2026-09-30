// Package harness measures empty local control-loop bookkeeping, not Wasm work.
package harness

import (
	"fmt"
	"time"

	"github.com/wasmbench/wasmbench/protocol"
)

//go:noinline
func verify(slots []uint64) bool {
	for i, value := range slots {
		if value != uint64(i+1) {
			return false
		}
	}
	return true
}

// Run must follow untimed workload verification by the embedding adapter.
func Run(p *protocol.Preparation, r *protocol.RunRequest) ([]protocol.Sample, error) {
	if err := protocol.ValidateHarnessCalibration(p, r); err != nil {
		return nil, err
	}
	slots := make([]uint64, r.Operations)
	out := make([]protocol.Sample, r.Samples)
	for i := range out {
		clear(slots)
		start := time.Now()
		for j := range slots {
			slots[j] = uint64(j + 1)
		}
		elapsed := time.Since(start).Nanoseconds()
		if !verify(slots) {
			return nil, fmt.Errorf("incorrect harness calibration bookkeeping")
		}
		kind := "batch_average"
		if r.Operations == 1 {
			kind = "individual_operation"
		}
		out[i] = protocol.Sample{Index: i, ElapsedNS: elapsed, Operations: r.Operations, SampleType: kind, Verified: true, Result: protocol.Values{uint64(r.Operations)}}
	}
	return out, nil
}
