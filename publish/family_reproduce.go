package publish

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/wasmbench/wasmbench/analysis"
	"github.com/wasmbench/wasmbench/experiment"
)

type reportReplayPlan struct {
	Passes    []reportReplayPass
	Preflight func(context.Context) error
	Finish    func(context.Context, map[string]string, string) error
}

func familyReplaySupported(kind string) bool {
	switch kind {
	case "comparison", "history", "aggregate", "native-export", "native-disassembly", "native-comparison", "source", "source-set", "stack":
		return true
	default:
		return false
	}
}

func newReportReplayPlan(source string) (reportReplayPlan, error) {
	var plan reportReplayPlan
	if err := VerifyAnyReport(source); err != nil {
		return plan, fmt.Errorf("source report: %w", err)
	}
	receipt, err := loadAnyBuilderReceipt(source)
	if err != nil {
		return plan, err
	}
	if receipt.Kind == "" {
		plan.Passes, err = reportReplayPasses(source)
		plan.Finish = func(_ context.Context, paths map[string]string, out string) error {
			return ReportWithPasses(paths["primary"], paths["memory"], paths["code"], out)
		}
		return plan, err
	}
	if !familyReplaySupported(receipt.Kind) {
		return plan, fmt.Errorf("measurement replay for %s reports is not implemented; source compiler measurements must also be replayed, not reused", receipt.Kind)
	}
	add := func(name, relative string) error {
		path := filepath.Join(source, relative)
		b, err := experiment.Load(path)
		if err != nil {
			return err
		}
		plan.Passes = append(plan.Passes, reportReplayPass{Name: name, Source: path, Lock: b.Manifest.Lock})
		return nil
	}
	read := func(value any) error {
		return experiment.ReadJSON(filepath.Join(source, familyDataPath(receipt.Kind)), value)
	}
	switch receipt.Kind {
	case "source", "source-set", "stack":
		return newSourceReportReplayPlan(source, receipt.Kind)
	case "comparison":
		var d comparisonPage
		if err := read(&d); err != nil {
			return plan, err
		}
		for _, side := range []string{"baseline", "candidate"} {
			if err := add(side, "raw/"+side); err != nil {
				return plan, err
			}
		}
		plan.Finish = func(_ context.Context, paths map[string]string, out string) error {
			return CompareReport(paths["baseline"], paths["candidate"], d.Comparison.BaselineConfiguration.ID, d.Comparison.CandidateConfiguration.ID, out)
		}
	case "history":
		var d analysis.HistoryReport
		if err := read(&d); err != nil {
			return plan, err
		}
		names := []string{}
		for i, e := range d.Evidence {
			want := fmt.Sprintf("raw/%03d", i)
			if e.Bundle != want {
				return plan, fmt.Errorf("invalid history evidence locator")
			}
			name := fmt.Sprintf("history-%03d", i)
			if err := add(name, want); err != nil {
				return plan, err
			}
			names = append(names, name)
		}
		plan.Finish = func(_ context.Context, paths map[string]string, out string) error {
			ordered := []string{}
			for _, name := range names {
				ordered = append(ordered, paths[name])
			}
			return HistoryReport(ordered, d.Runtime, out)
		}
	case "aggregate":
		var d aggregateDataset
		if err := read(&d); err != nil {
			return plan, err
		}
		var set analysis.AggregateSet
		if err := experiment.ReadJSON(filepath.Join(source, "set.json"), &set); err != nil {
			return plan, err
		}
		if err := add("primary", "raw"); err != nil {
			return plan, err
		}
		plan.Finish = func(_ context.Context, paths map[string]string, out string) error {
			return AggregateReport(paths["primary"], set, d.Analysis.Baseline.ID, d.Analysis.Candidate.ID, out)
		}
	case "native-export", "native-disassembly":
		if err := add("code", "raw"); err != nil {
			return plan, err
		}
		if receipt.Kind == "native-export" {
			plan.Finish = func(_ context.Context, paths map[string]string, out string) error {
				return ExportNativeCode(paths["code"], out)
			}
		} else {
			var d NativeExport
			if err := read(&d); err != nil {
				return plan, err
			}
			plan.Preflight = nativeReplayToolPreflight(d)
			plan.Finish = func(ctx context.Context, paths map[string]string, out string) error {
				return replayNativeExport(ctx, paths["code"], out, d)
			}
		}
	case "native-comparison":
		var d NativeCodeComparison
		if err := read(&d); err != nil {
			return plan, err
		}
		var exports [2]NativeExport
		for i, side := range []string{"baseline", "candidate"} {
			if err := experiment.ReadJSON(filepath.Join(source, "evidence", side, "native-code.json"), &exports[i]); err != nil {
				return plan, err
			}
			if err := add(side, "evidence/"+side+"/raw"); err != nil {
				return plan, err
			}
		}
		plan.Preflight = func(ctx context.Context) error {
			for _, e := range exports {
				if err := nativeReplayToolPreflight(e)(ctx); err != nil {
					return err
				}
			}
			return nil
		}
		plan.Finish = func(ctx context.Context, paths map[string]string, out string) error {
			// These outputs are new exports from the new code passes; old
			// images/listings are never substituted for replayed code evidence.
			exported := [2]string{}
			for i, side := range []string{"baseline", "candidate"} {
				exported[i] = filepath.Join(filepath.Dir(out), "exports", side)
				if err := replayNativeExport(ctx, paths[side], exported[i], exports[i]); err != nil {
					return err
				}
			}
			return CompareNativeCode(exported[0], exported[1], d.BaselineConfiguration.ID, d.CandidateConfiguration.ID, out)
		}
	}
	return plan, nil
}

func nativeReplayToolPreflight(e NativeExport) func(context.Context) error {
	return func(ctx context.Context) error {
		if e.Version == "native-image-export-v1" {
			return nil
		}
		if e.Version != "native-image-disassembly-v1" && e.Version != "native-image-disassembly-v2" && e.Version != "native-image-disassembly-v3" {
			return fmt.Errorf("unsupported native replay export version")
		}
		if len(e.Tools) != 2 || e.ToolTimeoutNS <= 0 {
			return fmt.Errorf("native replay lacks exact tool identities or timeout")
		}
		for _, tool := range e.Tools {
			if err := ctx.Err(); err != nil {
				return err
			}
			got, err := experiment.DigestFile(tool.Path)
			if err != nil || got != tool.SHA256 {
				return fmt.Errorf("exact recorded LLVM tool unavailable at %s; no new measurements started", tool.Path)
			}
		}
		return nil
	}
}

func replayNativeExport(ctx context.Context, run, out string, e NativeExport) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if e.Version == "native-image-export-v1" {
		return ExportNativeCode(run, out)
	}
	// Recheck identity after measurements, immediately before invoking tools.
	if err := nativeReplayToolPreflight(e)(ctx); err != nil {
		return err
	}
	return disassembleNativeModeWithTools(ctx, run, out, e.Tools[0], e.Tools[1], time.Duration(e.ToolTimeoutNS), e.Version == "native-image-disassembly-v3")
}
