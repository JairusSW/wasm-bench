package analysis

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/wasmbench/wasmbench/experiment"
)

const DedicatedQualificationVersion = "operator-dedicated-host-qualification-v1"
const qualificationDomain = "wasmbench/operator-dedicated-host-qualification-v1\x00"

// These are operator attestations, not facts inferred from cgroups or a signature.
// The publisher supplies the trusted public key separately from this envelope.
type DedicatedQualification struct {
	Statement DedicatedHostStatement `json:"statement"`
	PublicKey string                 `json:"public_key_hex"`
	Signature string                 `json:"signature_hex"`
}

type DedicatedHostStatement struct {
	Version         string            `json:"version"`
	Run             string            `json:"run"`
	ChecksumsSHA256 string            `json:"checksums_sha256"`
	LockSHA256      string            `json:"lock_sha256"`
	HostSHA256      string            `json:"host_sha256"`
	MeasurementCPUs string            `json:"measurement_cpus"`
	CgroupParent    string            `json:"cgroup_parent"`
	Operator        string            `json:"operator"`
	ValidFrom       time.Time         `json:"valid_from"`
	ValidUntil      time.Time         `json:"valid_until"`
	IssuedAt        time.Time         `json:"issued_at"`
	Assertions      map[string]string `json:"assertions"`
	MachinePolicy   map[string]string `json:"machine_policy"`
	OperationalLog  string            `json:"operational_log"`
}

var dedicatedAssertions = map[string]string{
	"measurement_cpus":      "exclusively_reserved_for_this_run",
	"controller_collectors": "outside_measurement_cpus",
	"unrelated_work":        "excluded_from_measurement_cpus",
	"builds_and_uploads":    "outside_measurement_cpus",
	"machine_policy":        "maintained_throughout_validity_interval",
}
var dedicatedPolicyFields = []string{"frequency", "smt", "numa", "huge_pages", "os_configuration"}

func qualificationMessage(s DedicatedHostStatement) ([]byte, error) {
	b, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	return append([]byte(qualificationDomain), b...), nil
}

// SignDedicatedQualification never asserts readiness or grants publisher trust.
// Operators must fill and review a draft; publication independently verifies it.
func SignDedicatedQualification(s DedicatedHostStatement, key ed25519.PrivateKey) (DedicatedQualification, error) {
	var q DedicatedQualification
	if len(key) != ed25519.PrivateKeySize {
		return q, fmt.Errorf("Ed25519 private key required")
	}
	b, err := qualificationMessage(s)
	if err != nil {
		return q, err
	}
	q.Statement = s
	q.PublicKey = hex.EncodeToString(key.Public().(ed25519.PublicKey))
	q.Signature = hex.EncodeToString(ed25519.Sign(key, b))
	return q, nil
}

func qualificationRunWindow(b experiment.Bundle) (time.Time, time.Time, error) {
	start, end := b.Manifest.Created, b.Manifest.Created
	if start.IsZero() || len(b.Trials) == 0 {
		return start, end, fmt.Errorf("recorded run/trial timestamps required")
	}
	for _, t := range b.Trials {
		if t.Started.IsZero() || t.Started.Before(start) || t.DurationNS <= 0 {
			return start, end, fmt.Errorf("invalid trial window: %s", t.ID)
		}
		finished := t.Started.Add(time.Duration(t.DurationNS))
		if !finished.After(t.Started) {
			return start, end, fmt.Errorf("invalid trial duration")
		}
		if finished.After(end) {
			end = finished
		}
	}
	return start, end, nil
}

func hostDigest(b experiment.Bundle) string {
	encoded, _ := json.Marshal(b.Manifest.Host)
	h := sha256.Sum256(encoded)
	return hex.EncodeToString(h[:])
}

// DedicatedQualificationDraft binds the sealed run, but deliberately leaves all
// operator assertions, policy, identity and log empty. It cannot qualify a run.
func DedicatedQualificationDraft(b experiment.Bundle, checksum string) (DedicatedHostStatement, error) {
	start, end, err := qualificationRunWindow(b)
	if err != nil {
		return DedicatedHostStatement{}, err
	}
	s := DedicatedHostStatement{Version: DedicatedQualificationVersion, Run: b.Manifest.ID, ChecksumsSHA256: checksum, LockSHA256: b.Manifest.LockSHA256, HostSHA256: hostDigest(b), MeasurementCPUs: b.Manifest.Lock.Options.Resources.CPUs, CgroupParent: b.Manifest.Lock.Options.Resources.CgroupParent, ValidFrom: start, ValidUntil: end, Assertions: map[string]string{}, MachinePolicy: map[string]string{}}
	for k := range dedicatedAssertions {
		s.Assertions[k] = ""
	}
	for _, k := range dedicatedPolicyFields {
		s.MachinePolicy[k] = ""
	}
	return s, nil
}

// VerifyDedicatedQualification authenticates an external operator's assertions
// for one exact sealed run. It does not verify their physical truth. All kernel,
// resource, host-baseline, pilot and correctness gates remain independently required.
func VerifyDedicatedQualification(q DedicatedQualification, b experiment.Bundle, checksum string, trusted ed25519.PublicKey) error {
	if len(trusted) != ed25519.PublicKeySize {
		return fmt.Errorf("separately trusted Ed25519 public key required")
	}
	pub, err := hex.DecodeString(q.PublicKey)
	if err != nil || !bytes.Equal(pub, trusted) {
		return fmt.Errorf("operator key is not trusted")
	}
	sig, err := hex.DecodeString(q.Signature)
	if err != nil || len(sig) != ed25519.SignatureSize {
		return fmt.Errorf("invalid qualification signature encoding")
	}
	message, err := qualificationMessage(q.Statement)
	if err != nil || len(message) > 64<<10 || !ed25519.Verify(trusted, message, sig) {
		return fmt.Errorf("invalid qualification signature")
	}
	s := q.Statement
	for _, digest := range []string{checksum, s.ChecksumsSHA256, s.LockSHA256, s.HostSHA256} {
		decoded, e := hex.DecodeString(digest)
		if e != nil || len(decoded) != sha256.Size {
			return fmt.Errorf("qualification requires exact SHA256 digests")
		}
	}
	if s.Version != DedicatedQualificationVersion || s.Run != b.Manifest.ID || s.ChecksumsSHA256 != checksum || s.LockSHA256 != b.Manifest.LockSHA256 || s.HostSHA256 != hostDigest(b) {
		return fmt.Errorf("qualification does not bind this sealed run")
	}
	l := b.Manifest.Lock
	if b.Manifest.Host.OS != "linux" || b.Manifest.Kind != "measurement" || l.Options.Check || l.Options.Profile != "timing" || l.Options.PhaseBarriers {
		return fmt.Errorf("qualification requires a Linux timing measurement")
	}
	if s.MeasurementCPUs == "" || s.CgroupParent == "" || s.MeasurementCPUs != l.Options.Resources.CPUs || s.CgroupParent != l.Options.Resources.CgroupParent {
		return fmt.Errorf("qualification resource allocation differs from run")
	}
	start, end, err := qualificationRunWindow(b)
	if err != nil {
		return err
	}
	if s.ValidFrom.IsZero() || s.ValidUntil.IsZero() || s.IssuedAt.IsZero() || s.ValidFrom.After(start) || s.ValidUntil.Before(end) || !s.ValidUntil.After(s.ValidFrom) || s.IssuedAt.Before(end) {
		return fmt.Errorf("qualification must cover all recorded trial windows and be issued after collection")
	}
	if s.Operator != strings.TrimSpace(s.Operator) || len(s.Operator) < 3 || len(s.Operator) > 256 || len(strings.TrimSpace(s.OperationalLog)) < 64 || len(s.OperationalLog) > 8192 {
		return fmt.Errorf("operator identity and reviewed operational log required")
	}
	if len(s.Assertions) != len(dedicatedAssertions) {
		return fmt.Errorf("complete dedicated-host assertions required")
	}
	for k, want := range dedicatedAssertions {
		if s.Assertions[k] != want {
			return fmt.Errorf("missing operator assertion: %s", k)
		}
	}
	if len(s.MachinePolicy) != len(dedicatedPolicyFields) {
		return fmt.Errorf("complete controlled machine policy required")
	}
	for _, k := range dedicatedPolicyFields {
		v := strings.TrimSpace(s.MachinePolicy[k])
		if len(v) < 8 || len(v) > 2048 || strings.Contains(strings.ToLower(v), "uncontrolled") || strings.Contains(strings.ToLower(v), "unknown") {
			return fmt.Errorf("explicit maintained machine policy required: %s", k)
		}
	}
	return nil
}

// Strict decoding rejects unknown fields, duplicate keys, trailing documents,
// oversized envelopes and excessive nesting before signature interpretation.
func decodeQualificationJSON(data []byte, dst any) error {
	if len(data) == 0 || len(data) > 64<<10 {
		return fmt.Errorf("qualification JSON required within 64 KiB")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	var walk func(int) error
	walk = func(depth int) error {
		if depth > 16 {
			return fmt.Errorf("qualification JSON nesting exceeds bound")
		}
		token, err := d.Token()
		if err != nil {
			return err
		}
		if delimiter, ok := token.(json.Delim); ok {
			switch delimiter {
			case '{':
				seen := map[string]bool{}
				for d.More() {
					key, e := d.Token()
					if e != nil {
						return e
					}
					k, ok := key.(string)
					if !ok || seen[k] {
						return fmt.Errorf("duplicate/invalid qualification field")
					}
					seen[k] = true
					if e = walk(depth + 1); e != nil {
						return e
					}
				}
			case '[':
				for d.More() {
					if e := walk(depth + 1); e != nil {
						return e
					}
				}
			default:
				return fmt.Errorf("unexpected JSON delimiter")
			}
			_, err = d.Token()
			return err
		}
		return nil
	}
	if err := walk(0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return fmt.Errorf("one qualification JSON document required")
	}
	d = json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	return d.Decode(dst)
}

func DecodeDedicatedQualification(data []byte) (DedicatedQualification, error) {
	var q DedicatedQualification
	err := decodeQualificationJSON(data, &q)
	return q, err
}

func DecodeDedicatedHostStatement(data []byte) (DedicatedHostStatement, error) {
	var s DedicatedHostStatement
	err := decodeQualificationJSON(data, &s)
	return s, err
}
