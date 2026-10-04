package experiment

import (
	"bytes"
	"fmt"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/protocol"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

// ValidateContinuationEvidence binds canonical artifacts, qualified engine,
// stage clocks, complete state proofs and separately scoped memory windows.
func ValidateContinuationEvidence(root string, b Bundle) error {
	workloads := map[string]protocol.Workload{}
	for _, w := range b.Manifest.Lock.Workloads {
		if w.Continuation == nil && w.Oracle.Kind != "native_continuation_v1" {
			continue
		}
		if err := protocol.ValidateContinuationWorkload(w); err != nil {
			return err
		}
		canonical, err := corpus.ContinuationModule(w.Continuation.Depth)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(filepath.Join(root, "artifacts", w.SHA256+".wasm"))
		if err != nil {
			return err
		}
		if w.SHA256 != corpus.Hash(canonical) || !bytes.Equal(data, canonical) {
			return fmt.Errorf("native continuation canonical artifact mismatch")
		}
		workloads[w.ID] = w
	}
	for _, t := range b.Trials {
		w, ok := workloads[t.Workload]
		if !ok {
			for _, s := range append(append([]protocol.Sample{}, t.Samples...), t.AdapterSamples...) {
				if s.ContinuationResult != nil {
					return fmt.Errorf("native continuation evidence outside workload contract")
				}
			}
			continue
		}
		if t.Status != "ok" {
			continue
		}
		qualified := false
		for _, r := range b.Manifest.Lock.Runtimes {
			d := r.Description
			if r.ID == t.Runtime && d != nil && d.Runtime == "wazero" && d.Version == "1.12.0" && d.Backend == "compiler" && d.Capabilities["can_native_continuation"] && d.Configuration["native_continuation_protocol"] == protocol.NativeContinuationMode {
				qualified = true
			}
		}
		if !qualified {
			return fmt.Errorf("native continuation trial lacks qualified engine identity")
		}
		if t.Profile != b.Manifest.Lock.Options.Profile || (t.Block < 0 && t.Scenario != "continuation-resume") {
			return fmt.Errorf("native continuation scenario/profile differs from lock")
		}
		r := trialRequest(b.Manifest.Lock.Options, w, t.Scenario, t.Block)
		if err := protocol.ValidateContinuation(&protocol.Preparation{Workload: w, Profile: t.Profile}, &r); err != nil {
			return err
		}
		if err := validateSampleSequence(r, t.Samples); err != nil {
			return err
		}
		stages := protocol.PhaseStages(t.Scenario)
		if r.PhaseBarriers {
			if len(t.PhaseEvents) != len(t.Samples)*len(stages) {
				return fmt.Errorf("native continuation missing phase barriers")
			}
			for i, event := range t.PhaseEvents {
				if event.Event.SampleIndex != i/len(stages) || event.Event.Stage != stages[i%len(stages)] {
					return fmt.Errorf("native continuation reordered phase barriers")
				}
			}
		} else if len(t.PhaseEvents) != 0 {
			return fmt.Errorf("unexpected native continuation phase instrumentation")
		}
		for index, s := range t.Samples {
			if err := protocol.VerifyContinuationSample(w, t.Scenario, s); err != nil {
				return err
			}
			obs := s.Observations
			if r.PhaseBarriers {
				var external []protocol.Observation
				for _, event := range t.PhaseEvents[index*3 : index*3+3] {
					seenSnapshots := map[string]bool{}
					for _, o := range event.Observations {
						if o.Profile != "memory" || !strings.HasPrefix(o.Phase, t.Scenario+"/") {
							return fmt.Errorf("native continuation external observation domain mismatch")
						}
						if o.Metric == "process.rss" || o.Metric == "process.pss" || o.Metric == "process.private" || o.Metric == "process.virtual" {
							if seenSnapshots[o.Metric] || o.Phase != t.Scenario+"/"+event.Event.Stage || o.DefinitionVersion != 1 || o.Unit != "bytes" || o.Scope != "adapter_process" || (o.Collector != "procfs" && o.Collector != "darwin_ps") || o.CollectorVersion != "1" || o.Quality != "boundary_snapshot_only" || o.Denominator != "process" {
								return fmt.Errorf("invalid native continuation process boundary provenance")
							}
							if o.Status == "available" {
								if o.Value == nil || math.IsNaN(*o.Value) || math.IsInf(*o.Value, 0) || *o.Value < 0 || *o.Value > 9007199254740991 || math.Trunc(*o.Value) != *o.Value {
									return fmt.Errorf("invalid native continuation process snapshot value")
								}
							} else if o.Value != nil || o.Reason == "" || (o.Status != "unsupported" && o.Status != "unavailable" && o.Status != "permission_denied") {
								return fmt.Errorf("invalid native continuation process snapshot availability")
							}
							seenSnapshots[o.Metric] = true
						}
					}
					if len(seenSnapshots) != 4 {
						return fmt.Errorf("incomplete native continuation process boundary snapshots")
					}
					external = append(external, event.Observations...)
				}
				if len(obs) != 7+len(external) || (len(external) > 0 && !reflect.DeepEqual(obs[7:], external)) {
					return fmt.Errorf("native continuation detached phase observations")
				}
				obs = obs[:7]
			}
			if t.Profile == "timing" {
				if len(obs) != 0 {
					return fmt.Errorf("native continuation timing contains memory instrumentation")
				}
				continue
			}
			units := map[string]string{"host.alloc.bytes": "bytes", "host.alloc.count": "count", "host.heap.start": "bytes", "host.heap.end": "bytes", "host.gc.cycles": "count", "host.gc.forced_cycles": "count", "host.gc.pause_time": "ns"}
			seen := map[string]bool{}
			for _, o := range obs {
				unit, known := units[o.Metric]
				if !known || seen[o.Metric] || o.DefinitionVersion != 1 || o.Unit != unit || o.Value == nil || math.IsNaN(*o.Value) || math.IsInf(*o.Value, 0) || *o.Value < 0 || o.Scope != "adapter_process_go_heap" || o.Phase != t.Scenario+"/operation_window" || o.Collector != "runtime.ReadMemStats" || o.CollectorVersion == "" || o.Quality != "engine_reported" || o.Profile != "memory" || o.Status != "available" || o.Denominator != protocol.ContinuationDenominator {
					return fmt.Errorf("invalid native continuation Go memory window")
				}
				seen[o.Metric] = true
			}
			if len(seen) != len(units) {
				return fmt.Errorf("incomplete native continuation memory evidence")
			}
		}
	}
	return nil
}
