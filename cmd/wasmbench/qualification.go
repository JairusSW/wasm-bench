package main

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/wasmbench/wasmbench/analysis"
	"github.com/wasmbench/wasmbench/experiment"
)

func readQualificationFile(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("regular qualification file required")
	}
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("qualification file exceeds size bound")
	}
	return b, nil
}

func qualificationSeed(path string) (ed25519.PrivateKey, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || (runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0) {
		return nil, fmt.Errorf("operator seed must be a private regular file (0600 on Unix); no symlinks")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	actual, err := f.Stat()
	if err != nil || !os.SameFile(info, actual) {
		return nil, fmt.Errorf("operator seed changed while opening")
	}
	b, err := io.ReadAll(io.LimitReader(f, 1025))
	if err != nil {
		return nil, err
	}
	if len(b) > 1024 {
		return nil, fmt.Errorf("operator seed exceeds size bound")
	}
	seed, err := hex.DecodeString(strings.TrimSpace(string(b)))
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("operator seed must contain 32 bytes encoded as hexadecimal")
	}
	key := ed25519.NewKeyFromSeed(seed)
	clear(seed)
	clear(b)
	return key, nil
}

func qualificationCommand(command string, args []string) error {
	f := flags(command)
	runPath := f.String("run", "", "checksum-verified run; never modified")
	statement := f.String("statement", "", "reviewed operator statement JSON")
	seedFile := f.String("seed-file", "", "private Ed25519 seed file; never archived")
	out := f.String("out", "", "new output JSON file")
	if err := f.Parse(args); err != nil {
		return err
	}
	if *out == "" || f.NArg() != 0 {
		return fmt.Errorf("%s requires --out and no positional arguments", command)
	}
	if command == "qualification-public-key" {
		if *seedFile == "" || *runPath != "" || *statement != "" {
			return fmt.Errorf("qualification-public-key requires only --seed-file and --out")
		}
		key, err := qualificationSeed(*seedFile)
		if err != nil {
			return err
		}
		defer clear(key)
		return experiment.WriteJSON(*out, hex.EncodeToString(key.Public().(ed25519.PublicKey)))
	}
	if *runPath == "" {
		return fmt.Errorf("%s requires --run", command)
	}
	if err := experiment.OutputOutsideBundles(*out, *runPath); err != nil {
		return err
	}
	b, err := experiment.Load(*runPath)
	if err != nil {
		return err
	}
	checksum, err := experiment.DigestFile(filepath.Join(*runPath, "checksums.json"))
	if err != nil {
		return err
	}
	if command == "qualification-draft" {
		if *seedFile != "" || *statement != "" {
			return fmt.Errorf("qualification-draft does not accept a key or statement")
		}
		s, err := analysis.DedicatedQualificationDraft(b, checksum)
		if err != nil {
			return err
		}
		return experiment.WriteJSON(*out, s)
	}
	if command != "qualification-sign" || *seedFile == "" || *statement == "" {
		return fmt.Errorf("qualification-sign requires --run, --statement, --seed-file and --out")
	}
	raw, err := readQualificationFile(*statement, 64<<10)
	if err != nil {
		return err
	}
	s, err := analysis.DecodeDedicatedHostStatement(raw)
	if err != nil {
		return err
	}
	key, err := qualificationSeed(*seedFile)
	if err != nil {
		return err
	}
	defer clear(key)
	q, err := analysis.SignDedicatedQualification(s, key)
	if err != nil {
		return err
	}
	if err := analysis.VerifyDedicatedQualification(q, b, checksum, key.Public().(ed25519.PublicKey)); err != nil {
		return fmt.Errorf("refusing incomplete/unbound operator statement: %w", err)
	}
	return experiment.WriteJSON(*out, q)
}

func loadQualificationEvidence(run, qualification, keyPath string) analysis.QualificationEvidence {
	e := analysis.QualificationEvidence{}
	if qualification == "" && keyPath == "" {
		return e
	}
	if qualification == "" || keyPath == "" {
		e.Error = "both --qualification and --qualification-key are required"
		return e
	}
	raw, err := readQualificationFile(qualification, 64<<10)
	if err != nil {
		e.Error = err.Error()
		return e
	}
	q, err := analysis.DecodeDedicatedQualification(raw)
	if err != nil {
		e.Error = err.Error()
		return e
	}
	pub, err := readQualificationPublicKey(keyPath)
	if err != nil {
		e.Error = err.Error()
		return e
	}
	checksum, err := experiment.DigestFile(filepath.Join(run, "checksums.json"))
	if err != nil {
		e.Error = err.Error()
		return e
	}
	e.Qualification, e.TrustedPublicKey, e.ChecksumsSHA256 = &q, pub, checksum
	return e
}

func readQualificationPublicKey(path string) (ed25519.PublicKey, error) {
	data, err := readQualificationFile(path, 1024)
	if err != nil {
		return nil, err
	}
	var encoded string
	if err := json.Unmarshal(data, &encoded); err != nil {
		return nil, fmt.Errorf("trusted public key must be a JSON hexadecimal string")
	}
	pub, err := hex.DecodeString(encoded)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("trusted public key must contain exactly 32 Ed25519 bytes")
	}
	return pub, nil
}
