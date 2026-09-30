package publish

import "github.com/wasmbench/wasmbench/experiment"

type CodeSource struct {
	ID      string `json:"id"`
	Profile string `json:"profile"`
	Note    string `json:"note"`
}

// CodeRecord points at a separately sealed code-pass record. ImageBytes is a
// mixed native image, never a guest-function instruction count.
type CodeRecord struct {
	Runtime    string `json:"runtime"`
	Workload   string `json:"workload"`
	Trial      string `json:"trial"`
	Status     string `json:"status"`
	Reason     string `json:"reason,omitempty"`
	ImageBytes *int   `json:"image_bytes"`
	Index      int    `json:"report_record_index"`
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
		if !eligible {
			record.Status = "withheld_host_mismatch"
			record.Reason = "code pass failed its observed host or CPU-partition policy"
			record.ImageBytes = nil
		}
		records = append(records, record)
	}
	return records
}
