package protocol

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
)

const MaxProfileBytes = 16 << 20

// CPUProfile is sampled diagnostic evidence, never an exact counter or a claim
// of guest-only attribution. Data retains the collector's native profile format.
type CPUProfile struct {
	Version          int    `json:"version"`
	ModuleSHA256     string `json:"module_sha256"`
	Format           string `json:"format"`
	Collector        string `json:"collector"`
	CollectorVersion string `json:"collector_version"`
	Scope            string `json:"scope"`
	Window           string `json:"window"`
	Quality          string `json:"quality"`
	Status           string `json:"status"`
	Reason           string `json:"reason,omitempty"`
	SHA256           string `json:"sha256,omitempty"`
	Data             []byte `json:"data,omitempty"`
}

func (p CPUProfile) Validate(module string) error {
	digest, err := hex.DecodeString(module)
	if err != nil || len(digest) != sha256.Size || p.ModuleSHA256 != module {
		return fmt.Errorf("profile module identity mismatch")
	}
	goProfile := p.Format == "pprof-gzip" && p.Collector == "runtime/pprof" && p.Scope == "adapter_process_go_cpu"
	v8Profile := p.Format == "v8-cpuprofile-json" && p.Collector == "node:inspector/Profiler" && p.Scope == "adapter_v8_isolate_sampled_stacks"
	if p.Version != 1 || (!goProfile && !v8Profile) || p.CollectorVersion == "" || p.Window != "run_request_including_setup_warmup_verification" || p.Quality != "sampled" {
		return fmt.Errorf("unsupported CPU profile contract")
	}
	if p.Status == "unavailable" {
		if p.Reason == "" || len(p.Data) != 0 || p.SHA256 != "" {
			return fmt.Errorf("invalid unavailable profile")
		}
		return nil
	}
	if p.Status != "collected" || len(p.Data) == 0 || len(p.Data) > MaxProfileBytes {
		return fmt.Errorf("invalid collected profile size/status")
	}
	sum := sha256.Sum256(p.Data)
	if hex.EncodeToString(sum[:]) != p.SHA256 {
		return fmt.Errorf("profile digest mismatch")
	}
	if v8Profile {
		var raw struct {
			Nodes []json.RawMessage `json:"nodes"`
			Start *float64          `json:"startTime"`
			End   *float64          `json:"endTime"`
		}
		if json.Unmarshal(p.Data, &raw) != nil || raw.Nodes == nil || raw.Start == nil || raw.End == nil || *raw.Start < 0 || *raw.End < *raw.Start {
			return fmt.Errorf("invalid V8 profile JSON envelope")
		}
		return nil // Native JSON is retained; not normalized into Go CPU nanoseconds.
	}
	r, err := gzip.NewReader(bytes.NewReader(p.Data))
	if err != nil {
		return fmt.Errorf("profile gzip: %w", err)
	}
	defer r.Close()
	n, err := io.Copy(io.Discard, io.LimitReader(r, 64<<20+1))
	if err != nil || n == 0 || n > 64<<20 {
		return fmt.Errorf("profile gzip payload invalid or oversized")
	}
	// This validates transport integrity, not protobuf semantics or symbol coverage.
	// Independent pprof tooling interprets the retained collector artifact.
	return nil
}

func (p CPUProfile) Extension() string {
	if p.Format == "v8-cpuprofile-json" {
		return ".cpuprofile"
	}
	return ".pprof"
}

func ValidateProfilingRun(p *Preparation, r *RunRequest) error {
	if p == nil || r == nil || p.Profile != "profiling" {
		return fmt.Errorf("profiling preparation required")
	}
	w := p.Workload
	if w.ABI != "core" || w.Command != nil || w.Vectors != nil || w.Density != nil || w.Oracle.Kind != "exact_u64" || w.Reset != "stateless" || w.Export == "" || r.Scenario != "steady" || r.PhaseBarriers {
		return fmt.Errorf("CPU profiling requires stateless core exact-scalar steady execution without phase barriers")
	}
	if r.Samples < 1 || r.Samples > 100000 || r.Operations < 1 || r.Operations > 1000000 || r.Warmup < 0 || r.Warmup > 100000 {
		return fmt.Errorf("profiling budget outside protocol bounds")
	}
	return nil
}
