package publish

import (
	"fmt"
	"math"
	"os"

	"github.com/parquet-go/parquet-go"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

// ObservationRow preserves the metric's own normalization and collection
// domain. Trial-level snapshots have no sample index or operation count.
type ObservationRow struct {
	Run                    string   `parquet:"run"`
	Trial                  string   `parquet:"trial"`
	Runtime                string   `parquet:"runtime"`
	Workload               string   `parquet:"workload"`
	Scenario               string   `parquet:"scenario"`
	Profile                string   `parquet:"trial_profile"`
	Block                  int64    `parquet:"block"`
	TrialStatus            string   `parquet:"trial_status"`
	TrialReason            string   `parquet:"trial_reason"`
	SampleIndex            *int64   `parquet:"sample_index,optional"`
	Operations             *int64   `parquet:"operations,optional"`
	Warmup                 *bool    `parquet:"warmup,optional"`
	Verified               *bool    `parquet:"verified,optional"`
	ObservationIndex       int64    `parquet:"observation_index"`
	Metric                 string   `parquet:"metric"`
	DefinitionVersion      int64    `parquet:"definition_version"`
	Value                  *float64 `parquet:"value,optional"`
	Unit                   string   `parquet:"unit"`
	Scope                  string   `parquet:"scope"`
	Phase                  string   `parquet:"phase"`
	Collector              string   `parquet:"collector"`
	CollectorVersion       string   `parquet:"collector_version"`
	Quality                string   `parquet:"quality"`
	InstrumentationProfile string   `parquet:"instrumentation_profile"`
	Status                 string   `parquet:"status"`
	Reason                 string   `parquet:"reason"`
	Denominator            string   `parquet:"normalization_denominator"`
}

func ExportObservations(b experiment.Bundle, path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	w := parquet.NewGenericWriter[ObservationRow](f, reportParquetWriterOption())
	for _, t := range b.Trials {
		base := ObservationRow{Run: b.Manifest.ID, Trial: t.ID, Runtime: t.Runtime, Workload: t.Workload, Scenario: t.Scenario, Profile: t.Profile, Block: int64(t.Block), TrialStatus: t.Status, TrialReason: t.Reason}
		write := func(row ObservationRow, observations []protocol.Observation) error {
			for i, o := range observations {
				r := row
				r.ObservationIndex = int64(i)
				r.Metric = o.Metric
				r.DefinitionVersion = int64(o.DefinitionVersion)
				r.Value = o.Value
				r.Unit = o.Unit
				r.Scope = o.Scope
				r.Phase = o.Phase
				r.Collector = o.Collector
				r.CollectorVersion = o.CollectorVersion
				r.Quality = o.Quality
				r.InstrumentationProfile = o.Profile
				r.Status = o.Status
				r.Reason = o.Reason
				r.Denominator = o.Denominator
				if r.Status != "available" {
					r.Value = nil
				}
				if r.Status == "available" && (r.Value == nil || math.IsNaN(*r.Value) || math.IsInf(*r.Value, 0)) {
					return fmt.Errorf("trial %s metric %s: available observation requires finite value", t.ID, o.Metric)
				}
				if _, err := w.Write([]ObservationRow{r}); err != nil {
					return err
				}
			}
			return nil
		}
		if err = write(base, t.Observations); err != nil {
			return err
		}
		for _, s := range t.Samples {
			r := base
			idx, ops, warmup, verified := int64(s.Index), int64(s.Operations), s.Warmup, s.Verified
			r.SampleIndex = &idx
			r.Operations = &ops
			r.Warmup = &warmup
			r.Verified = &verified
			if err = write(r, s.Observations); err != nil {
				return err
			}
			if s.SustainedRelease != nil && s.SustainedRelease.PostCollection != nil {
				// A collection is not normalized by guest invocation count or warmup.
				r.Operations, r.Warmup = nil, nil
				if err = write(r, s.SustainedRelease.PostCollection.Observations); err != nil {
					return err
				}
			}
		}
	}
	return w.Close()
}
