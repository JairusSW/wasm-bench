package publish

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wasmbench/wasmbench/experiment"
)

func replayFixtureReport(t *testing.T, paired bool) string {
	t.Helper()
	primary, _ := aggregateBundle(t, false)
	inputs := map[string]string{"timing": primary}
	if paired {
		for _, profile := range []string{"memory", "code"} {
			root := filepath.Join(t.TempDir(), profile)
			if err := os.CopyFS(root, os.DirFS(primary)); err != nil {
				t.Fatal(err)
			}
			b, err := experiment.Load(root)
			if err != nil {
				t.Fatal(err)
			}
			b.Manifest.ID = profile + "-fixture"
			b.Manifest.Lock.Options.Profile = profile
			if err := os.Remove(filepath.Join(root, "manifest.json")); err != nil {
				t.Fatal(err)
			}
			if err := experiment.WriteJSON(filepath.Join(root, "manifest.json"), b.Manifest); err != nil {
				t.Fatal(err)
			}
			for _, tr := range b.Trials {
				tr.Profile, tr.Status, tr.Samples = profile, "unsupported", nil
				if err := os.Remove(filepath.Join(root, "trials", tr.ID+".json")); err != nil {
					t.Fatal(err)
				}
				if err := experiment.WriteJSON(filepath.Join(root, "trials", tr.ID+".json"), tr); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Remove(filepath.Join(root, "checksums.json")); err != nil {
				t.Fatal(err)
			}
			if err := experiment.Seal(root); err != nil {
				t.Fatal(err)
			}
			inputs[profile] = root
		}
	}
	out := filepath.Join(t.TempDir(), "source-report")
	if err := ReportWithPasses(primary, inputs["memory"], inputs["code"], out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestReportReproductionPreservesSeparatePasses(t *testing.T) {
	for _, paired := range []bool{false, true} {
		t.Run(fmt.Sprint(paired), func(t *testing.T) {
			source := replayFixtureReport(t, paired)
			before, _ := experiment.DigestFile(filepath.Join(source, "checksums.json"))
			var events []string
			out := filepath.Join(t.TempDir(), "replayed")
			err := reproduceReport(context.Background(), source, out, nil,
				func(p *reportReplayPass) error {
					events = append(events, "check:"+p.Name+":"+p.Lock.Options.Profile)
					return nil
				},
				func(ctx context.Context, p reportReplayPass, path string, log func(string)) error {
					events = append(events, "run:"+p.Name)
					return os.CopyFS(path, os.DirFS(p.Source))
				})
			if err != nil {
				t.Fatal(err)
			}
			want := []string{"check:primary:timing", "run:primary"}
			if paired {
				want = []string{"check:primary:timing", "check:memory:memory", "check:code:code", "run:primary", "run:memory", "run:code"}
			}
			if !reflect.DeepEqual(events, want) {
				t.Fatal(events)
			}
			if err := VerifyReport(filepath.Join(out, "report")); err != nil {
				t.Fatal(err)
			}
			after, _ := experiment.DigestFile(filepath.Join(source, "checksums.json"))
			if before != after {
				t.Fatal("source mutated")
			}
			if err := reproduceReport(context.Background(), source, out, nil, func(*reportReplayPass) error { return nil }, nil); err == nil {
				t.Fatal("overwrote output")
			}
		})
	}
}

func TestReportReproductionPreflightAndCancellationDoNotStartPasses(t *testing.T) {
	source := replayFixtureReport(t, true)
	for _, mode := range []string{"missing-tools", "cancelled", "inside-source", "tampered"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			out := filepath.Join(t.TempDir(), "out")
			if mode == "cancelled" {
				cancel()
			}
			if mode == "inside-source" {
				out = filepath.Join(source, "new-output")
			}
			input := source
			if mode == "tampered" {
				input = filepath.Join(t.TempDir(), "bad-source")
				if err := os.CopyFS(input, os.DirFS(source)); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(input, "index.html"), []byte("changed"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			err := reproduceReport(ctx, input, out, nil, func(p *reportReplayPass) error {
				if p.Name == "memory" {
					return fmt.Errorf("unavailable exact tools")
				}
				return nil
			}, func(context.Context, reportReplayPass, string, func(string)) error {
				t.Fatal("measurement started")
				return nil
			})
			if err == nil {
				t.Fatal("accepted invalid reproduction")
			}
			if _, err := os.Stat(out); !os.IsNotExist(err) {
				t.Fatal("output created during preflight", err)
			}
		})
	}
}

func TestReportReproductionFailurePreservesEvidenceAndStopsLaterPasses(t *testing.T) {
	source := replayFixtureReport(t, true)
	out := filepath.Join(t.TempDir(), "out")
	var executed []string
	err := reproduceReport(context.Background(), source, out, nil, func(*reportReplayPass) error { return nil }, func(ctx context.Context, p reportReplayPass, path string, log func(string)) error {
		executed = append(executed, p.Name)
		if p.Name == "memory" {
			return fmt.Errorf("fixture failure")
		}
		return os.CopyFS(path, os.DirFS(p.Source))
	})
	if err == nil || !strings.Contains(err.Error(), "partial evidence preserved") {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(executed, []string{"primary", "memory"}) {
		t.Fatal(executed)
	}
	if err := experiment.Verify(filepath.Join(out, "primary")); err != nil {
		t.Fatal("lost successful pass", err)
	}
	if _, err := os.Stat(filepath.Join(out, "report")); !os.IsNotExist(err) {
		t.Fatal("published partial report", err)
	}
}

func TestExactReportRunnerSelection(t *testing.T) {
	current := filepath.Join(t.TempDir(), "current")
	if err := os.WriteFile(current, []byte("current"), 0755); err != nil {
		t.Fatal(err)
	}
	digest, _ := experiment.DigestFile(current)
	source := t.TempDir()
	got, err := exactReportRunner(current, source, digest)
	if err != nil || got != current {
		t.Fatal(got, err)
	}
	archive := filepath.Join(source, "tools", "runner", "wasmbench")
	if err := os.MkdirAll(filepath.Dir(archive), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archive, []byte("archived"), 0755); err != nil {
		t.Fatal(err)
	}
	digest, _ = experiment.DigestFile(archive)
	got, err = exactReportRunner(current, source, digest)
	if err != nil || got != archive {
		t.Fatal(got, err)
	}
	if _, err := exactReportRunner(current, source, "wrong"); err == nil {
		t.Fatal("ran unpinned executable")
	}
	if err := os.Chmod(archive, 0444); err != nil {
		t.Fatal(err)
	}
	pass := reportReplayPass{Source: source, Runner: archive, Lock: experiment.Lock{RunnerSHA256: digest}}
	dest := filepath.Join(t.TempDir(), "runners", "primary", "wasmbench")
	got, err = executableReportRunner(pass, dest)
	if err != nil || got != dest {
		t.Fatal(got, err)
	}
	info, _ := os.Stat(archive)
	if info.Mode().Perm() != 0444 {
		t.Fatal("source runner permissions changed")
	}
	info, _ = os.Stat(dest)
	if info.Mode().Perm()&0111 == 0 {
		t.Fatal("staged runner is not executable")
	}
	if _, err := executableReportRunner(pass, dest); err == nil {
		t.Fatal("overwrote staged runner")
	}
	pass.Lock.RunnerSHA256 = "wrong"
	if _, err := executableReportRunner(pass, filepath.Join(t.TempDir(), "runner")); err == nil {
		t.Fatal("accepted changed archived runner")
	}
}
