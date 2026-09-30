package publish

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/wasmbench/wasmbench/experiment"
)

const ReportBuilderVersion = "sealed-report-builder-v1"
const reportBuilderPath = "builder/wasmbench"

type ReportBuilderReceipt struct {
	Kind            string `json:"kind,omitempty"`
	PageSHA256      string `json:"page_sha256,omitempty"`
	Version         string `json:"version"`
	SHA256          string `json:"sha256"`
	OS              string `json:"os"`
	Arch            string `json:"arch"`
	DataSHA256      string `json:"data_sha256"`
	AnalysisVersion string `json:"analysis_version"`
	RendererSHA256  string `json:"renderer_sha256"`
}

func archiveReportBuilder(out string, d Dataset) error {
	return archiveBuilderReceipt(out, ReportBuilderReceipt{Version: ReportBuilderVersion, OS: runtime.GOOS, Arch: runtime.GOARCH, AnalysisVersion: d.AnalysisVersion, RendererSHA256: d.RendererSHA256})
}

func archiveBuilderReceipt(out string, receipt ReportBuilderReceipt) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	digest, err := experiment.DigestFile(executable)
	if err != nil {
		return err
	}
	path := filepath.Join(out, reportBuilderPath)
	if err := experiment.CopyExclusive(executable, path); err != nil {
		return err
	}
	if err := os.Chmod(path, 0444); err != nil {
		return err
	}
	copied, err := experiment.DigestFile(path)
	if err != nil {
		return err
	}
	if copied != digest {
		return fmt.Errorf("report builder changed during archival")
	}
	dataDigest, err := experiment.DigestFile(filepath.Join(out, familyDataPath(receipt.Kind)))
	if err != nil {
		return err
	}
	receipt.SHA256, receipt.DataSHA256 = digest, dataDigest
	return experiment.WriteJSON(filepath.Join(out, "builder.json"), receipt)
}

// This only reads evidence. It never loads or executes the archived builder.
func verifyReportBuilder(root string, d Dataset) (ReportBuilderReceipt, error) {
	var receipt ReportBuilderReceipt
	if d.BuilderArchiveVersion == "" {
		for _, name := range []string{"builder.json", "builder"} {
			if _, err := os.Stat(filepath.Join(root, name)); err == nil {
				return receipt, fmt.Errorf("unrecorded report builder archive")
			} else if !os.IsNotExist(err) {
				return receipt, err
			}
		}
		return receipt, nil
	}
	if d.BuilderArchiveVersion != ReportBuilderVersion {
		return receipt, fmt.Errorf("unsupported report builder archive version")
	}
	if err := experiment.ReadJSON(filepath.Join(root, "builder.json"), &receipt); err != nil {
		return receipt, err
	}
	digest, err := experiment.DigestFile(filepath.Join(root, reportBuilderPath))
	if err != nil {
		return receipt, err
	}
	dataDigest, err := experiment.DigestFile(filepath.Join(root, "data.json"))
	if err != nil {
		return receipt, err
	}
	if receipt.Version != ReportBuilderVersion || receipt.SHA256 != digest || receipt.DataSHA256 != dataDigest || receipt.AnalysisVersion != d.AnalysisVersion || receipt.RendererSHA256 != d.RendererSHA256 || receipt.OS == "" || receipt.Arch == "" {
		return receipt, fmt.Errorf("report builder receipt differs from sealed evidence")
	}
	return receipt, nil
}

// Recorded-builder mode is explicit code execution, not a checksum check.
// Self-contained hashes prove consistency, not publisher trust. Only invoke
// these functions for report archives the caller trusts to execute.
func VerifyReportWithRecordedBuilder(ctx context.Context, root string) error {
	return recordedReportBuilder(ctx, root, []string{"verify-report", "--dir", root}, executeRecordedBuilder)
}

func ReproduceReportWithRecordedBuilder(ctx context.Context, source, out string) error {
	if err := reportOutsideInputs([]string{source}, out); err != nil {
		return err
	}
	return recordedReportBuilder(ctx, source, []string{"reproduce-report", "--dir", source, "--out", out}, executeRecordedBuilder)
}

func executeRecordedBuilder(ctx context.Context, executable string, args []string) error {
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}

func recordedReportBuilder(ctx context.Context, root string, args []string, execute func(context.Context, string, []string) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := experiment.Verify(root); err != nil {
		return err
	}
	receipt, err := loadAnyBuilderReceipt(root)
	if err != nil {
		return err
	}
	if receipt.Version == "" {
		return fmt.Errorf("report predates builder archival; recover its original report-builder executable separately")
	}
	if receipt.Kind != "" && !familyReplaySupported(receipt.Kind) && len(args) > 0 && args[0] == "reproduce-report" {
		return fmt.Errorf("measurement replay for %s reports is not implemented; use their copied evidence and original builder to regenerate analysis", receipt.Kind)
	}
	if receipt.OS != runtime.GOOS || receipt.Arch != runtime.GOARCH {
		return fmt.Errorf("recorded builder requires %s/%s; current host is %s/%s", receipt.OS, receipt.Arch, runtime.GOOS, runtime.GOARCH)
	}
	temp, err := os.MkdirTemp("", "wasmbench-report-builder-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	path := experiment.NativeExecutable(filepath.Join(temp, "wasmbench"))
	if err := experiment.CopyExclusive(filepath.Join(root, reportBuilderPath), path); err != nil {
		return err
	}
	if err := os.Chmod(path, 0755); err != nil {
		return err
	}
	digest, err := experiment.DigestFile(path)
	if err != nil {
		return err
	}
	if digest != receipt.SHA256 {
		return fmt.Errorf("staged report builder differs from recorded hash")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return execute(ctx, path, args)
}
