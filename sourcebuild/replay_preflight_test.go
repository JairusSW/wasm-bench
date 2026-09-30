//go:build linux || darwin

package sourcebuild

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wasmbench/wasmbench/experiment"
)

func TestSourceRunnerArchiveAndReadOnlyReplayPreflight(t *testing.T) {
	r, dir, a := fixture(t)
	l, err := Pin(r, dir, a)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "build")
	built, err := Build(context.Background(), l, out, 12*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if built.ToolTimeoutNS != 12*time.Second || built.RunnerArchiveVersion != SourceRunnerArchiveVersion {
		t.Fatal("missing replay protocol", built)
	}
	archive := filepath.Join(out, sourceRunnerPath)
	info, err := os.Stat(archive)
	if err != nil || info.Mode().Perm() != 0444 {
		t.Fatal("executable source archive", err)
	}
	digest, err := experiment.DigestFile(archive)
	if err != nil || digest != l.RunnerSHA256 {
		t.Fatal("wrong runner archive", err)
	}
	before, _ := experiment.DigestFile(filepath.Join(out, "checksums.json"))
	// Original source may disappear; replay uses the sealed snapshot instead.
	if err := os.Remove(filepath.Join(dir, "source")); err != nil {
		t.Fatal(err)
	}
	if _, err := PreflightRebuild(out, archive); err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(cwd, out)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := PreflightRebuild(relative, archive); err != nil {
		t.Fatal("relative CLI locator rejected", err)
	}
	after, _ := experiment.DigestFile(filepath.Join(out, "checksums.json"))
	if before != after {
		t.Fatal("preflight changed source")
	}
	t.Setenv("GOGC", "replay-host-mismatch")
	if _, err := PreflightRebuild(out, archive); err == nil || !strings.Contains(err.Error(), "host fingerprint") {
		t.Fatal("accepted changed host environment", err)
	}
}

func TestSourceRunnerArchiveRejectsResealedReplacement(t *testing.T) {
	r, dir, a := fixture(t)
	l, err := Pin(r, dir, a)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "build")
	if _, err := Build(context.Background(), l, out, time.Second); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(out, sourceRunnerPath)
	if err := os.Chmod(archive, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archive, []byte("wrong archived runner"), 0444); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(out, "checksums.json")); err != nil {
		t.Fatal(err)
	}
	if err := experiment.Seal(out); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(out); err == nil || !strings.Contains(err.Error(), "runner archive differs") {
		t.Fatal("accepted resealed wrong runner", err)
	}
}
