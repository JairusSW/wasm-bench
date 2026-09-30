package agent

import (
	"fmt"
	"strings"
	"testing"
	"testing/fstest"
)

func TestActiveControllerBudgetsFailClosed(t *testing.T) {
	for _, mode := range []string{"missing_inventory", "invalid_thread", "too_many_threads", "oversized_status", "total_read_budget"} {
		t.Run(mode, func(t *testing.T) {
			root := fstest.MapFS{}
			switch mode {
			case "invalid_thread":
				root["proc/self/task/not-a-tid/status"] = &fstest.MapFile{Data: []byte("Cpus_allowed_list: 0")}
			case "too_many_threads":
				for i := 1; i <= 4097; i++ {
					root[fmt.Sprintf("proc/self/task/%d/status", i)] = &fstest.MapFile{Data: []byte("Cpus_allowed_list: 0")}
				}
			case "oversized_status":
				root["proc/self/task/1/status"] = &fstest.MapFile{Data: []byte(strings.Repeat("x", (64<<10)+1))}
			case "total_read_budget":
				for i := 1; i <= 150; i++ {
					root[fmt.Sprintf("proc/self/task/%d/status", i)] = &fstest.MapFile{Data: []byte("Name: " + strings.Repeat("x", 60000) + "\nCpus_allowed_list: 0\n")}
				}
			}
			facts := map[string]HostFact{}
			readActiveControllerThreads(root, facts)
			status, reason := checkActiveControllerThreads(facts, []int{2})
			if status == "ready" {
				t.Fatal("incomplete controller evidence accepted", reason)
			}
		})
	}
}
