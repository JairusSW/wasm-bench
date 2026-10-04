package experiment

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"time"
)

// AnalyzerLock identifies the independent executable and its accepted feature
// policy, not a runtime capability or inferred minimal feature requirement.
type AnalyzerLock struct {
	Executable      string `json:"executable"`
	SHA256          string `json:"sha256"`
	Profile         string `json:"validation_profile"`
	Name            string `json:"name"`
	Version         string `json:"version"`
	AnalysisVersion string `json:"analysis_version"`
}

// Verify checks the supported evidence contract and the pinned executable.
func (a *AnalyzerLock) Verify() error {
	if a == nil {
		return fmt.Errorf("independent analyzer required")
	}
	return a.verify()
}

// VerifyEvidence checks saved evidence without requiring the analyzer binary.
func (a *AnalyzerLock) VerifyEvidence(data []byte, digest string) error {
	if a == nil {
		return fmt.Errorf("independent analyzer required")
	}
	if err := a.validate(); err != nil {
		return err
	}
	return a.validateResult(data, digest)
}

// AnalyzeArtifact validates an exact artifact using this pinned independent
// analyzer and returns the versioned evidence for a separately sealed producer.
func (a *AnalyzerLock) AnalyzeArtifact(ctx context.Context, path, digest string) (json.RawMessage, error) {
	if a == nil {
		return nil, fmt.Errorf("independent analyzer required")
	}
	got, err := DigestFile(path)
	if err != nil {
		return nil, err
	}
	if got != digest {
		return nil, fmt.Errorf("artifact digest mismatch")
	}
	return a.analyze(ctx, path, digest)
}

func PinAnalyzer(path, profile string) (*AnalyzerLock, error) {
	abs, err := filepath.Abs(NativeExecutable(path))
	if err != nil {
		return nil, err
	}
	digest, err := DigestFile(abs)
	if err != nil {
		return nil, fmt.Errorf("build the independent analyzer first with make build or wasmbench build: %w", err)
	}
	a := &AnalyzerLock{Executable: abs, SHA256: digest, Profile: profile, Name: "wasmparser", Version: "0.251.0", AnalysisVersion: "artifact-structure-v1"}
	return a, a.validate()
}

func (a *AnalyzerLock) validate() error {
	if a == nil {
		return nil
	}
	digest, err := hex.DecodeString(a.SHA256)
	if err != nil || len(digest) != 32 || !recordedPathAbsolute(a.Executable) || !slices.Contains([]string{"default", "wasm1", "wasm2", "wasm3", "all"}, a.Profile) || a.Name != "wasmparser" || a.Version != "0.251.0" || !slices.Contains([]string{"core-structure-v2", "core-structure-v3", "artifact-structure-v1"}, a.AnalysisVersion) {
		return fmt.Errorf("invalid or unsupported analyzer lock")
	}
	return nil
}

func (a *AnalyzerLock) verify() error {
	if a == nil {
		return nil
	}
	if err := a.validate(); err != nil {
		return err
	}
	digest, err := DigestFile(a.Executable)
	if err != nil {
		return err
	}
	if digest != a.SHA256 {
		return fmt.Errorf("independent analyzer executable differs from locked SHA-256")
	}
	return nil
}

type boundedAnalysisOutput struct {
	buffer bytes.Buffer
	limit  int
}

func (b *boundedAnalysisOutput) Bytes() []byte  { return b.buffer.Bytes() }
func (b *boundedAnalysisOutput) String() string { return b.buffer.String() }

func (b *boundedAnalysisOutput) Write(p []byte) (int, error) {
	if len(p) > b.limit-b.buffer.Len() {
		return 0, fmt.Errorf("analyzer output exceeds %d bytes", b.limit)
	}
	return b.buffer.Write(p)
}

func (a *AnalyzerLock) analyze(ctx context.Context, artifact, digest string) (json.RawMessage, error) {
	if err := a.verify(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, a.Executable, artifact, a.Profile)
	stdout := &boundedAnalysisOutput{limit: 64 << 20}
	stderr := &boundedAnalysisOutput{limit: 64 << 10}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("independent validation (%s): %w: %s", a.Profile, err, stderr.String())
	}
	if err := a.verify(); err != nil {
		return nil, err
	}
	if err := a.validateResult(stdout.Bytes(), digest); err != nil {
		return nil, err
	}
	return json.RawMessage(stdout.Bytes()), nil
}

func (a *AnalyzerLock) validateResult(data []byte, digest string) error {
	var result struct {
		Schema          int                   `json:"schema"`
		Analyzer        string                `json:"analyzer"`
		Version         string                `json:"analyzer_version"`
		AnalysisVersion string                `json:"analysis_version"`
		SHA256          string                `json:"sha256"`
		Profile         string                `json:"validation_profile"`
		Encoding        string                `json:"encoding"`
		Validated       bool                  `json:"validated"`
		Features        []string              `json:"validation_features"`
		Probes          *featureProbeEvidence `json:"feature_probes"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("invalid analyzer JSON: %w", err)
	}
	versionOK := result.AnalysisVersion == a.AnalysisVersion && result.Encoding == "core-module"
	if a.AnalysisVersion == "artifact-structure-v1" {
		versionOK = result.AnalysisVersion == "core-structure-v3" && result.Encoding == "core-module" || result.AnalysisVersion == "component-structure-v1" && result.Encoding == "component"
	}
	if result.Schema != 2 || result.Analyzer != a.Name || result.Version != a.Version || !versionOK || result.SHA256 != digest || result.Profile != a.Profile || !result.Validated || len(result.Features) == 0 {
		return fmt.Errorf("analyzer evidence does not match locked artifact, policy, or analysis version")
	}
	if result.AnalysisVersion == "component-structure-v1" {
		if err := validateComponentTree(data, digest); err != nil {
			return err
		}
	}
	if a.AnalysisVersion != "core-structure-v2" {
		return result.Probes.validate(result.Features)
	}
	return nil
}

type featureProbeEvidence struct {
	Method     string `json:"method"`
	PolicyBits string `json:"policy_bits"`
	Probes     []struct {
		Feature       string  `json:"feature"`
		DisabledBits  string  `json:"disabled_bits"`
		RemainingBits string  `json:"remaining_bits"`
		ValidWithout  *bool   `json:"valid_without"`
		Failure       *string `json:"failure"`
		FailureOffset *uint64 `json:"failure_offset"`
	} `json:"probes"`
}

// Only called after validateResult. Legacy reports do not establish requirements.
func requiredValidatorFeatures(data []byte) []string {
	var result struct {
		AnalysisVersion string                `json:"analysis_version"`
		Probes          *featureProbeEvidence `json:"feature_probes"`
	}
	if json.Unmarshal(data, &result) != nil || !slices.Contains([]string{"core-structure-v3", "component-structure-v1"}, result.AnalysisVersion) || result.Probes == nil {
		return nil
	}
	var required []string
	for _, p := range result.Probes.Probes {
		if p.ValidWithout != nil && !*p.ValidWithout {
			required = append(required, p.Feature)
		}
	}
	return required
}

func (p *featureProbeEvidence) validate(features []string) error {
	if p == nil || p.Method != "single-flag-removal-v1" || len(p.Probes) != len(features) {
		return fmt.Errorf("missing or incomplete feature probe evidence")
	}
	policy, err := strconv.ParseUint(p.PolicyBits, 16, 64)
	if err != nil || policy == 0 {
		return fmt.Errorf("invalid feature probe policy bits")
	}
	want := map[string]bool{}
	for _, f := range features {
		if f == "" || want[f] {
			return fmt.Errorf("invalid validation feature names")
		}
		want[f] = true
	}
	for _, probe := range p.Probes {
		disabled, e1 := strconv.ParseUint(probe.DisabledBits, 16, 64)
		remaining, e2 := strconv.ParseUint(probe.RemainingBits, 16, 64)
		if !want[probe.Feature] || e1 != nil || e2 != nil || disabled == 0 || disabled&policy != disabled || remaining != policy&^disabled || probe.ValidWithout == nil {
			return fmt.Errorf("invalid feature removal probe")
		}
		delete(want, probe.Feature)
		if *probe.ValidWithout {
			if probe.Failure != nil || probe.FailureOffset != nil {
				return fmt.Errorf("successful feature probe carries a failure")
			}
		} else if probe.Failure == nil || *probe.Failure == "" || probe.FailureOffset == nil {
			return fmt.Errorf("failed feature probe has no validation evidence")
		}
	}
	return nil
}
