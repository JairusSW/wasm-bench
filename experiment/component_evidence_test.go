package experiment

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wasmbench/wasmbench/protocol"
)

func TestComponentTreeEvidence(t *testing.T) {
	digest := strings.Repeat("a", 64)
	base := `{"bytes":28,"nodes":[{"id":0,"parent":null,"encoding":"component","byte_start":0,"byte_end":28,"bytes":28,"sha256":"HASH"},{"id":1,"parent":0,"encoding":"core-module","byte_start":10,"byte_end":18,"bytes":8,"sha256":"HASH"},{"id":2,"parent":0,"encoding":"component","byte_start":20,"byte_end":28,"bytes":8,"sha256":"HASH"}]}`
	base = strings.ReplaceAll(base, "HASH", digest)
	if err := validateComponentTree([]byte(base), digest); err != nil {
		t.Fatal(err)
	}
	for _, change := range [][2]string{{`"parent":0`, `"parent":9`}, {`"id":1`, `"id":0`}, {`"byte_start":20`, `"byte_start":16`}, {`"bytes":8`, `"bytes":0`}, {`"byte_end":28`, `"byte_end":29`}, {`"parent":null`, `"parent":0`}, {`"encoding":"component"`, `"encoding":"core-module"`}} {
		if validateComponentTree([]byte(strings.Replace(base, change[0], change[1], 1)), digest) == nil {
			t.Fatal("accepted malformed tree", change)
		}
	}
	if validateArtifactABI([]byte(`{"encoding":"component"}`), "core") == nil || validateArtifactABI([]byte(`{"encoding":"core-module"}`), "component") == nil {
		t.Fatal("accepted encoding/ABI mismatch")
	}
}

func TestComponentAnalyzerContractVersions(t *testing.T) {
	digest := strings.Repeat("a", 64)
	report := map[string]any{"schema": 2, "analyzer": "wasmparser", "analyzer_version": "0.251.0", "analysis_version": "component-structure-v1", "encoding": "component", "sha256": digest, "bytes": 8, "validation_profile": "default", "validated": true, "validation_features": []string{"COMPONENT_MODEL"}, "nodes": []any{map[string]any{"id": 0, "parent": nil, "encoding": "component", "byte_start": 0, "byte_end": 8, "bytes": 8, "sha256": digest}}, "feature_probes": json.RawMessage(`{"method":"single-flag-removal-v1","policy_bits":"1","probes":[{"feature":"COMPONENT_MODEL","disabled_bits":"1","remaining_bits":"0","valid_without":false,"failure":"component disabled","failure_offset":0}]}`)}
	a := &AnalyzerLock{Name: "wasmparser", Version: "0.251.0", Profile: "default", AnalysisVersion: "artifact-structure-v1"}
	data, _ := json.Marshal(report)
	if err := a.validateResult(data, digest); err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"core-structure-v2", "core-structure-v3"} {
		a.AnalysisVersion = version
		if a.validateResult(data, digest) == nil {
			t.Fatal("legacy core lock widened", version)
		}
	}
	a.AnalysisVersion = "artifact-structure-v1"
	for _, version := range []string{"component-structure-v2", "core-structure-v3", "artifact-structure-v1"} {
		report["analysis_version"] = version
		data, _ = json.Marshal(report)
		if a.validateResult(data, digest) == nil {
			t.Fatal("invalid encoding/version pair accepted", version)
		}
	}
}

func TestComponentAdmissionBundle(t *testing.T) {
	analyzer := os.Getenv("WASMBENCH_COMPONENT_ANALYZER")
	if analyzer == "" {
		t.Skip("set WASMBENCH_COMPONENT_ANALYZER to the built analyzer")
	}
	analyzer, err := filepath.Abs(analyzer)
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	artifact := filepath.Join(tmp, "empty.component.wasm")
	if err = os.WriteFile(artifact, []byte{0, 97, 115, 109, 13, 0, 1, 0}, 0600); err != nil {
		t.Fatal(err)
	}
	digest, err := DigestFile(artifact)
	if err != nil {
		t.Fatal(err)
	}
	w := protocol.Workload{Schema: 1, ID: "component/structure-only", Artifact: artifact, SHA256: digest, ABI: "component", WorkUnit: "invocation", Units: 1, Reset: "stateless", Oracle: protocol.Oracle{Kind: "unimplemented_component_contract"}, UnsupportedReason: "structure-only fixture; no workload export"}
	runtimes, err := ResolveRuntimes(root, []string{"wazero"})
	if err != nil {
		t.Fatal(err)
	}
	lock, err := NewLock(Options{Suite: "component-test", Profile: "timing", Scenarios: []string{"first-call"}, Launches: 1, Samples: 1, Operations: 1, Timeout: 10 * time.Second}, runtimes, []protocol.Workload{w})
	if err != nil {
		t.Fatal(err)
	}
	lock.Analyzer, err = PinAnalyzer(analyzer, "default")
	if err != nil {
		t.Fatal(err)
	}
	out, err := Run(context.Background(), lock, tmp, filepath.Join(tmp, "run"), func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	b, err := Load(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Admission) != 1 || b.Admission[0].Status != "validated" || b.Admission[0].FeatureStatus != "conditional_single_flag_necessity" {
		t.Fatal(b.Admission)
	}
	if len(b.Trials) != 2 {
		t.Fatal("lost sacrificial or measured unsupported cell", b.Trials)
	}
	for _, tr := range b.Trials {
		if tr.Status != "unsupported" || len(tr.Samples) != 0 {
			t.Fatal("component executed by core adapter", tr)
		}
	}
	inspection, err := InspectWorkload(out, w.ID)
	if err != nil {
		t.Fatal(err)
	}
	var evidence map[string]any
	if err = json.Unmarshal(inspection.Analysis, &evidence); err != nil {
		t.Fatal(err)
	}
	if evidence["encoding"] != "component" {
		t.Fatal(evidence)
	}
	// A shared digest must not let a differently labelled workload evade ABI checks.
	alias := w
	alias.ID = "mislabelled"
	alias.ABI = "core"
	b.Manifest.Lock.Workloads = append(b.Manifest.Lock.Workloads, alias)
	changed, err := json.Marshal(b.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(out, "manifest.json"), changed, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(filepath.Join(out, "checksums.json")); err != nil {
		t.Fatal(err)
	}
	if err = Seal(out); err != nil {
		t.Fatal(err)
	}
	if _, err = Load(out); err == nil {
		t.Fatal("resealed mixed ABI alias accepted")
	}
	lock.Workloads = append(lock.Workloads, alias)
	if _, err = Run(context.Background(), lock, tmp, filepath.Join(tmp, "bad-alias"), func(string) {}); err == nil {
		t.Fatal("run admitted core/component alias")
	}
}
