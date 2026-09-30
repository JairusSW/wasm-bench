package experiment_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/experiment"
)

func TestBuiltAdapterToolArchiveReplay(t *testing.T) {
	ids := os.Getenv("WASMBENCH_ARCHIVE_TEST_RUNTIMES")
	if ids == "" {
		t.Skip("build adapters and set WASMBENCH_ARCHIVE_TEST_RUNTIMES")
	}
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	runtimes, err := experiment.ResolveRuntimes(root, strings.Split(ids, ","))
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	workloads, err := corpus.Generate(filepath.Join(tmp, "corpus"), "core")
	if err != nil {
		t.Fatal(err)
	}
	lock, err := experiment.NewLock(experiment.Options{Suite: "core", Profile: "timing", Scenarios: []string{"first-call"}, Launches: 1, Samples: 2, Operations: 1, Timeout: 15 * time.Second}, runtimes, workloads)
	if err != nil {
		t.Fatal(err)
	}
	lock.ArchiveTools = true
	source, err := experiment.Run(context.Background(), lock, tmp, filepath.Join(tmp, "source"), func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	record, err := experiment.RestoreTools(source, filepath.Join(tmp, "restored"))
	if err != nil {
		t.Fatal(err)
	}
	var restored experiment.Lock
	if err = experiment.ReadJSON(record.Lock, &restored); err != nil {
		t.Fatal(err)
	}
	for i, r := range restored.Runtimes {
		if r.Command[0] == runtimes[i].Command[0] {
			t.Fatal("replay did not use restored executable")
		}
		for path := range r.Files {
			if !strings.HasPrefix(path, filepath.Dir(record.Lock)+string(filepath.Separator)) {
				t.Fatal("tool not restored", path)
			}
		}
	}
	replay, err := experiment.Run(context.Background(), restored, filepath.Dir(record.Lock), filepath.Join(tmp, "replayed"), func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{source, replay} {
		b, err := experiment.Load(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(b.Trials) != len(runtimes)*len(workloads)*2 {
			t.Fatal("incomplete coverage", len(b.Trials))
		}
		for _, trial := range b.Trials {
			if trial.Status != "ok" {
				t.Fatal(trial.Runtime, trial.Status, trial.Reason)
			}
		}
	}
}
