package experiment

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/wasmbench/wasmbench/protocol"
)

// ArtifactAdmission is an offline view of sealed evidence, never a new runtime
// test or a claim that the artifact is supported by all engines.
type ArtifactAdmission struct {
	SHA256         string   `json:"sha256"`
	Status         string   `json:"status"`
	Reason         string   `json:"reason,omitempty"`
	ReportPath     string   `json:"report_path,omitempty"`
	FeatureStatus  string   `json:"feature_status"`
	NecessaryFlags []string `json:"necessary_flags,omitempty"`
}

func summarizeAdmission(a *AnalyzerLock, digest string, data []byte) ArtifactAdmission {
	s := ArtifactAdmission{SHA256: digest, Status: "validated", ReportPath: "validation/" + digest + ".json", FeatureStatus: "not_recorded"}
	if a.AnalysisVersion == "core-structure-v3" || a.AnalysisVersion == "artifact-structure-v1" {
		s.FeatureStatus = "conditional_single_flag_necessity"
		s.NecessaryFlags = requiredValidatorFeatures(data)
	}
	return s
}

type WorkloadInspection struct {
	Workload  protocol.Workload `json:"workload"`
	Analyzer  *AnalyzerLock     `json:"analyzer"`
	Admission ArtifactAdmission `json:"admission"`
	Analysis  json.RawMessage   `json:"analysis"`
	Trials    []Trial           `json:"trials"`
}

// InspectWorkload verifies the complete bundle before reading analyzer evidence.
// It works offline without the original runtime or analyzer executables.
func InspectWorkload(root, id string) (WorkloadInspection, error) {
	var result WorkloadInspection
	b, err := Load(root)
	if err != nil {
		return result, err
	}
	found := false
	for _, w := range b.Manifest.Lock.Workloads {
		if w.ID == id {
			result.Workload, found = w, true
			break
		}
	}
	if !found {
		return result, fmt.Errorf("unknown workload %q", id)
	}
	result.Analyzer = b.Manifest.Lock.Analyzer
	for _, a := range b.Admission {
		if a.SHA256 == result.Workload.SHA256 {
			result.Admission = a
			break
		}
	}
	if result.Admission.ReportPath != "" {
		result.Analysis, err = os.ReadFile(filepath.Join(root, filepath.FromSlash(result.Admission.ReportPath)))
		if err != nil {
			return result, err
		}
	}
	for _, trial := range b.Trials {
		if trial.Workload == id {
			result.Trials = append(result.Trials, trial)
		}
	}
	return result, nil
}
