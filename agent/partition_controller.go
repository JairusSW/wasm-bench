package agent

import (
	"fmt"
	"io"
	"io/fs"
	"slices"
	"strconv"
	"strings"

	"github.com/wasmbench/wasmbench/collectors"
)

// Inventories bracket only this non-atomic observation. Threads or affinity
// changes between observations are not excluded by this sampled contract.
func readActiveControllerThreads(root fs.FS, facts map[string]HostFact) {
	remaining := 8 << 20
	inventory := func() (HostFact, []string) {
		const name = "proc/self/task"
		f, err := root.Open(name)
		if err != nil {
			return factError("/"+name, err), nil
		}
		defer f.Close()
		rd, ok := f.(fs.ReadDirFile)
		if !ok {
			return HostFact{Source: "/" + name, Status: "read_error", Reason: "controller thread directory unavailable"}, nil
		}
		entries, err := rd.ReadDir(4097)
		if err != nil && err != io.EOF {
			return factError("/"+name, err), nil
		}
		if len(entries) == 0 || len(entries) > 4096 {
			return HostFact{Source: "/" + name, Status: "too_large", Reason: "controller thread inventory empty or exceeds 4096 threads"}, nil
		}
		ids := []string{}
		for _, entry := range entries {
			id, err := strconv.Atoi(entry.Name())
			if err != nil || id <= 0 || strconv.Itoa(id) != entry.Name() || !entry.IsDir() {
				return HostFact{Source: "/" + name, Status: "read_error", Reason: "invalid controller thread inventory"}, nil
			}
			ids = append(ids, entry.Name())
		}
		slices.Sort(ids)
		value := strings.Join(ids, ",")
		return HostFact{Source: "/" + name, Status: "available", Value: &value}, ids
	}
	for _, stage := range []string{"before", "after"} {
		fact, ids := inventory()
		facts["controller/"+stage+"/threads"] = fact
		for _, id := range ids {
			name := "proc/self/task/" + id + "/status"
			key := "controller/" + stage + "/thread/" + id
			if remaining <= 0 {
				facts[key] = HostFact{Source: "/" + name, Status: "too_large", Reason: "controller affinity sample exceeds 8 MiB read budget"}
				continue
			}
			f, err := root.Open(name)
			if err != nil {
				facts[key] = factError("/"+name, err)
				continue
			}
			limit := min(64<<10, remaining)
			b, err := io.ReadAll(io.LimitReader(f, int64(limit+1)))
			f.Close()
			remaining -= len(b)
			if err != nil {
				facts[key] = factError("/"+name, err)
			} else if len(b) > limit {
				facts[key] = HostFact{Source: "/" + name, Status: "too_large", Reason: "controller status exceeds sample read budget"}
			} else {
				value := string(b)
				facts[key] = selectedHostFields(HostFact{Source: "/" + name, Status: "available", Value: &value}, []string{"Cpus_allowed_list"})["Cpus_allowed_list"]
			}
		}
	}
}

func checkActiveControllerThreads(facts map[string]HostFact, measurement []int) (string, string) {
	var before string
	for _, stage := range []string{"before", "after"} {
		key := "controller/" + stage + "/threads"
		fact := facts[key]
		if fact.Status != "available" || fact.Value == nil {
			return "unavailable", key + " unavailable"
		}
		value := *fact.Value
		ids := strings.Split(value, ",")
		if value == "" || len(ids) > 4096 || !slices.IsSorted(ids) {
			return "invalid_evidence", "invalid controller thread inventory"
		}
		if stage == "after" && value != before {
			return "not_ready", "controller thread inventory changed during sample"
		}
		before = value
		seen := map[string]bool{}
		for _, id := range ids {
			n, err := strconv.Atoi(id)
			if err != nil || n <= 0 || strconv.Itoa(n) != id || seen[id] {
				return "invalid_evidence", "invalid controller thread inventory"
			}
			seen[id] = true
			key := "controller/" + stage + "/thread/" + id
			fact := facts[key]
			if fact.Status != "available" || fact.Value == nil {
				return "unavailable", key + " unavailable"
			}
			allowed, err := collectors.ParseCPUList(*fact.Value)
			if err != nil {
				return "invalid_evidence", key + ": " + err.Error()
			}
			for _, cpu := range allowed {
				if slices.Contains(measurement, cpu) {
					return "not_ready", fmt.Sprintf("controller thread %s may execute on measurement CPU %d", id, cpu)
				}
			}
		}
	}
	return "ready", "controller thread affinity disjoint at sample boundaries"
}
