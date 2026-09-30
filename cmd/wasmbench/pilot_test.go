package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wasmbench/wasmbench/analysis"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

func TestPilotDecisionRecomputedAndImmutable(t *testing.T) {
	source := filepath.Join(t.TempDir(), "pilot")
	if err := os.MkdirAll(filepath.Join(source, "trials"), 0755); err != nil {
		t.Fatal(err)
	}
	m := experiment.Manifest{Kind: "measurement", LockSHA256: strings.Repeat("a", 64), Lock: experiment.Lock{Options: experiment.Options{Profile: "timing", Launches: 6, Scenarios: []string{"compile"}}, Runtimes: []experiment.Runtime{{ID: "r"}}, Workloads: []protocol.Workload{{ID: "w"}}}}
	if err := experiment.WriteJSON(filepath.Join(source, "manifest.json"), m); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		trial := experiment.Trial{ID: fmt.Sprint(i), Runtime: "r", Workload: "w", Scenario: "compile", Profile: "timing", Status: "ok", Block: i, Samples: []protocol.Sample{{Verified: true, Operations: 1, ElapsedNS: 100}}}
		if err := experiment.WriteJSON(filepath.Join(source, "trials", fmt.Sprint(i)+".json"), trial); err != nil {
			t.Fatal(err)
		}
	}
	if err := experiment.Seal(source); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "budget.json")
	if err := pilotPlan([]string{"--run", source, "--out", path}); err != nil {
		t.Fatal(err)
	}
	p, _, err := loadPilotDecision(path)
	if err != nil || p.Launches != 6 {
		t.Fatal(p, err)
	}
	if pilotPlan([]string{"--run", source, "--out", path}) == nil {
		t.Fatal("overwrote plan")
	}
	p.Launches++
	changed := filepath.Join(t.TempDir(), "changed.json")
	experiment.WriteJSON(changed, p)
	if _, _, err := loadPilotDecision(changed); err == nil {
		t.Fatal("changed budget accepted")
	}
	p.Launches--
	p.SourceChecksumsSHA256 = strings.Repeat("b", 64)
	changed = filepath.Join(t.TempDir(), "source.json")
	experiment.WriteJSON(changed, p)
	if _, _, err := loadPilotDecision(changed); err == nil {
		t.Fatal("changed evidence accepted")
	}
	var clean analysis.PilotPlan
	if err := experiment.ReadJSON(path, &clean); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(clean)
	raw = append(raw, []byte(" {}")...)
	changed = filepath.Join(t.TempDir(), "extra.json")
	if err := os.WriteFile(changed, raw, 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadPilotDecision(changed); err == nil {
		t.Fatal("trailing JSON accepted")
	}
	t.Run("publication audit and relocated pilot", func(t *testing.T) {
		pilot, err := experiment.Load(source)
		if err != nil {
			t.Fatal(err)
		}
		confirmation := filepath.Join(t.TempDir(), "confirmation")
		if err := os.MkdirAll(filepath.Join(confirmation, "trials"), 0755); err != nil {
			t.Fatal(err)
		}
		manifest := pilot.Manifest
		manifest.ID = "confirmation"
		manifest.Created = time.Now().UTC()
		manifest.Publication = "official" // This label must never grant qualification.
		manifest.Lock.Options.Launches = clean.Launches
		manifest.Lock.PilotPlan, _ = json.Marshal(clean)
		if err := experiment.WriteJSON(filepath.Join(confirmation, "manifest.json"), manifest); err != nil {
			t.Fatal(err)
		}
		for _, trial := range pilot.Trials {
			if err := experiment.WriteJSON(filepath.Join(confirmation, "trials", trial.ID+".json"), trial); err != nil {
				t.Fatal(err)
			}
		}
		if err := experiment.Seal(confirmation); err != nil {
			t.Fatal(err)
		}
		relocated := filepath.Join(filepath.Dir(source), "relocated")
		if err := os.Rename(source, relocated); err != nil {
			t.Fatal(err)
		}
		auditPath := filepath.Join(t.TempDir(), "audit.json")
		err = publicationCheck([]string{"--run", confirmation, "--pilot-run", relocated, "--out", auditPath})
		if err == nil || !strings.Contains(err.Error(), "dedicated_machine_qualification") {
			t.Fatal("publication did not fail closed", err)
		}
		var audit analysis.PublicationAudit
		if err := experiment.ReadJSON(auditPath, &audit); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, requirement := range audit.Requirements {
			if requirement.ID == "pilot_confirmation" {
				found = true
				if requirement.Status != "passed" {
					t.Fatal(requirement)
				}
			}
		}
		if !found || audit.Status != "blocked" {
			t.Fatal(audit)
		}
		if err := publicationCheck([]string{"--run", confirmation, "--pilot-run", relocated, "--out", auditPath}); err == nil || !strings.Contains(err.Error(), "exist") {
			t.Fatal("audit overwritten", err)
		}
		if err := experiment.Verify(confirmation); err != nil {
			t.Fatal("audit changed input evidence", err)
		}
	})
}
