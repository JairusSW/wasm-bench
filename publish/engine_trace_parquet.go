package publish

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/parquet-go/parquet-go"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

const EngineTraceExportVersion = "native-engine-events-parquet-v1"

// EngineTraceRow is diagnostic evidence, not an aggregate or per-function
// attribution. Every profiling trial has an outcome row, even without events.
// Native event order, duplicate metadata and absent-versus-zero fields survive.
type EngineTraceRow struct {
	ExportVersion          string  `parquet:"export_version"`
	RowKind                string  `parquet:"row_kind"`
	Run                    string  `parquet:"run"`
	Trial                  string  `parquet:"trial"`
	Runtime                string  `parquet:"runtime_configuration"`
	Workload               string  `parquet:"workload"`
	Scenario               string  `parquet:"scenario"`
	Profile                string  `parquet:"profile"`
	Block                  int64   `parquet:"block"`
	TrialStatus            string  `parquet:"trial_status"`
	TrialReason            string  `parquet:"trial_reason"`
	TraceStatus            string  `parquet:"trace_status"`
	TraceReason            string  `parquet:"trace_reason"`
	ExperimentModuleSHA256 string  `parquet:"experiment_module_sha256"`
	TraceSHA256            string  `parquet:"trace_sha256"`
	Format                 string  `parquet:"format"`
	Collector              string  `parquet:"collector"`
	CollectorVersion       string  `parquet:"collector_version"`
	EmbeddingVersion       string  `parquet:"embedding_version"`
	Scope                  string  `parquet:"scope"`
	Window                 string  `parquet:"window"`
	Quality                string  `parquet:"quality"`
	CategoriesJSON         *string `parquet:"categories_json,optional"`
	StartClockNS           *uint64 `parquet:"collection_start_ns,optional"`
	EndClockNS             *uint64 `parquet:"collection_end_ns,optional"`
	TrajectoryEpochNS      *uint64 `parquet:"trajectory_epoch_ns,optional"`
	EventCount             *uint64 `parquet:"delivered_event_count,optional"`
	EventIndex             *uint64 `parquet:"event_index,optional"`
	PID                    *int64  `parquet:"native_pid,optional"`
	TID                    *int64  `parquet:"native_tid,optional"`
	Phase                  *string `parquet:"native_phase,optional"`
	Category               *string `parquet:"native_category,optional"`
	Name                   *string `parquet:"native_name,optional"`
	TimestampUS            *uint64 `parquet:"native_timestamp_us,optional"`
	DurationUS             *uint64 `parquet:"native_duration_us,optional"`
	ThreadDurationUS       *uint64 `parquet:"native_thread_duration_us,optional"`
	ArgsJSON               *string `parquet:"native_args_json,optional"`
	NativeEventJSON        *string `parquet:"native_event_json,optional"`
}

func ExportEngineTraces(b experiment.Bundle, path string) (err error) {
	if err = experiment.ValidateEngineTraceEvidence(b); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	return writeEngineTraceParquet(b, f)
}

func writeEngineTraceParquet(b experiment.Bundle, out io.Writer) (err error) {
	w := parquet.NewGenericWriter[EngineTraceRow](out, reportParquetWriterOption())
	defer func() { err = errors.Join(err, w.Close()) }()
	return eachEngineTraceRow(b, func(r EngineTraceRow) error { _, e := w.Write([]EngineTraceRow{r}); return e })
}

func traceNumber(v string) *uint64 {
	if v == "" {
		return nil
	}
	n, _ := protocol.DecimalClockNS(v)
	return &n
}

func eachEngineTraceRow(b experiment.Bundle, write func(EngineTraceRow) error) error {
	for _, t := range b.Trials {
		if t.Profile != "profiling" && t.EngineTrace == nil {
			continue
		}
		r := EngineTraceRow{ExportVersion: EngineTraceExportVersion, RowKind: "trial_outcome", Run: b.Manifest.ID, Trial: t.ID, Runtime: t.Runtime, Workload: t.Workload, Scenario: t.Scenario, Profile: t.Profile, Block: int64(t.Block), TrialStatus: t.Status, TrialReason: t.Reason}
		for _, w := range b.Manifest.Lock.Workloads {
			if w.ID == t.Workload {
				r.ExperimentModuleSHA256 = w.SHA256
				break
			}
		}
		x := t.EngineTrace
		if x == nil {
			r.TraceStatus, r.TraceReason = "not_recorded", "No engine trace delivered"
			if t.Block < 0 {
				r.TraceStatus, r.TraceReason = "not_applicable", "Sacrificial admission is not traced"
			} else {
				for _, runtime := range b.Manifest.Lock.Runtimes {
					if runtime.ID == t.Runtime && runtime.Description != nil && !runtime.Description.Capabilities["can_trace_v8_wasm_events"] {
						r.TraceStatus, r.TraceReason = "unsupported", "Runtime configuration does not advertise native engine events"
						break
					}
				}
			}
			if err := write(r); err != nil {
				return err
			}
			continue
		}
		r.TraceStatus, r.TraceReason = x.Status, x.Reason
		r.TraceSHA256, r.Format, r.Collector, r.CollectorVersion, r.EmbeddingVersion = x.SHA256, x.Format, x.Collector, x.CollectorVersion, x.EmbeddingVersion
		r.Scope, r.Window, r.Quality = x.Scope, x.Window, x.Quality
		categories, _ := json.Marshal(x.Categories)
		c := string(categories)
		r.CategoriesJSON = &c
		r.StartClockNS, r.EndClockNS, r.TrajectoryEpochNS = traceNumber(x.StartClockNS), traceNumber(x.EndClockNS), traceNumber(x.TrajectoryEpochNS)
		if x.Status == "unavailable" {
			if err := write(r); err != nil {
				return err
			}
			continue
		}
		events, err := x.Events()
		if err != nil {
			return err
		}
		var native struct {
			Events []json.RawMessage `json:"traceEvents"`
		}
		if err := json.Unmarshal(x.Data, &native); err != nil {
			return err
		}
		count := uint64(len(events))
		r.EventCount = &count
		if err := write(r); err != nil {
			return err
		}
		for i, e := range events {
			row := r
			row.RowKind = "native_event"
			row.EventCount = nil // Count is one trial outcome, not one repeated count per event.
			index := uint64(i)
			row.EventIndex = &index
			row.PID, row.TID, row.Phase, row.Category, row.Name = &e.PID, &e.TID, &e.Phase, &e.Category, &e.Name
			row.TimestampUS = traceNumber(e.Timestamp.String())
			if e.Duration != nil {
				row.DurationUS = traceNumber(e.Duration.String())
			}
			if e.ThreadDuration != nil {
				row.ThreadDurationUS = traceNumber(e.ThreadDuration.String())
			}
			if len(e.Args) > 0 {
				args := string(e.Args)
				row.ArgsJSON = &args
			}
			raw := string(native.Events[i])
			row.NativeEventJSON = &raw
			if err := write(row); err != nil {
				return err
			}
		}
	}
	return nil
}

func verifyEngineTraceParquet(b experiment.Bundle, root string) error {
	if err := experiment.ValidateEngineTraceEvidence(b); err != nil {
		return err
	}
	sum := sha256.New()
	if err := writeEngineTraceParquet(b, sum); err != nil {
		return err
	}
	recorded, err := experiment.DigestFile(filepath.Join(root, "engine-events.parquet"))
	if err != nil {
		return err
	}
	if recorded != hex.EncodeToString(sum.Sum(nil)) {
		return fmt.Errorf("engine-event Parquet differs from raw evidence and export version")
	}
	return nil
}
