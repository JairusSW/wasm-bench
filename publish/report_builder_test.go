package publish

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/wasmbench/wasmbench/experiment"
)

func TestReportBuilderArchiveAndExplicitStaging(t *testing.T) {
	root := replayFixtureReport(t, false)
	var d Dataset
	if err := experiment.ReadJSON(filepath.Join(root, "data.json"), &d); err != nil {
		t.Fatal(err)
	}
	receipt, err := verifyReportBuilder(root, d)
	if err != nil {
		t.Fatal(err)
	}
	executable, _ := os.Executable()
	want, _ := experiment.DigestFile(executable)
	if receipt.SHA256 != want || receipt.OS != runtime.GOOS || receipt.Arch != runtime.GOARCH || d.BuilderArchiveVersion != ReportBuilderVersion {
		t.Fatal(receipt)
	}
	before, _ := experiment.DigestFile(filepath.Join(root, "checksums.json"))
	var staged string
	err = recordedReportBuilder(context.Background(), root, []string{"verify-report", "--dir", root}, func(ctx context.Context, path string, args []string) error {
		staged = path
		got, err := experiment.DigestFile(path)
		if err != nil || got != want {
			t.Fatal(got, err)
		}
		if !reflect.DeepEqual(args, []string{"verify-report", "--dir", root}) {
			t.Fatal(args)
		}
		info, _ := os.Stat(path)
		if info.Mode().Perm()&0111 == 0 {
			t.Fatal("staged verifier is not executable")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(staged); !os.IsNotExist(err) {
		t.Fatal("temporary staging not removed", err)
	}
	info, _ := os.Stat(filepath.Join(root, reportBuilderPath))
	if info.Mode().Perm()&0111 != 0 {
		t.Fatal("source archive made executable")
	}
	after, _ := experiment.DigestFile(filepath.Join(root, "checksums.json"))
	if before != after {
		t.Fatal("source changed")
	}
	// Normal verification only reads the archive, even when it is not runnable.
	if err := VerifyReport(root); err != nil {
		t.Fatal(err)
	}
}

func TestRecordedBuilderRejectsInvalidEvidenceBeforeExecution(t *testing.T) {
	source := replayFixtureReport(t, false)
	for _, mode := range []string{"binary-tamper", "receipt-mismatch", "wrong-platform", "cancelled", "missing-archive", "unrecorded-archive", "unsafe-link"} {
		t.Run(mode, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "report")
			if err := os.CopyFS(root, os.DirFS(source)); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var receipt ReportBuilderReceipt
			if err := experiment.ReadJSON(filepath.Join(root, "builder.json"), &receipt); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "binary-tamper":
				if err := os.WriteFile(filepath.Join(root, reportBuilderPath), []byte("changed"), 0644); err != nil {
					t.Fatal(err)
				}
			case "receipt-mismatch", "wrong-platform":
				if mode == "receipt-mismatch" {
					receipt.SHA256 = "different"
				} else {
					receipt.Arch = "not-this-architecture"
				}
				if err := os.Remove(filepath.Join(root, "builder.json")); err != nil {
					t.Fatal(err)
				}
				if err := experiment.WriteJSON(filepath.Join(root, "builder.json"), receipt); err != nil {
					t.Fatal(err)
				}
			case "cancelled":
				cancel()
			case "missing-archive":
				if err := os.Remove(filepath.Join(root, reportBuilderPath)); err != nil {
					t.Fatal(err)
				}
			case "unrecorded-archive":
				var d Dataset
				if err := experiment.ReadJSON(filepath.Join(root, "data.json"), &d); err != nil {
					t.Fatal(err)
				}
				d.BuilderArchiveVersion = ""
				if err := os.Remove(filepath.Join(root, "data.json")); err != nil {
					t.Fatal(err)
				}
				if err := experiment.WriteJSON(filepath.Join(root, "data.json"), d); err != nil {
					t.Fatal(err)
				}
			case "unsafe-link":
				if err := os.Remove(filepath.Join(root, reportBuilderPath)); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(source, reportBuilderPath), filepath.Join(root, reportBuilderPath)); err != nil {
					t.Fatal(err)
				}
			}
			if mode != "binary-tamper" && mode != "missing-archive" && mode != "unsafe-link" {
				if err := os.Remove(filepath.Join(root, "checksums.json")); err != nil {
					t.Fatal(err)
				}
				if err := experiment.Seal(root); err != nil {
					t.Fatal(err)
				}
			}
			err := recordedReportBuilder(ctx, root, []string{"verify-report", "--dir", root}, func(context.Context, string, []string) error { t.Fatal("executed invalid archive"); return nil })
			if err == nil {
				t.Fatal("accepted invalid archive")
			}
		})
	}
}

func TestRecordedBuilderLegacyReportRequiresOriginalExecutable(t *testing.T) {
	root := t.TempDir()
	if err := experiment.WriteJSON(filepath.Join(root, "data.json"), Dataset{}); err != nil {
		t.Fatal(err)
	}
	if err := experiment.Seal(root); err != nil {
		t.Fatal(err)
	}
	err := recordedReportBuilder(context.Background(), root, nil, func(context.Context, string, []string) error { t.Fatal("ran legacy source"); return nil })
	if err == nil || !strings.Contains(err.Error(), "predates builder archival") {
		t.Fatal(err)
	}
}
