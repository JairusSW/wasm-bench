package collectors

import (
	"os"
	"testing"
)

func TestDarwinProcessRSSSnapshot(t *testing.T) {
	observations := Snapshot(os.Getpid(), "steady/after_batch")
	for _, observation := range observations {
		if observation.Metric != "process.rss" {
			continue
		}
		if observation.Status != "available" || observation.Value == nil || *observation.Value <= 0 {
			t.Fatalf("current RSS unavailable: %+v", observation)
		}
		if observation.Unit != "bytes" || observation.Phase != "steady/after_batch" || observation.Quality != "boundary_snapshot_only" {
			t.Fatalf("incorrect RSS metadata: %+v", observation)
		}
		return
	}
	t.Fatal("missing current RSS observation")
}

func TestDarwinProcessSizes(t *testing.T) {
	values, err := parseDarwinProcessSizes(" 42 3168 435312512\n", 42)
	if err != nil || values["process.rss"] != 3168*1024 || values["process.virtual"] != 435312512*1024 {
		t.Fatalf("wrong KiB conversion: %v, %v", values, err)
	}
	for _, output := range []string{"", "41 1 2", "42 -1 2", "42 1 nope", "42 1 2 3"} {
		if _, err := parseDarwinProcessSizes(output, 42); err == nil {
			t.Fatalf("accepted malformed ps output %q", output)
		}
	}
}

func TestDarwinMissingProcess(t *testing.T) {
	for _, observation := range Snapshot(0, "steady/after_batch") {
		if observation.Value != nil || observation.Reason == "" {
			t.Fatalf("invented missing process value: %+v", observation)
		}
		if observation.Collector != "darwin_ps" {
			t.Fatalf("incorrect collector: %+v", observation)
		}
	}
}
