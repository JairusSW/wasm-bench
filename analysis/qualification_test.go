package analysis

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/experiment"
)

func qualificationFixture(t *testing.T) (experiment.Bundle, string, DedicatedQualification, ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	now := time.Unix(1700000000, 0).UTC()
	b := experiment.Bundle{Manifest: experiment.Manifest{ID: "qualification-fixture", Created: now, Kind: "measurement", LockSHA256: strings.Repeat("a", 64), Host: agent.Host{OS: "linux", Arch: "arm64", Hostname: "fixture"}, Lock: experiment.Lock{Options: experiment.Options{Profile: "timing", Resources: agent.ResourcePolicy{CPUs: "2-3", CgroupParent: "/cg"}}}}, Trials: []experiment.Trial{{ID: "trial", Started: now.Add(time.Second), DurationNS: int64(time.Second)}}}
	checksum := strings.Repeat("b", 64)
	s, err := DedicatedQualificationDraft(b, checksum)
	if err != nil {
		t.Fatal(err)
	}
	s.Operator = "test operator; not real host certification"
	s.IssuedAt = now.Add(3 * time.Second)
	for k, v := range dedicatedAssertions {
		s.Assertions[k] = v
	}
	for _, k := range dedicatedPolicyFields {
		s.MachinePolicy[k] = "fixture maintained policy: " + k
	}
	s.OperationalLog = "Synthetic qualification fixture only. No development machine or real measurement host is certified by this test."
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	q, err := SignDedicatedQualification(s, key)
	if err != nil {
		t.Fatal(err)
	}
	return b, checksum, q, pub, key
}

func TestDedicatedQualificationTrustAndBindings(t *testing.T) {
	b, checksum, q, pub, _ := qualificationFixture(t)
	before, _ := json.Marshal(b)
	if err := VerifyDedicatedQualification(q, b, checksum, pub); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(b)
	if string(before) != string(after) {
		t.Fatal("verification mutated run")
	}
	for _, mode := range []string{"no-trust", "wrong-key", "self-declared-key", "signature", "run", "seal", "lock", "host", "cpus", "parent", "non-linux", "correctness", "memory", "barriers", "zero-duration", "before-created", "late-trial"} {
		t.Run(mode, func(t *testing.T) {
			b, checksum, q, pub, _ := qualificationFixture(t)
			switch mode {
			case "no-trust":
				pub = nil
			case "wrong-key":
				pub = make([]byte, ed25519.PublicKeySize)
			case "self-declared-key":
				q.PublicKey = strings.Repeat("0", 64)
			case "signature":
				q.Signature = strings.Repeat("0", 128)
			case "run":
				b.Manifest.ID = "other"
			case "seal":
				checksum = strings.Repeat("c", 64)
			case "lock":
				b.Manifest.LockSHA256 = strings.Repeat("c", 64)
			case "host":
				b.Manifest.Host.Hostname = "other"
			case "cpus":
				b.Manifest.Lock.Options.Resources.CPUs = "4"
			case "parent":
				b.Manifest.Lock.Options.Resources.CgroupParent = "/other"
			case "non-linux":
				b.Manifest.Host.OS = "darwin"
			case "correctness":
				b.Manifest.Lock.Options.Check = true
			case "memory":
				b.Manifest.Lock.Options.Profile = "memory"
			case "barriers":
				b.Manifest.Lock.Options.PhaseBarriers = true
			case "zero-duration":
				b.Trials[0].DurationNS = 0
			case "before-created":
				b.Trials[0].Started = b.Manifest.Created.Add(-time.Second)
			case "late-trial":
				b.Trials[0].DurationNS = int64(10 * time.Second)
			}
			if VerifyDedicatedQualification(q, b, checksum, pub) == nil {
				t.Fatal("changed trust/identity accepted")
			}
		})
	}
}

func TestDedicatedQualificationSignedIncompleteStatements(t *testing.T) {
	for _, mode := range []string{"draft", "empty-cpus", "operator", "log", "from", "until", "issued", "version", "assertion", "extra-assertion", "policy", "extra-policy", "uncontrolled", "unknown", "huge-policy", "invalid-digest"} {
		t.Run(mode, func(t *testing.T) {
			b, checksum, q, pub, key := qualificationFixture(t)
			s := q.Statement
			switch mode {
			case "draft":
				s, _ = DedicatedQualificationDraft(b, checksum)
			case "empty-cpus":
				s.MeasurementCPUs = ""
			case "operator":
				s.Operator = ""
			case "log":
				s.OperationalLog = ""
			case "from":
				s.ValidFrom = s.ValidFrom.Add(time.Second)
			case "until":
				s.ValidUntil = s.ValidUntil.Add(-time.Second)
			case "issued":
				s.IssuedAt = s.ValidFrom
			case "version":
				s.Version = "future"
			case "assertion":
				s.Assertions["unrelated_work"] = "unknown"
			case "extra-assertion":
				s.Assertions["future"] = "yes"
			case "policy":
				delete(s.MachinePolicy, "frequency")
			case "extra-policy":
				s.MachinePolicy["future"] = "maintained"
			case "uncontrolled":
				s.MachinePolicy["frequency"] = "uncontrolled frequency policy"
			case "unknown":
				s.MachinePolicy["smt"] = "unknown SMT configuration"
			case "huge-policy":
				s.MachinePolicy["frequency"] = strings.Repeat("x", 65<<10)
			case "invalid-digest":
				s.LockSHA256 = "not a digest"
			}
			q, err := SignDedicatedQualification(s, key)
			if err != nil {
				t.Fatal(err)
			}
			if VerifyDedicatedQualification(q, b, checksum, pub) == nil {
				t.Fatal("signed incomplete assertion accepted")
			}
		})
	}
}

func TestDedicatedQualificationStrictWireAndDomain(t *testing.T) {
	b, checksum, q, pub, key := qualificationFixture(t)
	raw, _ := json.Marshal(q)
	decoded, err := DecodeDedicatedQualification(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyDedicatedQualification(decoded, b, checksum, pub); err != nil {
		t.Fatal(err)
	}
	for _, raw := range [][]byte{nil, []byte("null"), []byte("{} {}"), []byte(`{"future":true}`), []byte(`{"signature_hex":"one","signature_hex":"two"}`), []byte(`{"statement":{"run":"one","run":"two"}}`), []byte(strings.Repeat("[", 20) + "0" + strings.Repeat("]", 20)), make([]byte, 65<<10)} {
		decoded, err := DecodeDedicatedQualification(raw)
		if err == nil && VerifyDedicatedQualification(decoded, b, checksum, pub) == nil {
			t.Fatal("invalid wire accepted")
		}
	}
	statement, _ := json.Marshal(q.Statement)
	q.Signature = hex.EncodeToString(ed25519.Sign(key, statement)) // No domain separation: must fail.
	if VerifyDedicatedQualification(q, b, checksum, pub) == nil {
		t.Fatal("undomained signature accepted")
	}
	if _, err := SignDedicatedQualification(q.Statement, []byte("wrong")); err == nil {
		t.Fatal("invalid private key accepted")
	}
}

func TestDedicatedQualificationDoesNotWaiveMeasurementEvidence(t *testing.T) {
	b, checksum, q, pub, _ := qualificationFixture(t)
	a := AuditPublicationWithQualification(b, PilotEvidence{}, QualificationEvidence{Qualification: &q, TrustedPublicKey: pub, ChecksumsSHA256: checksum})
	if a.Status != "blocked" || a.Err() == nil || a.DedicatedQualification == nil {
		t.Fatal("qualification waived missing machine/measurement evidence", a)
	}
	for _, r := range a.Requirements {
		if r.ID == "dedicated_machine_qualification" && r.Status != "passed" {
			t.Fatal(r)
		}
		if r.ID == "isolated_cpu_partition_boundaries" && r.Status == "passed" {
			t.Fatal("operator waived kernel evidence")
		}
	}
	a = AuditPublicationWithQualification(b, PilotEvidence{}, QualificationEvidence{Qualification: &q, ChecksumsSHA256: checksum})
	if a.DedicatedQualification != nil || a.Status != "blocked" {
		t.Fatal("untrusted envelope accepted", a)
	}
}
