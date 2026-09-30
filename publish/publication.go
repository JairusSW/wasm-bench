package publish

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"

	"github.com/wasmbench/wasmbench/analysis"
	"github.com/wasmbench/wasmbench/experiment"
)

const PublicationArchiveVersion = "qualified-publication-archive-v1"

// The recorded key describes the publisher's trust decision, not a reader's.
// VerifyPublicationReport requires the reader's separately trusted key.
type PublicationReceipt struct {
	Version              string                    `json:"version"`
	OperatorPublicKey    string                    `json:"operator_public_key_hex"`
	PilotChecksumsSHA256 string                    `json:"pilot_checksums_sha256"`
	Audit                analysis.PublicationAudit `json:"audit"`
}

type publicationInput struct {
	pilot         string
	qualification analysis.DedicatedQualification
	key           ed25519.PublicKey
	Receipt       *PublicationReceipt
}

func publicationReceipt(run, pilot string, q analysis.DedicatedQualification, key ed25519.PublicKey) (*PublicationReceipt, error) {
	b, err := experiment.Load(run)
	if err != nil {
		return nil, err
	}
	checksum, err := experiment.DigestFile(filepath.Join(run, "checksums.json"))
	if err != nil {
		return nil, err
	}
	p, err := experiment.Load(pilot)
	if err != nil {
		return nil, err
	}
	pilotHash, err := experiment.DigestFile(filepath.Join(pilot, "checksums.json"))
	if err != nil {
		return nil, err
	}
	audit := analysis.AuditPublicationWithQualification(b, analysis.PilotEvidence{Bundle: &p, ChecksumsSHA256: pilotHash}, analysis.QualificationEvidence{Qualification: &q, TrustedPublicKey: key, ChecksumsSHA256: checksum})
	if err := audit.Err(); err != nil {
		return nil, err
	}
	return &PublicationReceipt{Version: PublicationArchiveVersion, OperatorPublicKey: hex.EncodeToString(key), PilotChecksumsSHA256: pilotHash, Audit: audit}, nil
}

// ReportQualified performs every gate before creating output. It archives only
// public qualification and sealed pilot evidence, never an operator private key.
func ReportQualified(run, pilot string, q analysis.DedicatedQualification, key ed25519.PublicKey, out string) error {
	if err := reportOutsideInputs([]string{run, pilot}, out); err != nil {
		return err
	}
	receipt, err := publicationReceipt(run, pilot, q, key)
	if err != nil {
		return err
	}
	return reportWithPasses(run, "", "", out, &publicationInput{pilot: pilot, qualification: q, key: key, Receipt: receipt})
}

func readPublicationQualification(root string) (analysis.DedicatedQualification, error) {
	f, err := os.Open(filepath.Join(root, "qualification.json"))
	if err != nil {
		return analysis.DedicatedQualification{}, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, (64<<10)+1))
	if err != nil {
		return analysis.DedicatedQualification{}, err
	}
	return analysis.DecodeDedicatedQualification(b)
}

func recomputePublicationReceipt(root string, recorded *PublicationReceipt, key ed25519.PublicKey) (*PublicationReceipt, error) {
	if recorded == nil || recorded.Version != PublicationArchiveVersion {
		return nil, fmt.Errorf("qualified publication receipt required")
	}
	if hex.EncodeToString(key) != recorded.OperatorPublicKey {
		return nil, fmt.Errorf("publication operator is not trusted by this reader")
	}
	q, err := readPublicationQualification(root)
	if err != nil {
		return nil, err
	}
	recomputed, err := publicationReceipt(filepath.Join(root, "raw"), filepath.Join(root, "raw-pilot"), q, key)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(recorded, recomputed) {
		return nil, fmt.Errorf("publication receipt differs from independently recomputed raw evidence")
	}
	var audit analysis.PublicationAudit
	if err := experiment.ReadJSON(filepath.Join(root, "publication-audit.json"), &audit); err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(audit, recomputed.Audit) {
		return nil, fmt.Errorf("publication audit sidecar differs from raw evidence")
	}
	return recomputed, nil
}

// VerifyPublicationReport authenticates operator trust and rechecks all gates.
// Plain VerifyReport establishes consistency under the recorded key only.
func VerifyPublicationReport(root string, key ed25519.PublicKey) error {
	if len(key) != ed25519.PublicKeySize {
		return fmt.Errorf("separately trusted Ed25519 public key required")
	}
	if err := VerifyReport(root); err != nil {
		return err
	}
	var d Dataset
	if err := experiment.ReadJSON(filepath.Join(root, "data.json"), &d); err != nil {
		return err
	}
	_, err := recomputePublicationReceipt(root, d.Publication, key)
	return err
}

func checkPublicationFiles(root string, d *Dataset) error {
	if d.Publication == nil {
		for _, name := range []string{"qualification.json", "publication-audit.json", "raw-pilot"} {
			if _, err := os.Lstat(filepath.Join(root, name)); err == nil {
				return fmt.Errorf("unrecorded publication evidence in exploratory report")
			} else if !os.IsNotExist(err) {
				return err
			}
		}
		return nil
	}
	key, err := hex.DecodeString(d.Publication.OperatorPublicKey)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid recorded publication key")
	}
	d.Publication, err = recomputePublicationReceipt(root, d.Publication, key)
	return err
}

// json is used here rather than a byte copy so qualification retains its strict,
// typed meaning; signatures cover canonical statements, not pretty-print layout.
func writePublicationEvidence(out string, input *publicationInput) error {
	if err := os.CopyFS(filepath.Join(out, "raw-pilot"), os.DirFS(input.pilot)); err != nil {
		return err
	}
	if err := experiment.WriteJSON(filepath.Join(out, "qualification.json"), input.qualification); err != nil {
		return err
	}
	if err := experiment.WriteJSON(filepath.Join(out, "publication-audit.json"), input.Receipt.Audit); err != nil {
		return err
	}
	q, err := readPublicationQualification(out)
	if err != nil {
		return err
	}
	a, _ := json.Marshal(q)
	b, _ := json.Marshal(input.qualification)
	if string(a) != string(b) {
		return fmt.Errorf("qualification changed while archiving")
	}
	_, err = recomputePublicationReceipt(out, input.Receipt, input.key)
	return err
}
