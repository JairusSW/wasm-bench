package publish

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/wasmbench/wasmbench/experiment"
)

func runtimeFamilyReplayFixture(t *testing.T, kind string) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "report")
	if kind == "native-export" {
		if err := ExportNativeCode(nativeBundle(t, "code"), out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	if kind == "native-comparison" {
		a, b := nativeComparisonFixture()
		exports := []string{}
		for _, input := range []nativeComparisonInput{a, b} {
			root := t.TempDir()
			if err := experiment.WriteJSON(filepath.Join(root, "manifest.json"), input.bundle.Manifest); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(filepath.Join(root, "trials"), 0755); err != nil {
				t.Fatal(err)
			}
			for _, trial := range input.bundle.Trials {
				if err := experiment.WriteJSON(filepath.Join(root, "trials", trial.ID+".json"), trial); err != nil {
					t.Fatal(err)
				}
			}
			if err := experiment.Seal(root); err != nil {
				t.Fatal(err)
			}
			exported := filepath.Join(t.TempDir(), "export")
			if err := ExportNativeCode(root, exported); err != nil {
				t.Fatal(err)
			}
			exports = append(exports, exported)
		}
		if err := CompareNativeCode(exports[0], exports[1], "a", "b", out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	// Timing fixtures with independent run IDs; history must not collapse them.
	lifecycle := replayFixtureReport(t, false)
	a := filepath.Join(lifecycle, "raw")
	b := filepath.Join(t.TempDir(), "candidate")
	if err := os.CopyFS(b, os.DirFS(a)); err != nil {
		t.Fatal(err)
	}
	bundle, err := experiment.Load(b)
	if err != nil {
		t.Fatal(err)
	}
	bundle.Manifest.ID = "candidate-history"
	if err := os.Remove(filepath.Join(b, "manifest.json")); err != nil {
		t.Fatal(err)
	}
	if err := experiment.WriteJSON(filepath.Join(b, "manifest.json"), bundle.Manifest); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(b, "checksums.json")); err != nil {
		t.Fatal(err)
	}
	if err := experiment.Seal(b); err != nil {
		t.Fatal(err)
	}
	switch kind {
	case "comparison":
		err = CompareReport(a, b, "baseline", "candidate", out)
	case "history":
		err = HistoryReport([]string{a, b}, "baseline", out)
	case "aggregate":
		root, set := aggregateBundle(t, false)
		err = AggregateReport(root, set, "baseline", "candidate", out)
	default:
		t.Fatal("unsupported test family", kind)
	}
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestRuntimeFamilyReplayPreflightsEveryPassAndRegenerates(t *testing.T) {
	for _, kind := range []string{"comparison", "history", "aggregate", "native-export", "native-comparison"} {
		t.Run(kind, func(t *testing.T) {
			source := runtimeFamilyReplayFixture(t, kind)
			before, err := experiment.DigestFile(filepath.Join(source, "checksums.json"))
			if err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(t.TempDir(), "replayed")
			var checked, executed []string
			plan, err := newReportReplayPlan(source)
			if err != nil {
				t.Fatal(err)
			}
			err = reproduceReport(context.Background(), source, out, nil, func(p *reportReplayPass) error {
				if len(executed) != 0 {
					t.Fatal("measurement before full preflight")
				}
				checked = append(checked, p.Name)
				return nil
			}, func(_ context.Context, p reportReplayPass, path string, _ func(string)) error {
				if len(checked) != len(plan.Passes) {
					t.Fatal("not all passes checked")
				}
				executed = append(executed, p.Name)
				return os.CopyFS(path, os.DirFS(p.Source))
			})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(checked, executed) {
				t.Fatal(checked, executed)
			}
			if err := VerifyAnyReport(filepath.Join(out, "report")); err != nil {
				t.Fatal(err)
			}
			receipt, err := loadAnyBuilderReceipt(filepath.Join(out, "report"))
			if err != nil || receipt.Kind != kind {
				t.Fatal("wrong output family", receipt, err)
			}
			after, _ := experiment.DigestFile(filepath.Join(source, "checksums.json"))
			if before != after {
				t.Fatal("source changed")
			}
			if err := reproduceReport(context.Background(), source, out, nil, func(*reportReplayPass) error { return nil }, nil); err == nil {
				t.Fatal("overwrote output")
			}
		})
	}
}

func TestFamilyReplayFailureStopsBeforeOutputAndPreservesPartialEvidence(t *testing.T) {
	source := runtimeFamilyReplayFixture(t, "comparison")
	for _, mode := range []string{"preflight", "cancel", "measurement"} {
		t.Run(mode, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "out")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "cancel" {
				cancel()
			}
			var executed []string
			err := reproduceReport(ctx, source, out, nil, func(p *reportReplayPass) error {
				if mode == "preflight" && p.Name == "candidate" {
					return fmt.Errorf("candidate runner missing")
				}
				return nil
			}, func(_ context.Context, p reportReplayPass, path string, _ func(string)) error {
				executed = append(executed, p.Name)
				if mode != "measurement" {
					t.Fatal("measurement started")
				}
				if p.Name == "candidate" {
					return fmt.Errorf("deliberate candidate failure")
				}
				return os.CopyFS(path, os.DirFS(p.Source))
			})
			if err == nil {
				t.Fatal("accepted failed replay")
			}
			if mode == "measurement" {
				if !strings.Contains(err.Error(), "partial evidence preserved") || !reflect.DeepEqual(executed, []string{"baseline", "candidate"}) {
					t.Fatal(executed, err)
				}
				if err := experiment.Verify(filepath.Join(out, "baseline")); err != nil {
					t.Fatal("lost baseline evidence", err)
				}
				if _, err := os.Stat(filepath.Join(out, "report")); !os.IsNotExist(err) {
					t.Fatal("published partial comparison", err)
				}
			} else if _, err := os.Stat(out); !os.IsNotExist(err) {
				t.Fatal("created output before full preflight", err)
			}
		})
	}
}

func TestNativeReplayToolPreflightRequiresExactFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tool")
	if err := writeDiagnostic(path, []byte("not executed during preflight")); err != nil {
		t.Fatal(err)
	}
	digest, err := experiment.DigestFile(path)
	if err != nil {
		t.Fatal(err)
	}
	e := NativeExport{Version: "native-image-disassembly-v2", ToolTimeoutNS: 1, Tools: []NativeTool{{Path: path, SHA256: digest}, {Path: path, SHA256: digest}}}
	if err := nativeReplayToolPreflight(e)(context.Background()); err != nil {
		t.Fatal(err)
	}
	e.Tools[1].SHA256 = "changed"
	if err := nativeReplayToolPreflight(e)(context.Background()); err == nil {
		t.Fatal("accepted changed LLVM")
	}
	e.Tools[1].Path = filepath.Join(t.TempDir(), "absent")
	if err := nativeReplayToolPreflight(e)(context.Background()); err == nil {
		t.Fatal("accepted missing LLVM")
	}
}

func TestLLVMReportReplayCreatesNewListingsFromReplayedCodePasses(t *testing.T) {
	if os.Getenv("WASMBENCH_NATIVE_LLVM_TEST") != "1" {
		t.Skip("set WASMBENCH_NATIVE_LLVM_TEST=1 for installed LLVM integration")
	}
	copyPath := os.Getenv("WASMBENCH_LLVM_OBJCOPY")
	if copyPath == "" {
		copyPath = "/opt/homebrew/opt/llvm/bin/llvm-objcopy"
	}
	dumpPath := os.Getenv("WASMBENCH_LLVM_OBJDUMP")
	if dumpPath == "" {
		dumpPath = "/opt/homebrew/opt/llvm/bin/llvm-objdump"
	}
	comparison := runtimeFamilyReplayFixture(t, "native-comparison")
	disassembly := filepath.Join(t.TempDir(), "disassembly")
	if err := DisassembleNativeCode(context.Background(), filepath.Join(comparison, "evidence/baseline/raw"), disassembly, copyPath, dumpPath, 10*time.Second); err != nil {
		t.Fatal(err)
	}
	mixed := filepath.Join(t.TempDir(), "mixed")
	if err := CompareNativeCode(disassembly, filepath.Join(comparison, "evidence/candidate"), "a", "b", mixed); err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{disassembly, mixed} {
		out := filepath.Join(t.TempDir(), "replay")
		err := reproduceReport(context.Background(), source, out, nil, func(*reportReplayPass) error { return nil }, func(_ context.Context, p reportReplayPass, path string, _ func(string)) error {
			return os.CopyFS(path, os.DirFS(p.Source))
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := VerifyAnyReport(filepath.Join(out, "report")); err != nil {
			t.Fatal(err)
		}
		if source == mixed {
			var baseline, candidate NativeExport
			if err := experiment.ReadJSON(filepath.Join(out, "exports/baseline/native-code.json"), &baseline); err != nil {
				t.Fatal(err)
			}
			if err := experiment.ReadJSON(filepath.Join(out, "exports/candidate/native-code.json"), &candidate); err != nil {
				t.Fatal(err)
			}
			if baseline.Version != "native-image-disassembly-v2" || candidate.Version != "native-image-export-v1" {
				t.Fatal("changed independent export policies", baseline.Version, candidate.Version)
			}
		}
	}
}
