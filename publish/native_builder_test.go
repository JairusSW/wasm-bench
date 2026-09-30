package publish

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wasmbench/wasmbench/experiment"
)

func refreshNativeReceipt(t *testing.T, root string) {
	t.Helper()
	var receipt ReportBuilderReceipt
	if err := experiment.ReadJSON(filepath.Join(root, "builder.json"), &receipt); err != nil {
		t.Fatal(err)
	}
	receipt.DataSHA256, _ = experiment.DigestFile(filepath.Join(root, familyDataPath(receipt.Kind)))
	receipt.PageSHA256, _ = experiment.DigestFile(filepath.Join(root, "index.html"))
	if err := os.Remove(filepath.Join(root, "builder.json")); err != nil {
		t.Fatal(err)
	}
	if err := experiment.WriteJSON(filepath.Join(root, "builder.json"), receipt); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "checksums.json")); err != nil {
		t.Fatal(err)
	}
	if err := experiment.Seal(root); err != nil {
		t.Fatal(err)
	}
}

func TestNativeBuilderArchivesAndExactRegeneration(t *testing.T) {
	root := nativeBundle(t, "code")
	out := filepath.Join(t.TempDir(), "export")
	if err := ExportNativeCode(root, out); err != nil {
		t.Fatal(err)
	}
	if err := VerifyAnyReport(out); err != nil {
		t.Fatal(err)
	}
	r, err := loadAnyBuilderReceipt(out)
	if err != nil || r.Kind != "native-export" {
		t.Fatal(r, err)
	}
	info, err := os.Stat(filepath.Join(out, reportBuilderPath))
	if err != nil || info.Mode().Perm() != 0444 {
		t.Fatal("archive must remain nonexecutable", err)
	}
	called := false
	if err := recordedReportBuilder(context.Background(), out, []string{"verify-report", "--dir", out}, func(_ context.Context, path string, _ []string) error {
		called = true
		digest, err := experiment.DigestFile(path)
		if err != nil || digest != r.SHA256 {
			t.Fatal("changed staged builder", err)
		}
		return nil
	}); err != nil || !called {
		t.Fatal(err)
	}
	called = false
	if err := recordedReportBuilder(context.Background(), out, []string{"reproduce-report", "--dir", out, "--out", filepath.Join(t.TempDir(), "replay")}, func(context.Context, string, []string) error {
		called = true
		return nil
	}); err != nil || !called {
		t.Fatal(err)
	}
	// Sealing extra output is not enough: it must belong to the regenerated set.
	if err := writeDiagnostic(filepath.Join(out, "unrelated.txt"), []byte("extra")); err != nil {
		t.Fatal(err)
	}
	refreshNativeReceipt(t, out)
	if err := VerifyAnyReport(out); err == nil || !strings.Contains(err.Error(), "file set differs") {
		t.Fatal("accepted unrelated file", err)
	}
	if err := os.Remove(filepath.Join(out, "unrelated.txt")); err != nil {
		t.Fatal(err)
	}
	// Recomputed HTML, not the receipt alone, is the page authority.
	if err := os.WriteFile(filepath.Join(out, "index.html"), []byte("forged"), 0644); err != nil {
		t.Fatal(err)
	}
	refreshNativeReceipt(t, out)
	if err := VerifyAnyReport(out); err == nil || !strings.Contains(err.Error(), "page differs") {
		t.Fatal("accepted resealed forged HTML", err)
	}
	if err := experiment.Verify(root); err != nil {
		t.Fatal("changed source", err)
	}
}

func TestNativeBuilderCannotBeRemovedOrRelabeled(t *testing.T) {
	for _, mode := range []string{"removed", "relabelled", "unknown-version"} {
		t.Run(mode, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "export")
			if err := ExportNativeCode(nativeBundle(t, "code"), out); err != nil {
				t.Fatal(err)
			}
			r, err := loadAnyBuilderReceipt(out)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(filepath.Join(out, "builder.json")); err != nil {
				t.Fatal(err)
			}
			if mode != "removed" {
				if mode == "relabelled" {
					r.Kind = "native-disassembly"
				} else {
					r.Version = "unknown"
				}
				if err := experiment.WriteJSON(filepath.Join(out, "builder.json"), r); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Remove(filepath.Join(out, "checksums.json")); err != nil {
				t.Fatal(err)
			}
			if err := experiment.Seal(out); err != nil {
				t.Fatal(err)
			}
			if err := VerifyNativeCode(out); err == nil {
				t.Fatal("accepted missing or mismatched archive")
			}
			if err := VerifyAnyReport(out); err == nil {
				t.Fatal("accepted missing or mismatched archive")
			}
		})
	}
}

func TestNativeComparisonBuilderRegeneratesNestedExports(t *testing.T) {
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
		out := filepath.Join(t.TempDir(), "export")
		if err := ExportNativeCode(root, out); err != nil {
			t.Fatal(err)
		}
		exports = append(exports, out)
	}
	out := filepath.Join(t.TempDir(), "comparison")
	if err := CompareNativeCode(exports[0], exports[1], "a", "b", out); err != nil {
		t.Fatal(err)
	}
	if err := VerifyAnyReport(out); err != nil {
		t.Fatal(err)
	}
	r, err := loadAnyBuilderReceipt(out)
	if err != nil || r.Kind != "native-comparison" {
		t.Fatal(r, err)
	}
	if err := os.WriteFile(filepath.Join(out, "index.html"), []byte("forged comparison"), 0644); err != nil {
		t.Fatal(err)
	}
	refreshNativeReceipt(t, out)
	if err := VerifyAnyReport(out); err == nil || !strings.Contains(err.Error(), "page differs") {
		t.Fatal("accepted forged comparison", err)
	}
}

func TestLLVMNativeBuilderRegeneratesWithoutToolExecution(t *testing.T) {
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
	out := filepath.Join(t.TempDir(), "disassembly")
	if err := DisassembleNativeCode(context.Background(), nativeBundle(t, "code"), out, copyPath, dumpPath, 10*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := VerifyAnyReport(out); err != nil {
		t.Fatal(err)
	}
	var saved NativeExport
	if err := experiment.ReadJSON(filepath.Join(out, "native-code.json"), &saved); err != nil {
		t.Fatal(err)
	}
	// Original paths are provenance, not commands to execute in offline checks.
	for i := range saved.Tools {
		saved.Tools[i].Path = filepath.Join(t.TempDir(), "absent-llvm")
	}
	if err := os.Remove(filepath.Join(out, "native-code.json")); err != nil {
		t.Fatal(err)
	}
	if err := experiment.WriteJSON(filepath.Join(out, "native-code.json"), saved); err != nil {
		t.Fatal(err)
	}
	page, err := renderCodeHTML(saved)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "index.html"), page, 0644); err != nil {
		t.Fatal(err)
	}
	refreshNativeReceipt(t, out)
	if err := VerifyAnyReport(out); err != nil {
		t.Fatal("verification must not need installed tools", err)
	}
}
