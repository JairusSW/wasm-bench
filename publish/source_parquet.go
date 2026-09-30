package publish

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/parquet-go/parquet-go"
	"github.com/wasmbench/wasmbench/sourcebuild"
)

const SourceTableVersion = "source-build-tables-v1"

type SourceBuildRow struct {
	Version            string   `parquet:"table_version"`
	ConfigSHA256       string   `parquet:"config_sha256"`
	MeasurementVersion string   `parquet:"measurement_version"`
	Profile            string   `parquet:"profile"`
	Stage              string   `parquet:"stage"`
	Variant            string   `parquet:"variant"`
	Bundle             string   `parquet:"build_bundle"`
	Block              *int64   `parquet:"block,optional"`
	Warmup             bool     `parquet:"warmup"`
	Status             string   `parquet:"status"`
	Reason             string   `parquet:"reason"`
	ArtifactSHA256     string   `parquet:"artifact_sha256"`
	WallNS             *int64   `parquet:"summed_tool_process_wall_ns,optional"`
	CPUNS              *int64   `parquet:"summed_tool_wait_cpu_ns,optional"`
	CPUStatus          string   `parquet:"cpu_status"`
	PeakBytes          *float64 `parquet:"max_step_cgroup_peak_bytes,optional"`
	MemoryStatus       string   `parquet:"memory_status"`
}

type SourceStepRow struct {
	Version      string `parquet:"table_version"`
	ConfigSHA256 string `parquet:"config_sha256"`
	Profile      string `parquet:"profile"`
	Stage        string `parquet:"stage"`
	Variant      string `parquet:"variant"`
	Bundle       string `parquet:"build_bundle"`
	Block        *int64 `parquet:"block,optional"`
	Warmup       bool   `parquet:"warmup"`
	BuildStatus  string `parquet:"build_status"`
	Index        int64  `parquet:"step_index"`
	Tool         string `parquet:"tool"`
	Log          string `parquet:"log"`
	WallNS       int64  `parquet:"process_wall_ns"`
	ContextError string `parquet:"context_error"`
	CPUNS        *int64 `parquet:"wait_cpu_ns,optional"`
	CPUStatus    string `parquet:"cpu_status"`
	OOM          *bool  `parquet:"oom_kill,optional"`
	CleanupError string `parquet:"cleanup_error"`
	EvidenceJSON string `parquet:"step_evidence_json"`
}

// ExportSourceTables requires a verified source benchmark. Admission rows never
// carry performance aggregates. Step rows are diagnostics, not replications.
// JSON retains full CPU/isolation/observation metadata without flattening scopes.
func ExportSourceTables(root, out string) error {
	if err := reportOutsideInputs([]string{root}, out); err != nil {
		return err
	}
	b, err := sourcebuild.VerifyBenchmark(root)
	if err != nil {
		return err
	}
	var builds []SourceBuildRow
	var steps []SourceStepRow
	profile := b.Config.Profile
	if profile == "" {
		profile = "timing"
	}
	appendRow := func(row SourceBuildRow, result sourcebuild.Result) error {
		row.Version, row.ConfigSHA256, row.MeasurementVersion, row.Profile = SourceTableVersion, b.ConfigSHA256, b.MeasurementVersion, profile
		builds = append(builds, row)
		for i, s := range result.Steps {
			data, err := json.Marshal(s)
			if err != nil {
				return err
			}
			x := SourceStepRow{Version: SourceTableVersion, ConfigSHA256: b.ConfigSHA256, Profile: profile, Stage: row.Stage, Variant: row.Variant, Bundle: row.Bundle, Block: row.Block, Warmup: row.Warmup, BuildStatus: row.Status, Index: int64(i), Tool: s.Tool, Log: s.Log, WallNS: s.ElapsedNS, ContextError: s.ContextError, CPUStatus: "not_collected", EvidenceJSON: string(data)}
			if s.CPU != nil {
				x.CPUNS, x.CPUStatus = s.CPU.TotalNS, s.CPU.Status
			}
			if s.Resources != nil && s.Resources.Isolation != nil {
				oom := s.Resources.OOM
				x.OOM = &oom
				x.CleanupError = s.Resources.CleanupError
			}
			steps = append(steps, x)
		}
		return nil
	}
	for _, a := range b.Admissions {
		var result sourcebuild.Result
		if a.FailedBuild != nil {
			result = *a.FailedBuild
		} else if a.ArtifactSHA256 != "" {
			result, err = sourcebuild.Verify(filepath.Join(root, a.Build))
			if err != nil {
				return err
			}
		}
		row := SourceBuildRow{Stage: "admission", Variant: b.Config.Variants[a.Variant].Recipe.ID, Bundle: a.Build, Status: a.Status, Reason: a.Reason, ArtifactSHA256: a.ArtifactSHA256, CPUStatus: "not_a_measurement", MemoryStatus: "not_a_measurement"}
		if err := appendRow(row, result); err != nil {
			return err
		}
	}
	for _, trial := range b.Trials {
		block := int64(trial.Block)
		row := SourceBuildRow{Stage: "trial", Variant: b.Config.Variants[trial.Variant].Recipe.ID, Bundle: trial.Bundle, Block: &block, Warmup: trial.Warmup, Status: trial.Status, Reason: trial.Reason, ArtifactSHA256: trial.Result.ArtifactSHA256, WallNS: trial.ToolWallNS, CPUNS: trial.ToolCPUNS, CPUStatus: trial.CPUStatus, PeakBytes: trial.MemoryPeakBytes, MemoryStatus: trial.MemoryStatus}
		if row.CPUStatus == "" {
			row.CPUStatus = "not_collected"
		}
		if row.MemoryStatus == "" {
			row.MemoryStatus = "not_collected"
		}
		if trial.Status != "ok" {
			row.CPUStatus = "failed_build"
			row.MemoryStatus = "failed_build"
		}
		if err := appendRow(row, trial.Result); err != nil {
			return err
		}
	}
	if err := writeSourceParquet(filepath.Join(out, "compiler-builds.parquet"), builds); err != nil {
		return err
	}
	return writeSourceParquet(filepath.Join(out, "compiler-steps.parquet"), steps)
}

func writeSourceParquet[T any](path string, rows []T) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	w := parquet.NewGenericWriter[T](f, reportParquetWriterOption())
	_, writeErr := w.Write(rows)
	err = errors.Join(writeErr, w.Close(), f.Close())
	if err != nil {
		return fmt.Errorf("write source table: %w", err)
	}
	return nil
}
