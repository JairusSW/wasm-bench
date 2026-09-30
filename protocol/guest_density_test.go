package protocol_test

import (
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/protocol"
	"testing"
)

func TestGuestDensityContractAndEveryInstance(t *testing.T) {
	ws, err := corpus.Generate(t.TempDir(), "guest-density")
	if err != nil {
		t.Fatal(err)
	}
	if len(ws) != 32 {
		t.Fatal("missing provisioning/state/size/count cases", len(ws))
	}
	curves := map[string]int{}
	for _, w := range ws {
		curves[w.Generator]++
		d := w.GuestDensity
		e := &protocol.GuestDensityResult{Provisioning: d.Provisioning, Instances: make([]protocol.GuestDensityInstance, d.Instances)}
		if d.Provisioning == "eager_guest_restore" {
			e.PayloadBytes = uint64(d.Pages)*65536 + 4
			e.SourceIndependent = true
			e.PayloadUnchanged = true
		}
		g := uint32(42)
		if d.FirstWrite {
			g = 43
		}
		for i := range e.Instances {
			e.Instances[i] = protocol.GuestDensityInstance{MemorySHA256: protocol.CheckpointMemorySHA256(d.Pages, d.FirstWrite), Global: g, Checksum: w.Oracle.Expected[0]}
		}
		s := protocol.Sample{Operations: 1, SampleType: "individual_operation", Verified: true, Result: w.Oracle.Expected, GuestDensityResult: e}
		if err := protocol.VerifyGuestDensitySample(w, s); err != nil {
			t.Fatal(err)
		}
		e.Instances[len(e.Instances)-1].Checksum++
		if protocol.VerifyGuestDensitySample(w, s) == nil {
			t.Fatal("unchecked final instance")
		}
		e.Instances[len(e.Instances)-1].Checksum--
		e.PayloadUnchanged = !e.PayloadUnchanged
		if protocol.VerifyGuestDensitySample(w, s) == nil {
			t.Fatal("accepted false/invented payload isolation")
		}
		p := &protocol.Preparation{Workload: w, Profile: "timing"}
		r := &protocol.RunRequest{Scenario: "guest-density", Samples: 1, Operations: 1}
		if err := protocol.ValidateGuestDensity(p, r); err != nil {
			t.Fatal(err)
		}
		r.Operations = 2
		if protocol.ValidateGuestDensity(p, r) == nil {
			t.Fatal("accepted batch groups")
		}
		r.Operations = 1
		r.PhaseBarriers = true
		if protocol.ValidateGuestDensity(p, r) == nil {
			t.Fatal("timing barriers")
		}
		bad := w
		bad.Generator = "wasmbench-guest-density-v1"
		if protocol.ValidateGuestDensityWorkload(bad) == nil {
			t.Fatal("pooled incompatible curves")
		}
	}
	if len(curves) != 8 {
		t.Fatal("merged policy/state/memory curves", curves)
	}
	for _, n := range curves {
		if n != 4 {
			t.Fatal("missing scaling points")
		}
	}
}
