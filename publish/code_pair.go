package publish

import (
	"github.com/wasmbench/wasmbench/experiment"
	"math"
)

type CodeSource struct {
	ID      string `json:"id"`
	Profile string `json:"profile"`
	Note    string `json:"note"`
}

// CodeRecord points at a separately sealed code-pass record. ImageBytes is a
// mixed native image, never a guest-function instruction count.
type CodeRecord struct {
	Runtime    string  `json:"runtime"`
	Workload   string  `json:"workload"`
	Trial      string  `json:"trial"`
	Status     string  `json:"status"`
	Reason     string  `json:"reason,omitempty"`
	ImageBytes *int    `json:"image_bytes"`
	Index      int     `json:"report_record_index"`
	SizeBytes  *uint64 `json:"size_bytes,omitempty"`
	SizeNote   string  `json:"size_note,omitempty"`
}

func pairedCodeRecords(code experiment.Bundle, matched map[string]bool) []CodeRecord {
	records := []CodeRecord{}
	eligible := experiment.HostBaselineAllowsMeasurements(code.Manifest)
	for i, trial := range code.Trials {
		if trial.Block < 0 || trial.Profile != "code" || trial.Scenario != "compile" || !matched[trial.Runtime+"\x00"+trial.Workload] {
			continue
		}
		native := nativeRecord(trial, i)
		record := CodeRecord{Runtime: trial.Runtime, Workload: trial.Workload, Trial: trial.ID, Status: native.Status, Reason: native.Reason, ImageBytes: native.Bytes, Index: i}
		if eligible && trial.Status == "ok" {
			var sizes []uint64
			var notes []string
			for _, observation := range trial.Observations {
				if observation.Metric != "native.code_size" {
					continue
				}
				if observation.DefinitionVersion != 1 || observation.Status != "available" || observation.Value == nil || observation.Unit != "bytes" || observation.Scope != "compiled_module" || observation.Profile != "code" || observation.Phase != "compile" || observation.Quality != "engine_reported" || observation.Denominator != "module" || observation.Collector == "" || observation.CollectorVersion == "" {
					continue
				}
				value := *observation.Value
				if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1<<53 || math.Trunc(value) != value {
					continue
				}
				sizes = append(sizes, uint64(value))
				notes = append(notes, observation.Reason)
			}
			if len(sizes) == 1 && (native.Bytes == nil || uint64(*native.Bytes) == sizes[0]) {
				record.SizeBytes = &sizes[0]
				record.SizeNote = notes[0]
			}
		}
		if !eligible {
			record.Status = "withheld_host_mismatch"
			record.Reason = "code pass failed its observed host or CPU-partition policy"
			record.ImageBytes = nil
		}
		records = append(records, record)
	}
	return records
}
