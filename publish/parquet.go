package publish

import (
	"encoding/json"
	"github.com/parquet-go/parquet-go"
	"github.com/wasmbench/wasmbench/analysis"
	"github.com/wasmbench/wasmbench/experiment"
	"os"
)

const SampleExportVersion = "sample-evidence-parquet-v11"

type SampleRow struct {
	SnapshotDensityJSON     *string `parquet:"snapshot_density_evidence_json,optional"`
	SnapshotMemoryJSON      *string `parquet:"snapshot_memory_evidence_json,optional"`
	ProcessSnapshotJSON     *string `parquet:"process_snapshot_result_json,optional"`
	ContinuationJSON        *string `parquet:"continuation_result_json,optional"`
	TierWindowJSON          *string `parquet:"tier_window_json,optional"`
	GuestDensityJSON        *string `parquet:"guest_density_evidence_json,optional"`
	ReleasePolicy           *string `parquet:"logical_release_policy,optional"`
	PostCollectionRequested *bool   `parquet:"post_collection_requested,optional"`
	PostCollectionStartNS   *int64  `parquet:"post_collection_start_ns,optional"`
	PostCollectionEndNS     *int64  `parquet:"post_collection_end_ns,optional"`
	PostCollectionPolicy    *string `parquet:"post_collection_policy,optional"`
	SustainedTargetNS       *int64  `parquet:"sustained_target_api_ns,optional"`
	SessionWindowStartNS    *int64  `parquet:"session_window_start_ns,optional"`
	SessionOperationStartNS *int64  `parquet:"session_operation_start_ns,optional"`
	SessionOperationEndNS   *int64  `parquet:"session_operation_end_ns,optional"`
	SessionWindowEndNS      *int64  `parquet:"session_window_end_ns,optional"`
	ReleaseStartNS          *int64  `parquet:"logical_release_start_ns,optional"`
	ReleaseEndNS            *int64  `parquet:"logical_release_end_ns,optional"`
	Released                *bool   `parquet:"logically_released,optional"`
	ExportVersion           string  `parquet:"export_version"`
	LatencyEligible         bool    `parquet:"latency_eligible"`
	LatencyStatus           string  `parquet:"latency_status"`
	LatencyReason           string  `parquet:"latency_reason"`
	Run                     string  `parquet:"run"`
	Trial                   string  `parquet:"trial"`
	Runtime                 string  `parquet:"runtime"`
	Workload                string  `parquet:"workload"`
	Scenario                string  `parquet:"scenario"`
	Profile                 string  `parquet:"profile"`
	Block                   int64   `parquet:"block"`
	Status                  string  `parquet:"status"`
	Reason                  string  `parquet:"reason"`
	SampleIndex             *int64  `parquet:"sample_index,optional"`
	Warmup                  bool    `parquet:"warmup"`
	ElapsedNS               *int64  `parquet:"elapsed_ns,optional"`
	Operations              *int64  `parquet:"operations,optional"`
	SampleType              string  `parquet:"sample_type"`
	Verified                bool    `parquet:"verified"`
}

// ExportParquet retains failed trials as rows with null measurements, not zero.
func ExportParquet(b experiment.Bundle, path string) error {
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if e != nil {
		return e
	}
	defer f.Close()
	writer := parquet.NewGenericWriter[SampleRow](f, reportParquetWriterOption())
	policy := analysis.HeadlineLatencyPolicy(b.Manifest)
	for _, t := range b.Trials {
		r := SampleRow{Run: b.Manifest.ID, Trial: t.ID, Runtime: t.Runtime, Workload: t.Workload, Scenario: t.Scenario, Profile: t.Profile, Block: int64(t.Block), Status: t.Status, Reason: t.Reason}
		if t.SnapshotMemory != nil {
			encoded, err := json.Marshal(t.SnapshotMemory)
			if err != nil {
				return err
			}
			value := string(encoded)
			r.SnapshotMemoryJSON = &value
		}
		if t.SnapshotDensity != nil {
			encoded, err := json.Marshal(t.SnapshotDensity)
			if err != nil {
				return err
			}
			value := string(encoded)
			r.SnapshotDensityJSON = &value
		}
		if t.Scenario == "sustained" {
			requested := b.Manifest.Lock.Options.SustainedPostCollection
			r.PostCollectionRequested = &requested
			target := int64(b.Manifest.Lock.Options.SustainedDuration)
			r.SustainedTargetNS = &target
		}
		p := policy.ForProfile(t.Profile)
		r.ExportVersion, r.LatencyStatus, r.LatencyReason = SampleExportVersion, p.Status, p.Reason
		if len(t.Samples) == 0 {
			if _, e = writer.Write([]SampleRow{r}); e != nil {
				return e
			}
			continue
		}
		for _, s := range t.Samples {
			x := r
			idx, elapsed, operations := int64(s.Index), s.ElapsedNS, int64(s.Operations)
			x.SampleIndex = &idx
			x.ElapsedNS = &elapsed
			x.Operations = &operations
			x.Warmup = s.Warmup
			x.SampleType = s.SampleType
			x.Verified = s.Verified
			if s.ProcessSnapshotResult != nil {
				encoded, err := json.Marshal(s.ProcessSnapshotResult)
				if err != nil {
					return err
				}
				value := string(encoded)
				x.ProcessSnapshotJSON = &value
			}
			if s.ContinuationResult != nil {
				encoded, err := json.Marshal(s.ContinuationResult)
				if err != nil {
					return err
				}
				value := string(encoded)
				x.ContinuationJSON = &value
			}
			if s.TierWindow != nil {
				encoded, err := json.Marshal(s.TierWindow)
				if err != nil {
					return err
				}
				value := string(encoded)
				x.TierWindowJSON = &value
			}
			if s.GuestDensityResult != nil {
				encoded, err := json.Marshal(s.GuestDensityResult)
				if err != nil {
					return err
				}
				value := string(encoded)
				x.GuestDensityJSON = &value
			}
			if p := s.SustainedWindow; p != nil {
				x.SessionWindowStartNS = &p.StartNS
				x.SessionOperationStartNS = &p.OperationStartNS
				x.SessionOperationEndNS = &p.OperationEndNS
				x.SessionWindowEndNS = &p.EndNS
			}
			if p := s.SustainedRelease; p != nil {
				policy := p.Policy
				if policy == "" {
					policy = "runtime_closed"
				}
				x.ReleasePolicy = &policy
				x.ReleaseStartNS = &p.StartNS
				x.ReleaseEndNS = &p.EndNS
				x.Released = &p.Closed
				if c := p.PostCollection; c != nil {
					x.PostCollectionStartNS, x.PostCollectionEndNS, x.PostCollectionPolicy = &c.StartNS, &c.EndNS, &c.Policy
				}
			}
			x.LatencyEligible = p.Status == "timing_pass" && t.Status == "ok" && t.Block >= 0 && !s.Warmup && s.Verified && s.Operations > 0 && s.ElapsedNS >= 0
			if _, e = writer.Write([]SampleRow{x}); e != nil {
				return e
			}
		}
	}
	return writer.Close()
}
