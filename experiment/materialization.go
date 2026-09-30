package experiment

import (
	"encoding/hex"
	"fmt"
	"path/filepath"

	"github.com/wasmbench/wasmbench/protocol"
)

// Shared by collection and offline validation: an adapter cannot certify its
// own input coverage merely by reporting internally consistent native ranges.
func validateMaterializationInput(root, sha string, m *protocol.CompileMaterialization) error {
	var source struct {
		SHA256    string  `json:"sha256"`
		Version   string  `json:"analysis_version"`
		Encoding  string  `json:"encoding"`
		Validated bool    `json:"validated"`
		Imported  *uint32 `json:"imported_functions"`
		Defined   *uint32 `json:"defined_functions"`
	}
	digest, err := hex.DecodeString(sha)
	if err != nil || len(digest) != 32 || m == nil {
		return fmt.Errorf("invalid materialization input identity")
	}
	if err := ReadJSON(filepath.Join(root, "validation", sha+".json"), &source); err != nil {
		return err
	}
	if source.SHA256 != sha || source.Version != "core-structure-v3" || source.Encoding != "core-module" || !source.Validated || source.Imported == nil || source.Defined == nil || *source.Imported != m.ImportedFunctions || *source.Defined != m.DefinedFunctions {
		return fmt.Errorf("materialization native ranges differ from independent input function coverage")
	}
	return nil
}

// The engine's full native coverage is independently checked against the sealed
// input analyzer. Counts from the runtime alone do not establish input coverage.
func ValidateMaterializationEvidence(root string, b Bundle) error {
	for _, t := range b.Trials {
		image := t.CodeImage
		claimed := image != nil && image.Materialization != nil
		if !claimed {
			if t.Scenario == "compile-materialized" && t.Block >= 0 && t.Status == "ok" {
				return fmt.Errorf("trial %s omitted materialization evidence", t.ID)
			}
			continue
		}
		if t.Profile != "code" || t.Block < 0 || t.Status != "ok" || t.Scenario != "compile-materialized" {
			return fmt.Errorf("materialization outside successful diagnostic compile trial")
		}
		var w protocol.Workload
		var d *protocol.Description
		for _, candidate := range b.Manifest.Lock.Workloads {
			if candidate.ID == t.Workload {
				w = candidate
				break
			}
		}
		for _, r := range b.Manifest.Lock.Runtimes {
			if r.ID == t.Runtime {
				d = r.Description
				break
			}
		}
		o := b.Manifest.Lock.Options
		r := protocol.RunRequest{Scenario: t.Scenario, Samples: o.Samples, Operations: o.Operations, Warmup: o.Warmup, PhaseBarriers: o.PhaseBarriers}
		if o.Profile != t.Profile || d == nil || !d.Capabilities["can_compile_materialized"] || d.Runtime != "wasmtime" || image.Backend != d.Backend || image.Materialization.CollectorVersion != d.Version {
			return fmt.Errorf("materialization differs from locked runtime or profile")
		}
		if err := protocol.ValidateMaterializedRun(&protocol.Preparation{Workload: w, Profile: t.Profile}, &r); err != nil {
			return err
		}
		if err := image.Validate(w.SHA256); err != nil {
			return err
		}
		if err := validateSampleSequence(r, t.Samples); err != nil {
			return err
		}
		if len(t.Samples) != 1 || !t.Samples[0].Verified || t.Samples[0].ElapsedNS != image.Materialization.CompileElapsedNS {
			return fmt.Errorf("materialized compile timer missing or inconsistent")
		}
		matched := false
		for _, a := range b.Admission {
			if a.SHA256 != w.SHA256 || a.Status != "validated" {
				continue
			}
			if a.ReportPath != "validation/"+w.SHA256+".json" {
				return fmt.Errorf("invalid materialization analyzer path")
			}
			matched = true
			break
		}
		if !matched {
			return fmt.Errorf("materialization omitted independent input admission")
		}
		if err := validateMaterializationInput(root, w.SHA256, image.Materialization); err != nil {
			return err
		}
	}
	return nil
}
