package publish

import (
	"math"
	"testing"

	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

func TestCodeSizeSurvivesTransportLimitWithoutInventingImage(t *testing.T) {
	observation := protocol.Observation{Metric: "native.code_size", DefinitionVersion: 1, Value: protocol.Value(114175248), Unit: "bytes", Scope: "compiled_module", Phase: "compile", Profile: "code", Quality: "engine_reported", Status: "available", Denominator: "module", Collector: "Wazevo", CollectorVersion: "1.12.0"}
	trial := experiment.Trial{ID: "large", Runtime: "r", Workload: "w", Scenario: "compile", Profile: "code", Status: "ok", Observations: []protocol.Observation{observation, {Metric: "native.code_export", Status: "unavailable", Reason: "transport limit"}}}
	b := experiment.Bundle{Manifest: experiment.Manifest{Kind: "measurement"}, Trials: []experiment.Trial{trial}}
	matched := map[string]bool{"r\x00w": true}
	got := pairedCodeRecords(b, matched)[0]
	if got.SizeBytes == nil || *got.SizeBytes != 114175248 || got.ImageBytes != nil || got.Status != "unavailable" {
		t.Fatal(got)
	}
	for name, mutate := range map[string]func(*experiment.Trial){
		"duplicate":         func(t *experiment.Trial) { t.Observations = append(t.Observations, observation) },
		"failed":            func(t *experiment.Trial) { t.Status = "error" },
		"nan":               func(t *experiment.Trial) { t.Observations[0].Value = protocol.Value(math.NaN()) },
		"scope":             func(t *experiment.Trial) { t.Observations[0].Scope = "unknown" },
		"profile":           func(t *experiment.Trial) { t.Observations[0].Profile = "timing" },
		"conflicting image": func(t *experiment.Trial) { t.CodeImage = &protocol.CodeImage{Data: []byte{1, 2, 3}} },
	} {
		t.Run(name, func(t *testing.T) {
			copy := trial
			copy.Observations = append([]protocol.Observation(nil), trial.Observations...)
			mutate(&copy)
			b.Trials = []experiment.Trial{copy}
			if pairedCodeRecords(b, matched)[0].SizeBytes != nil {
				t.Fatal("invalid size evidence accepted")
			}
		})
	}
}

func TestCodePairRequiresExactRuntimeAndWorkloadIdentity(t *testing.T) {
	base := experiment.Bundle{Manifest: experiment.Manifest{Kind: "measurement", Host: agent.Host{OS: "darwin", Arch: "arm64"}, Lock: experiment.Lock{Protocol: 1, Options: experiment.Options{Profile: "timing"}, Runtimes: []experiment.Runtime{{ID: "r", Files: map[string]string{"adapter": "one"}}}, Workloads: []protocol.Workload{{ID: "w", SHA256: "digest", Artifact: "/original.wasm", Oracle: protocol.Oracle{Kind: "exact_u64", Expected: []uint64{7}}}}}}}
	code := base
	code.Manifest.Lock.Options.Profile = "code"
	code.Manifest.Lock.Workloads = []protocol.Workload{{ID: "w", SHA256: "digest", Artifact: "/restored.wasm", Oracle: protocol.Oracle{Kind: "exact_u64", Expected: []uint64{7}}}}
	matched, err := matchingPassCells(base, code, "code")
	if err != nil || !matched["r\x00w"] {
		t.Fatalf("identical content and contracts should pair across restored paths: %v %v", matched, err)
	}
	for name, mutate := range map[string]func(*experiment.Bundle){
		"IRQ requirement":       func(b *experiment.Bundle) { b.Manifest.Lock.RequireIRQAffinity = true },
		"partition requirement": func(b *experiment.Bundle) { b.Manifest.Lock.RequireIsolatedCPUPartition = true },
		"host baseline": func(b *experiment.Bundle) {
			b.Manifest.Lock.HostPolicy = &agent.HostPolicy{Version: agent.HostPolicyVersion}
		},
		"wrong profile": func(b *experiment.Bundle) { b.Manifest.Lock.Options.Profile = "memory" },
		"wrong host":    func(b *experiment.Bundle) { b.Manifest.Host.Arch = "amd64" },
		"wrong binary":  func(b *experiment.Bundle) { b.Manifest.Lock.Runtimes[0].Files["adapter"] = "other" },
		"wrong oracle":  func(b *experiment.Bundle) { b.Manifest.Lock.Workloads[0].Oracle.Expected = []uint64{8} },
	} {
		t.Run(name, func(t *testing.T) {
			changed := code
			changed.Manifest.Lock.Runtimes = append([]experiment.Runtime(nil), code.Manifest.Lock.Runtimes...)
			changed.Manifest.Lock.Runtimes[0].Files = map[string]string{"adapter": "one"}
			changed.Manifest.Lock.Workloads = append([]protocol.Workload(nil), code.Manifest.Lock.Workloads...)
			mutate(&changed)
			if _, err := matchingPassCells(base, changed, "code"); err == nil {
				t.Fatal("paired incompatible code evidence")
			}
		})
	}
}

func TestPairedCodeRecordsRetainMissingAndWithholdHostMismatch(t *testing.T) {
	b := experiment.Bundle{Manifest: experiment.Manifest{Kind: "measurement", Host: agent.Host{OS: "darwin"}}, Trials: []experiment.Trial{
		{ID: "check", Runtime: "r", Workload: "w", Scenario: "compile", Profile: "code", Block: -1, Status: "ok"},
		{ID: "image", Runtime: "r", Workload: "w", Scenario: "compile", Profile: "code", Block: 0, Status: "ok", CodeImage: &protocol.CodeImage{Data: []byte{1, 2, 3}}},
		{ID: "missing", Runtime: "r", Workload: "w", Scenario: "compile", Profile: "code", Block: 1, Status: "ok"},
		{ID: "other", Runtime: "other", Workload: "w", Scenario: "compile", Profile: "code", Block: 0, Status: "ok"},
	}}
	got := pairedCodeRecords(b, map[string]bool{"r\x00w": true})
	if len(got) != 2 || got[0].Index != 1 || got[0].Status != "available" || got[0].ImageBytes == nil || *got[0].ImageBytes != 3 || got[1].Status != "not_recorded" || got[1].ImageBytes != nil {
		t.Fatal(got)
	}
	b.Manifest.Lock.HostPolicy = &agent.HostPolicy{Version: agent.HostPolicyVersion}
	got = pairedCodeRecords(b, map[string]bool{"r\x00w": true})
	if got[0].Status != "withheld_host_mismatch" || got[0].ImageBytes != nil {
		t.Fatal("invalid host evidence yielded code size", got)
	}
}
