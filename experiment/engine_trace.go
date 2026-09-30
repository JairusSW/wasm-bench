package experiment

import (
	"fmt"
	"github.com/wasmbench/wasmbench/protocol"
)

func ValidateEngineTraceEvidence(b Bundle) error {
	for _, t := range b.Trials {
		var w protocol.Workload
		var description *protocol.Description
		for _, candidate := range b.Manifest.Lock.Workloads {
			if candidate.ID == t.Workload {
				w = candidate
				break
			}
		}
		for _, r := range b.Manifest.Lock.Runtimes {
			if r.ID == t.Runtime {
				description = r.Description
				break
			}
		}
		if err := validateEngineTraceTrial(w, description, b.Manifest.Lock.Options, t); err != nil {
			return fmt.Errorf("trial %s: %w", t.ID, err)
		}
	}
	return nil
}

func validateEngineTraceTrial(w protocol.Workload, d *protocol.Description, o Options, t Trial) error {
	active := d != nil && d.Capabilities["can_trace_v8_wasm_events"] && d.Capabilities["can_profile_tier_trajectory"] && t.Profile == "profiling" && t.Block >= 0
	if t.EngineTrace == nil {
		if active && t.Status == "ok" {
			return fmt.Errorf("adapter omitted engine trace outcome")
		}
		return nil
	}
	if !active || o.Profile != t.Profile || t.Scenario != "trajectory" {
		return fmt.Errorf("engine trace outside declared diagnostic trial")
	}
	r := protocol.RunRequest{Scenario: t.Scenario, Samples: o.Samples, Warmup: o.Warmup, Operations: o.Operations, PhaseBarriers: o.PhaseBarriers}
	if err := protocol.ValidateEngineTraceRun(&protocol.Preparation{Workload: w, Profile: t.Profile}, &r); err != nil {
		return err
	}
	x := t.EngineTrace
	if err := x.Validate(w.SHA256); err != nil {
		return err
	}
	if x.CollectorVersion != d.Version || x.EmbeddingVersion != d.Build {
		return fmt.Errorf("engine trace versions differ from locked runtime")
	}
	if x.Status == "unavailable" {
		return nil
	}
	if len(t.Samples) != 0 && x.TrajectoryEpochNS == "" {
		return fmt.Errorf("engine trace lacks trajectory clock bridge")
	}
	if x.TrajectoryEpochNS != "" {
		epoch, _ := protocol.DecimalClockNS(x.TrajectoryEpochNS)
		end, _ := protocol.DecimalClockNS(x.EndClockNS)
		for _, s := range t.Samples {
			if s.TierWindow == nil || s.TierWindow.After.EndNS < 0 || uint64(s.TierWindow.After.EndNS) > end-epoch {
				return fmt.Errorf("tier observation extends beyond engine trace collection")
			}
		}
	}
	return nil
}
