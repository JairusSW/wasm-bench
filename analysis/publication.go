package analysis

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

const PublicationAuditVersion = "publication-evidence-audit-v7"

type PublicationRequirement struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Reason string `json:"reason"`
}
type PublicationAudit struct {
	DedicatedQualification *DedicatedQualification  `json:"dedicated_qualification,omitempty"`
	Version                string                   `json:"version"`
	Run                    string                   `json:"run"`
	Status                 string                   `json:"status"`
	Requirements           []PublicationRequirement `json:"requirements"`
	Interpretation         string                   `json:"interpretation"`
}
type PilotEvidence struct {
	Bundle          *experiment.Bundle
	ChecksumsSHA256 string
	Error           string
}

type QualificationEvidence struct {
	Qualification    *DedicatedQualification
	TrustedPublicKey ed25519.PublicKey
	ChecksumsSHA256  string
	Error            string
}

func (a PublicationAudit) Err() error {
	var unmet []string
	for _, r := range a.Requirements {
		if r.Status != "passed" {
			unmet = append(unmet, r.ID+": "+r.Reason)
		}
	}
	if len(unmet) > 0 {
		return fmt.Errorf("publication blocked: %s", strings.Join(unmet, "; "))
	}
	return nil
}

// DecodePilotPlan rejects partial/forward-shaped decisions instead of silently
// interpreting unknown fields under today's publication policy.
func DecodePilotPlan(data []byte) (PilotPlan, error) {
	var p PilotPlan
	if len(data) == 0 || len(data) > 16<<20 {
		return p, fmt.Errorf("embedded pilot plan required within 16 MiB")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&p); err != nil {
		return p, err
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return p, fmt.Errorf("pilot plan must contain one JSON value")
	}
	return p, nil
}

// VerifyPilotDecision re-derives the full decision, including every cell and
// interval. Callers must load source with experiment.Load and hash checksums.json.
func VerifyPilotDecision(p PilotPlan, source experiment.Bundle, checksum string) error {
	expected, err := PlanPilot(source, p.TargetRelativeHalfWidth, p.MinLaunches, p.MaxLaunches)
	if err != nil {
		return err
	}
	expected.SourceBundle = p.SourceBundle
	expected.SourceChecksumsSHA256 = checksum
	if checksum == "" || !reflect.DeepEqual(p, expected) {
		return fmt.Errorf("pilot decision differs from independently recomputed sealed evidence")
	}
	if p.Status != "ready" {
		return fmt.Errorf("pilot has unresolved cells; no confirmation budget selected")
	}
	return nil
}

func VerifyPilotConfirmation(b, source experiment.Bundle, checksum string) error {
	p, err := DecodePilotPlan(b.Manifest.Lock.PilotPlan)
	if err != nil {
		return err
	}
	if err := VerifyPilotDecision(p, source, checksum); err != nil {
		return err
	}
	if b.Manifest.Kind != "measurement" || b.Manifest.Lock.Options.Check || b.Manifest.Lock.Options.Launches != p.Launches {
		return fmt.Errorf("confirmation must use the fixed pilot timing budget")
	}
	if !reflect.DeepEqual(b.Manifest.Host, source.Manifest.Host) {
		return fmt.Errorf("confirmation host differs from pilot")
	}
	// Both inputs are sealed-bundle locks: artifact/fixture paths are already
	// content-addressed. Only the new launch count and embedded decision may differ.
	got := b.Manifest.Lock
	got.PilotPlan = nil
	got.Options.Launches = source.Manifest.Lock.Options.Launches
	a, err := json.Marshal(got)
	if err != nil {
		return err
	}
	want, err := json.Marshal(source.Manifest.Lock)
	if err != nil {
		return err
	}
	if !bytes.Equal(a, want) {
		return fmt.Errorf("confirmation changed the pilot experiment contract")
	}
	if b.Manifest.ID == source.Manifest.ID || !b.Manifest.Created.After(source.Manifest.Created) {
		return fmt.Errorf("confirmation must identify a later separate run; timestamps alone do not prove independence")
	}
	return nil
}

// AuditPublication is a read-only policy assessment of already verified bundles.
// It never upgrades labels or treats boundary matching as dedicated-host proof.
func AuditPublication(b experiment.Bundle, pilot PilotEvidence) PublicationAudit {
	return AuditPublicationWithQualification(b, pilot, QualificationEvidence{})
}

func AuditPublicationWithQualification(b experiment.Bundle, pilot PilotEvidence, qualification QualificationEvidence) PublicationAudit {
	a := PublicationAudit{Version: PublicationAuditVersion, Run: b.Manifest.ID, Status: "blocked", Interpretation: "Offline evidence audit. Failed/unsupported workloads remain in raw evidence. No manifest label grants qualification. Dedicated-host assertions require a separately trusted operator key; signature verification authenticates assertions, not their physical truth. Kernel and measurement evidence remain independently required."}
	add := func(id string, ok bool, reason string) {
		status := "unmet"
		if ok {
			status = "passed"
		}
		a.Requirements = append(a.Requirements, PublicationRequirement{id, status, reason})
	}
	l := b.Manifest.Lock
	calibrationOnly := false
	for _, scenario := range l.Options.Scenarios {
		calibrationOnly = calibrationOnly || scenario == protocol.HarnessCalibrationScenario
	}
	add("workload_performance_scope", !calibrationOnly, "empty local harness calibration cannot be published as workload performance")
	add("timing_measurement", b.Manifest.Kind == "measurement" && !l.Options.Check && l.Options.Profile == "timing" && !l.Options.PhaseBarriers, "requires a timing measurement, not a correctness or instrumented pass")
	add("matched_host_baseline", l.HostPolicy != nil && experiment.ValidateHostEvidence(b.Manifest) == nil && b.Manifest.HostEndCheck.Status == "matched_observed_baseline", "requires a pinned baseline and independently consistent matched start/end observations; not continuous control")
	partitionOK := l.RequireIsolatedCPUPartition && experiment.ValidateCPUPartitionEvidence(b.Manifest) == nil && b.Manifest.CPUPartitionEnd.Err() == nil
	irqOK := l.RequireIRQAffinity && experiment.ValidateIRQAffinityEvidence(b.Manifest) == nil && b.Manifest.IRQAffinityEnd.Err() == nil
	add("device_irq_affinity_boundaries", irqOK, "requires locked, independently consistent default/requested/effective device IRQ masks disjoint from measurement CPUs at both run boundaries; not interrupt delivery or continuous host control")
	add("isolated_cpu_partition_boundaries", partitionOK, "requires locked, independently consistent empty isolated-partition observations before preparation and after cleanup; not continuous isolation")
	sampledOK := partitionOK && experiment.ValidatePartitionTrialEvidence(b) == nil
	controllerSamplesOK := sampledOK
	monitoredTrials := 0
	for _, t := range b.Trials {
		if t.PartitionMonitor != nil {
			monitoredTrials++
			if t.PartitionMonitor.Status != "ready_at_samples" {
				sampledOK = false
			}
			if t.PartitionMonitor.Version != agent.ActivePartitionVersion {
				controllerSamplesOK = false
			}
		} else if t.Status == "ok" && t.Block >= 0 {
			sampledOK = false
		}
	}
	add("sampled_partition_workers", sampledOK && monitoredTrials > 0, "requires every successful measured trial to retain valid sampled isolated CPU masks and single-worker cgroup membership; gaps between samples and other host workloads remain unqualified")
	add("sampled_controller_separation", sampledOK && controllerSamplesOK && monitoredTrials > 0, "requires v2 occupied-partition samples with bounded controller/collector thread inventories and CPU masks disjoint from measurement CPUs at both sample boundaries; v1 evidence and unsampled intervals do not establish separation")
	pilotErr := fmt.Errorf("sealed pilot evidence and embedded decision required")
	if pilot.Error != "" {
		pilotErr = fmt.Errorf("pilot evidence unavailable: %s", pilot.Error)
	} else if pilot.Bundle != nil {
		pilotErr = VerifyPilotConfirmation(b, *pilot.Bundle, pilot.ChecksumsSHA256)
	}
	reason := "full pilot decision recomputed; confirmation contract and fixed launch budget match"
	if pilotErr != nil {
		reason = pilotErr.Error()
	}
	add("pilot_confirmation", pilotErr == nil, reason)
	admitted := l.Analyzer != nil && len(l.Workloads) > 0
	for _, w := range l.Workloads {
		found := false
		for _, v := range b.Admission {
			if v.SHA256 == w.SHA256 && v.Status == "validated" {
				found = true
			}
		}
		admitted = admitted && found
	}
	add("artifact_admission", admitted, "requires pinned independent analyzer admission for every declared artifact")
	checks := map[string]int{}
	correctnessOK := len(l.Runtimes) > 0 && len(l.Workloads) > 0
	for _, t := range b.Trials {
		if t.Block >= 0 {
			continue
		}
		key := t.Runtime + "\x00" + t.Workload
		checks[key]++
		if t.Block != -1 || t.Status != "ok" || t.Profile != "timing" || len(t.Samples) == 0 {
			correctnessOK = false
		}
		for _, sample := range t.Samples {
			if !sample.Verified || sample.ElapsedNS < 0 || sample.Operations <= 0 {
				correctnessOK = false
			}
		}
	}
	for _, runtime := range l.Runtimes {
		for _, workload := range l.Workloads {
			key := runtime.ID + "\x00" + workload.ID
			if checks[key] != 1 {
				correctnessOK = false
			}
			delete(checks, key)
		}
	}
	add("sacrificial_correctness", correctnessOK && len(checks) == 0, "requires one successful separate sacrificial correctness trial per runtime/workload, with verified outputs; no missing or duplicate admissions")
	// PlanPilot already enforces complete cells, verified samples, replication,
	// and stability. Reuse those checks without selecting a new stopping budget.
	diagnostic := b
	diagnostic.Manifest.Lock.PilotPlan = nil
	coverage, err := PlanPilot(diagnostic, .99, 6, 10000)
	ok := err == nil && len(coverage.Cells) > 0
	var issues []string
	if err != nil {
		issues = append(issues, err.Error())
	} else {
		for _, c := range coverage.Cells {
			if c.Status != "ready" {
				ok = false
				issues = append(issues, c.Runtime+"/"+c.Workload+"/"+c.Scenario+": "+c.Reason)
			}
		}
	}
	reason = "all declared cells retain successful verified launches and estimable intervals; no within-launch instability detected"
	if len(issues) > 0 {
		reason = strings.Join(issues, "; ")
	}
	add("complete_stable_measurements", ok, reason)
	resourceOK := l.Options.Resources.CPUs != "" && l.Options.Resources.MemoryMaxBytes > 0 && l.Options.Resources.Mems != "" && len(b.Trials) > 0
	for _, t := range b.Trials {
		if t.Status == "ok" && agent.ValidateNUMAIsolation(l.Options.Resources, t.Isolation, "response_end_before_cleanup") != nil {
			resourceOK = false
		}
	}
	add("resource_boundary_evidence", resourceOK, "requires explicit CPU, NUMA-node and memory budgets with consistent spawn/end readbacks for successful trials; no exclusivity or ancestor guarantee")
	qualificationErr := fmt.Errorf("operator-signed dedicated-host qualification and separately trusted public key required")
	if qualification.Error != "" {
		qualificationErr = fmt.Errorf("qualification evidence unavailable: %s", qualification.Error)
	} else if qualification.Qualification != nil {
		qualificationErr = VerifyDedicatedQualification(*qualification.Qualification, b, qualification.ChecksumsSHA256, qualification.TrustedPublicKey)
	}
	reason = "separately trusted operator authenticated exact sealed-run dedication and maintained-policy assertions; not independent proof of physical truth"
	if qualificationErr != nil {
		reason = qualificationErr.Error()
	} else {
		a.DedicatedQualification = qualification.Qualification
	}
	add("dedicated_machine_qualification", qualificationErr == nil, reason)
	if a.Err() == nil {
		a.Status = "eligible"
	}
	return a
}
