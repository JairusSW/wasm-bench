package experiment

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/protocol"
)

// ValidateGuestDensityEvidence independently checks the canonical state scope,
// every held instance and the locked measurement boundary without running Wasm.
func ValidateGuestDensityEvidence(root string, b Bundle) error {
	workloads := map[string]protocol.Workload{}
	for _, w := range b.Manifest.Lock.Workloads {
		if w.GuestDensity == nil && w.Oracle.Kind != "guest_density_v1" {
			continue
		}
		if err := protocol.ValidateGuestDensityWorkload(w); err != nil {
			return err
		}
		canonical, err := corpus.CheckpointModule(w.GuestDensity.Pages)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(filepath.Join(root, "artifacts", w.SHA256+".wasm"))
		if err != nil {
			return err
		}
		if w.SHA256 != corpus.Hash(canonical) || !bytes.Equal(data, canonical) {
			return fmt.Errorf("guest density canonical artifact mismatch")
		}
		workloads[w.ID] = w
	}
	for _, t := range b.Trials {
		w, ok := workloads[t.Workload]
		if !ok {
			for _, s := range t.Samples {
				if s.GuestDensityResult != nil {
					return fmt.Errorf("guest density evidence outside contract")
				}
			}
			continue
		}
		if t.Status != "ok" {
			continue
		}
		if t.Scenario != "guest-density" || t.Profile != b.Manifest.Lock.Options.Profile {
			return fmt.Errorf("guest density scenario/profile differs from lock")
		}
		r := trialRequest(b.Manifest.Lock.Options, w, t.Scenario, t.Block)
		profile := t.Profile
		if t.Block < 0 && (profile == "counters" || profile == "profiling") {
			profile = "timing"
		}
		if err := protocol.ValidateGuestDensity(&protocol.Preparation{Workload: w, Profile: profile}, &r); err != nil {
			return err
		}
		if err := validateSampleSequence(r, t.Samples); err != nil {
			return err
		}
		stages := protocol.PhaseStages(t.Scenario)
		if r.PhaseBarriers {
			if len(t.PhaseEvents) != len(t.Samples)*len(stages) {
				return fmt.Errorf("guest density missing phase barriers")
			}
			for i, e := range t.PhaseEvents {
				if e.Event.SampleIndex != i/len(stages) || e.Event.Stage != stages[i%len(stages)] {
					return fmt.Errorf("guest density reordered phase barriers")
				}
			}
		} else if len(t.PhaseEvents) != 0 {
			return fmt.Errorf("unexpected guest density phase instrumentation")
		}
		for sampleIndex, s := range t.Samples {
			if err := protocol.VerifyGuestDensitySample(w, s); err != nil {
				return fmt.Errorf("guest density trial %s: %w", t.ID, err)
			}
			logical := 0
			seen := map[string]bool{}
			observations := s.Observations
			if r.PhaseBarriers {
				var external []protocol.Observation
				for _, record := range t.PhaseEvents[sampleIndex*len(stages) : (sampleIndex+1)*len(stages)] {
					for _, o := range record.Observations {
						if o.Profile != "memory" || !strings.HasPrefix(o.Phase, "guest-density/") {
							return fmt.Errorf("guest density external phase domain mismatch")
						}
					}
					external = append(external, record.Observations...)
				}
				if len(observations) != 8+len(external) || (len(external) > 0 && !reflect.DeepEqual(observations[8:], external)) {
					return fmt.Errorf("guest density external snapshots differ from phase records")
				}
				observations = observations[:8]
			}
			for _, o := range observations {
				if profile != "memory" {
					return fmt.Errorf("guest density timing contains memory instrumentation")
				}
				if o.Metric == "density.guest_memory.logical" {
					logical++
					if o.DefinitionVersion != 1 || o.Value == nil || *o.Value != float64(uint64(w.GuestDensity.Pages)*65536*uint64(w.GuestDensity.Instances)) || o.Unit != "bytes" || o.Scope != "instance_group_linear_memory" || o.Phase != stages[1] || o.Collector != "wazero.Memory.Size" || o.CollectorVersion != "1.12.0" || o.Quality != "exact" || o.Profile != "memory" || o.Status != "available" || o.Denominator != "instance_group" {
						return fmt.Errorf("invalid guest density logical memory metric")
					}
				} else {
					units := map[string]string{"host.alloc.bytes": "bytes", "host.alloc.count": "count", "host.heap.start": "bytes", "host.heap.end": "bytes", "host.gc.cycles": "count", "host.gc.forced_cycles": "count", "host.gc.pause_time": "ns"}
					if seen[o.Metric] || units[o.Metric] == "" || o.DefinitionVersion != 1 || o.Unit != units[o.Metric] || o.Phase != protocol.GuestDensityPhase || o.Denominator != protocol.GuestDensityDenominator || o.Scope != "adapter_process_go_heap" || o.Profile != "memory" || o.Collector != "runtime.ReadMemStats" || o.CollectorVersion == "" || o.Quality != "engine_reported" || o.Status != "available" || o.Value == nil || math.IsNaN(*o.Value) || math.IsInf(*o.Value, 0) || *o.Value < 0 {
						return fmt.Errorf("invalid guest density allocator boundary/value")
					}
					seen[o.Metric] = true
				}
			}
			if profile == "memory" && logical != 1 {
				return fmt.Errorf("missing/duplicate guest density logical metric")
			}
			if profile == "memory" && (len(seen) != 7 || len(observations) != 8) {
				return fmt.Errorf("guest density allocator observations incomplete")
			}
		}
	}
	return nil
}
