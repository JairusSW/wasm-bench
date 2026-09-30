package experiment

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/protocol"
)

// ValidateCheckpointEvidence independently rechecks exact module shape and
// phase-specific state evidence offline. It never executes adapters or Wasm.
func ValidateCheckpointEvidence(root string, b Bundle) error {
	workloads := map[string]protocol.Workload{}
	for _, w := range b.Manifest.Lock.Workloads {
		if w.Checkpoint == nil && w.Oracle.Kind != "guest_checkpoint_v1" {
			continue
		}
		if err := protocol.ValidateCheckpointWorkload(w); err != nil {
			return err
		}
		canonical, err := corpus.CheckpointModule(w.Checkpoint.Pages)
		if err != nil {
			return err
		}
		if w.SHA256 != corpus.Hash(canonical) {
			return fmt.Errorf("checkpoint canonical artifact identity mismatch")
		}
		data, err := os.ReadFile(filepath.Join(root, "artifacts", w.SHA256+".wasm"))
		if err != nil {
			return err
		}
		if !bytes.Equal(data, canonical) {
			return fmt.Errorf("checkpoint artifact does not match exact fixed-memory scalar fixture")
		}
		workloads[w.ID] = w
	}
	for _, t := range b.Trials {
		w, ok := workloads[t.Workload]
		if !ok {
			for _, s := range t.Samples {
				if s.CheckpointResult != nil {
					return fmt.Errorf("checkpoint evidence outside checkpoint workload")
				}
			}
			continue
		}
		if t.Status != "ok" {
			continue
		}
		if t.Profile != b.Manifest.Lock.Options.Profile {
			return fmt.Errorf("checkpoint trial %s: profile differs from lock", t.ID)
		}
		scenario := t.Scenario
		if t.Block < 0 {
			if scenario != "checkpoint-restore" {
				return fmt.Errorf("checkpoint trial %s: invalid sacrificial scenario", t.ID)
			}
			scenario = "checkpoint-restore"
		}
		r := trialRequest(b.Manifest.Lock.Options, w, scenario, t.Block)
		profile := t.Profile
		if t.Block < 0 && (profile == "counters" || profile == "profiling") {
			profile = "timing"
		}
		if err := protocol.ValidateCheckpoint(&protocol.Preparation{Workload: w, Profile: profile}, &r); err != nil {
			return fmt.Errorf("checkpoint trial %s: %w", t.ID, err)
		}
		if err := validateSampleSequence(r, t.Samples); err != nil {
			return err
		}
		if r.PhaseBarriers {
			stages := protocol.PhaseStages(scenario)
			if len(t.PhaseEvents) != len(t.Samples)*len(stages) {
				return fmt.Errorf("checkpoint trial %s: missing phase boundaries", t.ID)
			}
			for i, event := range t.PhaseEvents {
				if event.Event.SampleIndex != i/len(stages) || event.Event.Stage != stages[i%len(stages)] {
					return fmt.Errorf("checkpoint trial %s: reordered phase boundaries", t.ID)
				}
			}
		} else if len(t.PhaseEvents) != 0 {
			return fmt.Errorf("checkpoint trial %s: unexpected phase instrumentation", t.ID)
		}
		for _, s := range t.Samples {
			if err := protocol.VerifyCheckpointSample(w, scenario, s); err != nil {
				return fmt.Errorf("checkpoint trial %s: %w", t.ID, err)
			}
			payloads := 0
			for _, o := range s.Observations {
				if o.Metric == "checkpoint.payload_bytes" {
					payloads++
					if profile != "memory" || o.DefinitionVersion != 1 || o.Value == nil || *o.Value != float64(s.CheckpointResult.PayloadBytes) || o.Unit != "bytes" || o.Scope != "guest_state_checkpoint" || o.Phase != protocol.PhaseStages(scenario)[1] || o.Collector != "wasmbench.eager_guest_checkpoint" || o.CollectorVersion != "1" || o.Quality != "exact" || o.Profile != "memory" || o.Status != "available" || o.Denominator != "one_checkpoint" {
						return fmt.Errorf("checkpoint trial %s: invalid payload metric", t.ID)
					}
				}
			}
			if (profile == "memory" && payloads != 1) || (profile != "memory" && payloads != 0) {
				return fmt.Errorf("checkpoint trial %s: missing/duplicate payload metric or wrong profile", t.ID)
			}
		}
	}
	return nil
}
