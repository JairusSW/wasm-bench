package main

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/analysis"
	"github.com/wasmbench/wasmbench/experiment"
)

func TestPublicationCLITrustModesFailClosed(t *testing.T) {
	root := t.TempDir()
	key := filepath.Join(root, "public.json")
	if err := experiment.WriteJSON(key, strings.Repeat("00", 32)); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"verify-report", "--dir", root, "--qualification-key", key, "--recorded-builder"},
		{"verify-report", "--dir", root, "--qualification-key", key},
		{"report", "--run", root, "--qualification-key", key, "--out", filepath.Join(root, "report")},
		{"publish", "--run", root, "--qualification-key", key, "--out", filepath.Join(root, "publication")},
	} {
		if err := run(context.Background(), args); err == nil {
			t.Fatalf("accepted invalid publication trust mode: %v", args)
		}
	}
	for _, name := range []string{"report", "publication"} {
		if _, err := os.Lstat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Fatal("created output before invalid evidence rejected", name, err)
		}
	}
}

func TestQualificationCLIReviewedStatementAndExternalTrust(t *testing.T) {
	root := t.TempDir()
	bundle := filepath.Join(root, "synthetic-run")
	if err := os.MkdirAll(filepath.Join(bundle, "trials"), 0755); err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1700000000, 0).UTC()
	m := experiment.Manifest{ID: "synthetic-only", Kind: "measurement", Created: now, LockSHA256: strings.Repeat("a", 64), Host: agent.Host{OS: "linux", Arch: "arm64", Hostname: "synthetic-not-certified"}, Lock: experiment.Lock{Options: experiment.Options{Profile: "timing", Resources: agent.ResourcePolicy{CPUs: "2", CgroupParent: "/cg"}}}}
	if err := experiment.WriteJSON(filepath.Join(bundle, "manifest.json"), m); err != nil {
		t.Fatal(err)
	}
	if err := experiment.WriteJSON(filepath.Join(bundle, "trials", "trial.json"), experiment.Trial{ID: "trial", Started: now.Add(time.Second), DurationNS: int64(time.Second)}); err != nil {
		t.Fatal(err)
	}
	if err := experiment.Seal(bundle); err != nil {
		t.Fatal(err)
	}
	checksum, err := experiment.DigestFile(filepath.Join(bundle, "checksums.json"))
	if err != nil {
		t.Fatal(err)
	}
	draft, seed, pub, signed := filepath.Join(root, "draft.json"), filepath.Join(root, "operator.seed"), filepath.Join(root, "operator.pub.json"), filepath.Join(root, "signed.json")
	if err := os.WriteFile(seed, []byte(strings.Repeat("3a", 32)), 0600); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := run(ctx, []string{"qualification-draft", "--run", bundle, "--out", draft}); err != nil {
		t.Fatal(err)
	}
	if err := run(ctx, []string{"qualification-public-key", "--seed-file", seed, "--out", pub}); err != nil {
		t.Fatal(err)
	}
	if err := run(ctx, []string{"qualification-sign", "--run", bundle, "--statement", draft, "--seed-file", seed, "--out", signed}); err == nil {
		t.Fatal("blank draft signed")
	}
	if _, err := os.Stat(signed); !os.IsNotExist(err) {
		t.Fatal("failed signing created output")
	}
	var s analysis.DedicatedHostStatement
	if err := experiment.ReadJSON(draft, &s); err != nil {
		t.Fatal(err)
	}
	s.Operator = "synthetic operator for automated tests only"
	s.IssuedAt = now.Add(3 * time.Second)
	s.Assertions = map[string]string{"measurement_cpus": "exclusively_reserved_for_this_run", "controller_collectors": "outside_measurement_cpus", "unrelated_work": "excluded_from_measurement_cpus", "builds_and_uploads": "outside_measurement_cpus", "machine_policy": "maintained_throughout_validity_interval"}
	s.MachinePolicy = map[string]string{"frequency": "synthetic fixed frequency policy", "smt": "synthetic disabled SMT policy", "numa": "synthetic NUMA policy", "huge_pages": "synthetic disabled huge-page policy", "os_configuration": "synthetic OS configuration policy"}
	s.OperationalLog = "Synthetic fixtures exercise verification only. They do not certify this machine, any container, or any real measurement host."
	reviewed := filepath.Join(root, "reviewed.json")
	if err := experiment.WriteJSON(reviewed, s); err != nil {
		t.Fatal(err)
	}
	if err := run(ctx, []string{"qualification-sign", "--run", bundle, "--statement", reviewed, "--seed-file", seed, "--out", signed}); err != nil {
		t.Fatal(err)
	}
	evidence := loadQualificationEvidence(bundle, signed, pub)
	if evidence.Error != "" || evidence.Qualification == nil || evidence.ChecksumsSHA256 != checksum {
		t.Fatal(evidence)
	}
	b, err := experiment.Load(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if err := analysis.VerifyDedicatedQualification(*evidence.Qualification, b, checksum, evidence.TrustedPublicKey); err != nil {
		t.Fatal(err)
	}
	auditPath := filepath.Join(root, "audit.json")
	if err := publicationCheck([]string{"--run", bundle, "--qualification", signed, "--qualification-key", pub, "--out", auditPath}); err == nil {
		t.Fatal("signature waived missing kernel and measurement gates")
	}
	var audit analysis.PublicationAudit
	if err := experiment.ReadJSON(auditPath, &audit); err != nil {
		t.Fatal(err)
	}
	if audit.Status != "blocked" || audit.DedicatedQualification == nil {
		t.Fatal(audit)
	}
	for _, r := range audit.Requirements {
		if r.ID == "dedicated_machine_qualification" && r.Status != "passed" {
			t.Fatal(r)
		}
	}
	for _, args := range [][]string{
		{"qualification-sign", "--run", bundle, "--statement", reviewed, "--seed-file", seed, "--out", signed},
		{"qualification-draft", "--run", bundle, "--out", filepath.Join(bundle, "sidecar.json")},
		{"publication-check", "--run", bundle, "--out", filepath.Join(bundle, "audit.json")},
	} {
		if err := run(ctx, args); err == nil {
			t.Fatal("overwritten/nested output accepted", args)
		}
	}
	for _, pair := range [][2]string{{signed, ""}, {"", pub}, {signed, reviewed}, {reviewed, pub}} {
		if e := loadQualificationEvidence(bundle, pair[0], pair[1]); e.Error == "" {
			t.Fatal("incomplete/untyped evidence accepted", pair)
		}
	}
	if err := experiment.Verify(bundle); err != nil {
		t.Fatal("qualification modified sealed run", err)
	}
	key, _ := qualificationSeed(seed)
	defer clear(key)
	var publicHex string
	if err := experiment.ReadJSON(pub, &publicHex); err != nil {
		t.Fatal(err)
	}
	if publicHex != hex.EncodeToString(key.Public().(ed25519.PublicKey)) {
		t.Fatal("wrong public key export")
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(seed, 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := qualificationSeed(seed); err == nil {
			t.Fatal("world-readable seed accepted")
		}
		if err := os.Chmod(seed, 0600); err != nil {
			t.Fatal(err)
		}
		alias := filepath.Join(root, "alias.seed")
		if err := os.Symlink(seed, alias); err != nil {
			t.Fatal(err)
		}
		if _, err := qualificationSeed(alias); err == nil {
			t.Fatal("seed alias accepted")
		}
	}
}

func TestQualificationCLIRefusesInvalidSeedAndWire(t *testing.T) {
	for _, contents := range []string{"", strings.Repeat("xx", 32), strings.Repeat("ab", 31), strings.Repeat("ab", 33), strings.Repeat("ab", 1024)} {
		p := filepath.Join(t.TempDir(), "seed")
		if err := os.WriteFile(p, []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := qualificationSeed(p); err == nil {
			t.Fatal("invalid seed accepted")
		}
	}
	for _, cmd := range []string{"qualification-draft", "qualification-sign", "qualification-public-key"} {
		if err := run(context.Background(), []string{cmd}); err == nil {
			t.Fatal("missing arguments accepted")
		}
	}
}
