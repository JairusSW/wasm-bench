package agent

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func irqFixture() fstest.MapFS {
	m := fstest.MapFS{}
	for name, value := range map[string]string{
		"sys/devices/system/cpu/online":       "0-3",
		"proc/irq/default_smp_affinity":       "00000003",
		"proc/irq/44/smp_affinity_list":       "0-1",
		"proc/irq/44/effective_affinity_list": "1",
	} {
		m[name] = &fstest.MapFile{Data: []byte(value)}
	}
	return m
}

func irqRequest() IRQAffinityProbe {
	return IRQAffinityProbe{Version: IRQAffinityVersion, Scope: IRQAffinityScope, At: time.Now().UTC(), CPUs: "2-3", Facts: map[string]HostFact{}}
}

func TestIRQAffinityReadOnly(t *testing.T) {
	root := irqFixture()
	before, _ := json.Marshal(root)
	p := readIRQAffinity(root, irqRequest())
	if p.Status != "disjoint_at_observed_boundaries" || len(p.Facts) != 10 {
		t.Fatal(p.Status, p.Reason, len(p.Facts))
	}
	if err := ValidateIRQAffinityProbe(p); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(root)
	if string(before) != string(after) {
		t.Fatal("probe modified sources")
	}
	p.Status = "ready"
	if err := ValidateIRQAffinityProbe(p); err == nil {
		t.Fatal("trusted forged verdict")
	}
}

func TestIRQAffinityRefusals(t *testing.T) {
	for _, tc := range []struct{ name, file, value, status string }{
		{"default overlap", "proc/irq/default_smp_affinity", "f", "not_ready"},
		{"requested overlap", "proc/irq/44/smp_affinity_list", "2", "not_ready"},
		{"effective overlap", "proc/irq/44/effective_affinity_list", "3", "not_ready"},
		{"offline", "sys/devices/system/cpu/online", "0-2", "not_ready"},
		{"zero default", "proc/irq/default_smp_affinity", "0", "invalid_evidence"},
		{"bad default", "proc/irq/default_smp_affinity", "0x3", "invalid_evidence"},
		{"bad effective", "proc/irq/44/effective_affinity_list", "1-0", "invalid_evidence"},
		{"oversized", "proc/irq/44/effective_affinity_list", strings.Repeat("1", (64<<10)+1), "unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := irqFixture()
			root[tc.file].Data = []byte(tc.value)
			p := readIRQAffinity(root, irqRequest())
			if p.Status != tc.status {
				t.Fatal(p.Status, p.Reason)
			}
			if err := ValidateIRQAffinityProbe(p); err != nil {
				t.Fatal(err)
			}
		})
	}
	root := irqFixture()
	delete(root, "proc/irq/44/effective_affinity_list")
	p := readIRQAffinity(root, irqRequest())
	if p.Status != "unavailable" {
		t.Fatal(p)
	}
	for _, tc := range []struct{ key, value, status string }{
		{"after/online", "0-4", "not_ready"},
		{"after/inventory", "[]", "not_ready"},
		{"before/inventory", "null", "invalid_evidence"},
		{"before/inventory", `["44","44"]`, "invalid_evidence"},
		{"before/inventory", `["044"]`, "invalid_evidence"},
	} {
		p := readIRQAffinity(irqFixture(), irqRequest())
		fact := p.Facts[tc.key]
		fact.Value = &tc.value
		p.Facts[tc.key] = fact
		if status, reason := CheckIRQAffinity(p); status != tc.status {
			t.Fatal(tc.key, status, reason)
		}
	}
}

func TestIRQAffinityBudgets(t *testing.T) {
	root := irqFixture()
	for i := 0; i < 4097; i++ {
		root[fmt.Sprintf("proc/irq/%d/smp_affinity_list", i)] = &fstest.MapFile{Data: []byte("1")}
	}
	p := readIRQAffinity(root, irqRequest())
	if p.Facts["before/inventory"].Status != "unavailable" {
		t.Fatal("inventory bound ignored")
	}
	root = irqFixture()
	for i := 0; i < 150; i++ {
		for _, kind := range []string{"smp_affinity_list", "effective_affinity_list"} {
			root[fmt.Sprintf("proc/irq/%d/%s", i, kind)] = &fstest.MapFile{Data: []byte(strings.Repeat(" ", 60000) + "1")}
		}
	}
	p = readIRQAffinity(root, irqRequest())
	if p.Facts["after/online"].Status != "too_large" {
		t.Fatal("aggregate read bound ignored")
	}
}

func TestIRQMask(t *testing.T) {
	for _, tc := range []struct {
		mask string
		cpus []int
	}{
		{"3", []int{0, 1}}, {"80000000", []int{31}}, {"1,00000000", []int{32}}, {"00000000,3", []int{0, 1}},
	} {
		got, err := parseIRQMask(tc.mask)
		if err != nil || !slices.Equal(got, tc.cpus) {
			t.Fatal(tc.mask, got, err)
		}
	}
	for _, mask := range []string{"", "0", "0x1", "100000000", "1,", "-1", strings.Repeat("1,", 128) + "1"} {
		if _, err := parseIRQMask(mask); err == nil {
			t.Fatal("accepted", mask)
		}
	}
}

func TestIRQAffinityRequests(t *testing.T) {
	for _, cpus := range []string{"", "3-2"} {
		p := ProbeIRQAffinity(cpus)
		if err := ValidateIRQAffinityProbe(p); err != nil {
			t.Fatal(err)
		}
	}
	p := irqRequest()
	p.Status = "unsupported"
	p.Reason = "device IRQ affinity probes require Linux procfs"
	if err := ValidateIRQAffinityProbe(p); err != nil {
		t.Fatal(err)
	}
	p.CPUs = ""
	if err := ValidateIRQAffinityProbe(p); err == nil {
		t.Fatal("accepted empty unsupported request")
	}
}
