//go:build linux

package agent

import (
	"os"
	"testing"

	"github.com/wasmbench/wasmbench/collectors"
)

func TestActiveControllerLiveReadOnly(t *testing.T) {
	facts := map[string]HostFact{}
	readActiveControllerThreads(os.DirFS("/"), facts)
	inventory := facts["controller/before/threads"]
	if inventory.Status != "available" || inventory.Value == nil {
		t.Fatal("live controller inventory unavailable", inventory)
	}
	// This is a read-only raw-source/rejection test, not a host isolation test.
	// Select an actually allowed CPU to prove controller overlap is rejected.
	for key, fact := range facts {
		if key == "controller/before/threads" || key == "controller/after/threads" || fact.Value == nil {
			continue
		}
		cpus, err := collectors.ParseCPUList(*fact.Value)
		if err != nil {
			t.Fatal(err)
		}
		status, reason := checkActiveControllerThreads(facts, cpus[:1])
		if status != "not_ready" {
			t.Fatal("live allowed controller CPU overlap not rejected", status, reason)
		}
		t.Log("actual controller affinity overlap rejected:", reason)
		return
	}
	t.Fatal("live controller masks missing")
}
