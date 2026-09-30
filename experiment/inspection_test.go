package experiment

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wasmbench/wasmbench/protocol"
)

func TestOfflineArtifactInspection(t *testing.T) {
	for _, version := range []string{"legacy", "core-structure-v2", "core-structure-v3"} {
		t.Run(version, func(t *testing.T) {
			root := t.TempDir()
			digest := strings.Repeat("a", 64)
			w := protocol.Workload{ID: "one", SHA256: digest}
			other := w
			other.ID = "two"
			m := Manifest{Lock: Lock{Workloads: []protocol.Workload{w, other}}}
			if version != "legacy" {
				m.Lock.Analyzer = &AnalyzerLock{Executable: filepath.Join(root, "absent-analyzer"), SHA256: strings.Repeat("b", 64), Profile: "default", Name: "wasmparser", Version: "0.251.0", AnalysisVersion: version}
				if err := os.Mkdir(filepath.Join(root, "validation"), 0755); err != nil {
					t.Fatal(err)
				}
				report := map[string]any{"schema": 2, "analyzer": "wasmparser", "analyzer_version": "0.251.0", "analysis_version": version, "sha256": digest, "encoding": "core-module", "validated": true, "validation_profile": "default", "validation_features": []string{"TAIL_CALL", "SIMD"}}
				if version == "core-structure-v3" {
					report["feature_probes"] = json.RawMessage(`{"method":"single-flag-removal-v1","policy_bits":"3","probes":[{"feature":"TAIL_CALL","disabled_bits":"1","remaining_bits":"2","valid_without":false,"failure":"disabled","failure_offset":3},{"feature":"SIMD","disabled_bits":"2","remaining_bits":"1","valid_without":true,"failure":null,"failure_offset":null}]}`)
				}
				if err := WriteJSON(filepath.Join(root, "validation", digest+".json"), report); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Mkdir(filepath.Join(root, "trials"), 0755); err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{"one", "two"} {
				if err := WriteJSON(filepath.Join(root, "trials", id+".json"), Trial{ID: id, Workload: id, Status: "unsupported"}); err != nil {
					t.Fatal(err)
				}
			}
			if err := WriteJSON(filepath.Join(root, "manifest.json"), m); err != nil {
				t.Fatal(err)
			}
			if err := Seal(root); err != nil {
				t.Fatal(err)
			}
			b, err := Load(root)
			if err != nil {
				t.Fatal(err)
			}
			if len(b.Admission) != 1 {
				t.Fatal("duplicate artifact summaries", b.Admission)
			}
			got, err := InspectWorkload(root, "one")
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Trials) != 1 || got.Trials[0].ID != "one" {
				t.Fatal(got.Trials)
			}
			if version == "legacy" {
				if got.Admission.Status != "not_recorded" || got.Analysis != nil || got.Analyzer != nil {
					t.Fatal(got)
				}
			} else {
				if got.Admission.Status != "validated" || !json.Valid(got.Analysis) {
					t.Fatal(got)
				}
				if version == "core-structure-v3" {
					if len(got.Admission.NecessaryFlags) != 1 || got.Admission.NecessaryFlags[0] != "TAIL_CALL" {
						t.Fatal(got.Admission)
					}
				} else if got.Admission.FeatureStatus != "not_recorded" {
					t.Fatal(got.Admission)
				}
			}
			if _, err := InspectWorkload(root, "missing"); err == nil {
				t.Fatal("unknown workload accepted")
			}
			if err := os.WriteFile(filepath.Join(root, "manifest.json"), []byte(`{}`), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := InspectWorkload(root, "one"); err == nil {
				t.Fatal("tampered evidence accepted")
			}
		})
	}
}
