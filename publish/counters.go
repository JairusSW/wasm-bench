package publish

import (
	"errors"
	"os"

	"github.com/parquet-go/parquet-go"
	"github.com/wasmbench/wasmbench/experiment"
)

const CounterExportVersion = "raw-perf-counters-parquet-v2"

// CounterRow is raw diagnostic evidence, not an aggregate or eligibility claim.
// Unsigned counts retain their entire 64-bit range. Missing windows/events get
// explicit placeholder rows so unsupported trials never disappear from coverage.
type CounterRow struct {
	ExportVersion          string  `parquet:"export_version"`
	RowKind                string  `parquet:"row_kind"`
	Run                    string  `parquet:"run"`
	Trial                  string  `parquet:"trial"`
	Runtime                string  `parquet:"runtime"`
	Workload               string  `parquet:"workload"`
	Scenario               string  `parquet:"scenario"`
	Profile                string  `parquet:"profile"`
	Block                  int64   `parquet:"block"`
	TrialStatus            string  `parquet:"trial_status"`
	TrialReason            string  `parquet:"trial_reason"`
	WindowIndex            *int64  `parquet:"window_index,optional"`
	SampleIndex            *int64  `parquet:"sample_index,optional"`
	SampleVerified         *bool   `parquet:"sample_verified,optional"`
	SampleWarmup           *bool   `parquet:"sample_warmup,optional"`
	SampleOperations       *int64  `parquet:"sample_operations,optional"`
	Phase                  string  `parquet:"phase"`
	WindowStatus           string  `parquet:"window_status"`
	WindowReason           string  `parquet:"window_reason"`
	WindowCollectorVersion string  `parquet:"window_collector_version"`
	ReadingIndex           *int64  `parquet:"reading_index,optional"`
	CollectorVersion       string  `parquet:"collector_version"`
	Scope                  string  `parquet:"scope"`
	PrivilegeScope         string  `parquet:"privilege_scope"`
	Event                  string  `parquet:"event"`
	EventType              *uint32 `parquet:"event_type,optional"`
	EventConfig            *uint64 `parquet:"event_config,optional"`
	CPU                    *int64  `parquet:"cpu,optional"`
	RawCount               *uint64 `parquet:"raw_count,optional"`
	EnabledNS              *uint64 `parquet:"time_enabled_ns,optional"`
	RunningNS              *uint64 `parquet:"time_running_ns,optional"`
	ReadingStatus          string  `parquet:"reading_status"`
	ReadingReason          string  `parquet:"reading_reason"`
}

func ExportCounters(b experiment.Bundle, path string) (err error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	w := parquet.NewGenericWriter[CounterRow](f, reportParquetWriterOption())
	defer func() { err = errors.Join(err, w.Close()) }()
	write := func(r CounterRow) error { _, err := w.Write([]CounterRow{r}); return err }
	return eachCounterRow(b, write)
}

func eachCounterRow(b experiment.Bundle, write func(CounterRow) error) (err error) {
	for _, t := range b.Trials {
		if t.Profile != "counters" && len(t.CounterPhases) == 0 {
			continue
		}
		base := CounterRow{ExportVersion: CounterExportVersion, RowKind: "trial_outcome", Run: b.Manifest.ID, Trial: t.ID, Runtime: t.Runtime, Workload: t.Workload, Scenario: t.Scenario, Profile: t.Profile, Block: int64(t.Block), TrialStatus: t.Status, TrialReason: t.Reason}
		if len(t.CounterPhases) == 0 {
			if err = write(base); err != nil {
				return err
			}
			continue
		}
		for pi, p := range t.CounterPhases {
			r := base
			wi, si := int64(pi), int64(p.Sample)
			r.WindowIndex = &wi
			r.SampleIndex = &si
			r.RowKind = "window_outcome"
			r.Phase = p.Phase
			r.WindowStatus = p.Status
			r.WindowReason = p.Reason
			r.WindowCollectorVersion = p.CollectorVersion
			matches := 0
			for _, s := range t.Samples {
				if s.Index == p.Sample {
					v := s.Verified
					r.SampleVerified = &v
					warmup, operations := s.Warmup, int64(s.Operations)
					r.SampleWarmup, r.SampleOperations = &warmup, &operations
					if operations < 1 {
						r.SampleOperations = nil
					}
					matches++
				}
			}
			if matches != 1 {
				r.SampleVerified = nil
				r.SampleWarmup, r.SampleOperations = nil, nil
			}
			if len(p.Readings) == 0 {
				if err = write(r); err != nil {
					return err
				}
				continue
			}
			for ri, reading := range p.Readings {
				x := r
				x.RowKind = "reading"
				idx, cpu := int64(ri), int64(reading.CPU)
				typ, config := reading.Event.Type, reading.Event.Config
				x.ReadingIndex = &idx
				x.CPU = &cpu
				x.EventType = &typ
				x.EventConfig = &config
				x.Event = reading.Event.Name
				x.CollectorVersion = reading.CollectorVersion
				x.Scope = reading.Scope
				x.PrivilegeScope = reading.PrivilegeScope
				x.RawCount = reading.Count
				x.EnabledNS = reading.EnabledNS
				x.RunningNS = reading.RunningNS
				x.ReadingStatus = reading.Status
				x.ReadingReason = reading.Reason
				if err = write(x); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
