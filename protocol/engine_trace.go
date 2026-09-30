package protocol

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
)

const MaxEngineTraceBytes = 4 << 20

// EngineTrace is native category-scoped event evidence. The requested module
// identifies the experiment, not attribution of every event to that module.
// Complete delivery is not proof of exhaustive instrumentation or idle jobs.
type EngineTrace struct {
	Version           int      `json:"version"`
	ModuleSHA256      string   `json:"module_sha256"`
	Format            string   `json:"format"`
	Collector         string   `json:"collector"`
	CollectorVersion  string   `json:"collector_version"`
	EmbeddingVersion  string   `json:"embedding_version"`
	Scope             string   `json:"scope"`
	Window            string   `json:"window"`
	Quality           string   `json:"quality"`
	Categories        []string `json:"categories"`
	Status            string   `json:"status"`
	Reason            string   `json:"reason,omitempty"`
	StartClockNS      string   `json:"start_clock_ns,omitempty"`
	EndClockNS        string   `json:"end_clock_ns,omitempty"`
	TrajectoryEpochNS string   `json:"trajectory_epoch_ns,omitempty"`
	SHA256            string   `json:"sha256,omitempty"`
	Data              []byte   `json:"data,omitempty"`
}

type EngineTraceEvent struct {
	PID            int64           `json:"pid"`
	TID            int64           `json:"tid"`
	Timestamp      json.Number     `json:"ts"`
	Duration       *json.Number    `json:"dur"`
	ThreadDuration *json.Number    `json:"tdur"`
	Phase          string          `json:"ph"`
	Category       string          `json:"cat"`
	Name           string          `json:"name"`
	Args           json.RawMessage `json:"args"`
}

func (t EngineTrace) Events() ([]EngineTraceEvent, error) {
	var raw struct {
		Events []EngineTraceEvent `json:"traceEvents"`
	}
	if err := json.Unmarshal(t.Data, &raw); err != nil {
		return nil, err
	}
	if raw.Events == nil {
		return nil, fmt.Errorf("missing native trace event array")
	}
	return raw.Events, nil
}

func DecimalClockNS(value string) (uint64, error) {
	n, err := strconv.ParseUint(value, 10, 64)
	if err != nil || strconv.FormatUint(n, 10) != value {
		return 0, fmt.Errorf("invalid canonical trace clock")
	}
	return n, nil
}

func (t EngineTrace) Validate(module string) error {
	digest, err := hex.DecodeString(module)
	if err != nil || len(digest) != 32 || t.ModuleSHA256 != module || t.Version != 1 || t.Format != "chrome-trace-event-json" || t.Collector != "node:inspector/NodeTracing" || t.CollectorVersion == "" || t.EmbeddingVersion == "" || t.Scope != "adapter_process_v8_wasm_events_without_module_attribution" || t.Window != "run_request_including_setup_warmup_verification_and_trace_flush" || t.Quality != "engine_reported" || len(t.Categories) != 1 || t.Categories[0] != "v8.wasm" {
		return fmt.Errorf("invalid engine trace identity or scope")
	}
	if t.Status == "unavailable" {
		if t.Reason == "" || len(t.Data) != 0 || t.SHA256 != "" || t.StartClockNS != "" || t.EndClockNS != "" || t.TrajectoryEpochNS != "" {
			return fmt.Errorf("invalid unavailable trace")
		}
		return nil
	}
	if (t.Status != "collected" && t.Status != "incomplete") || (t.Status == "collected" && t.Reason != "") || (t.Status == "incomplete" && t.Reason == "") || len(t.Data) == 0 || len(t.Data) > MaxEngineTraceBytes {
		return fmt.Errorf("invalid engine trace outcome or size")
	}
	start, err := DecimalClockNS(t.StartClockNS)
	if err != nil {
		return err
	}
	end, err := DecimalClockNS(t.EndClockNS)
	if err != nil || end < start {
		return fmt.Errorf("invalid trace collection clocks")
	}
	if t.TrajectoryEpochNS != "" {
		epoch, err := DecimalClockNS(t.TrajectoryEpochNS)
		if err != nil || epoch < start || epoch > end {
			return fmt.Errorf("invalid trajectory clock bridge")
		}
	}
	sum := sha256.Sum256(t.Data)
	if hex.EncodeToString(sum[:]) != t.SHA256 {
		return fmt.Errorf("trace content digest mismatch")
	}
	events, err := t.Events()
	if err != nil {
		return err
	}
	for _, e := range events {
		if e.PID <= 0 || e.TID <= 0 || e.Name == "" || len(e.Phase) != 1 || (e.Category != "v8.wasm" && e.Category != "__metadata") {
			return fmt.Errorf("invalid native trace event identity")
		}
		if n, err := DecimalClockNS(e.Timestamp.String()); err != nil || n > 9007199254740991 {
			return fmt.Errorf("invalid trace event microsecond timestamp")
		}
		for _, v := range []*json.Number{e.Duration, e.ThreadDuration} {
			if v != nil {
				if n, err := DecimalClockNS(v.String()); err != nil || n > 9007199254740991 {
					return fmt.Errorf("invalid trace duration")
				}
			}
		}
		if e.Phase == "X" && e.Duration == nil {
			return fmt.Errorf("complete trace interval lacks duration")
		}
	}
	return nil
}

func ValidateEngineTraceRun(p *Preparation, r *RunRequest) error {
	if err := ValidateTierRun(p, r); err != nil {
		return err
	}
	if r.Samples+r.Warmup > 10000 || len(p.Workload.Export) > 1024 {
		return fmt.Errorf("traced trajectories require at most 10000 total calls and bounded export names")
	}
	return nil
}
