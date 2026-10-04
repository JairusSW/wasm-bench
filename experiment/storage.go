package experiment

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/wasmbench/wasmbench/agent"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func WriteJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}
func ReadJSON(path string, v any) error {
	b, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	return json.Unmarshal(b, v)
}
func DigestFile(path string) (string, error) {
	f, e := os.Open(path)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	if _, e := io.Copy(h, f); e != nil {
		return "", e
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func CopyExclusive(src, dst string) error {
	input, e := os.Open(src)
	if e != nil {
		return e
	}
	defer input.Close()
	if e = os.MkdirAll(filepath.Dir(dst), 0755); e != nil {
		return e
	}
	if copied, err := cloneExclusive(src, dst); copied || err != nil {
		return err
	}
	f, e := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if e != nil {
		return e
	}
	_, e = io.Copy(f, input)
	ce := f.Close()
	if e != nil {
		return e
	}
	return ce
}

// CopyTree preserves an independent bundle, using copy-on-write where available.
// Symlinks are rejected; destinations are new-only, like os.CopyFS.
func CopyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("cannot copy non-regular bundle file: %s", path)
		}
		// Immutable archived tools remain links to the shared cache in report
		// snapshots. Mutable evidence still receives an independent copy.
		if strings.HasPrefix(filepath.ToSlash(rel), "tools/") && info.Mode().Perm() == 0444 {
			digest, err := DigestFile(path)
			if err != nil {
				return err
			}
			return linkCachedTool(path, target, digest)
		}
		if err := CopyExclusive(path, target); err != nil {
			return err
		}
		return os.Chmod(target, info.Mode().Perm()|0200)
	})
}
func Seal(root string) error {
	hashes := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		rel, e := filepath.Rel(root, path)
		if e != nil {
			return e
		}
		if rel == "checksums.json" {
			return fmt.Errorf("bundle already sealed")
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink in bundle")
		}
		// Finder metadata at the bundle root is not benchmark evidence.
		if filepath.Base(rel) == ".DS_Store" {
			return nil
		}
		hash, e := DigestFile(path)
		hashes[filepath.ToSlash(rel)] = hash
		return e
	})
	if err != nil {
		return err
	}
	return WriteJSON(filepath.Join(root, "checksums.json"), hashes)
}
func Verify(root string) error {
	var hashes map[string]string
	if err := ReadJSON(filepath.Join(root, "checksums.json"), &hashes); err != nil {
		return err
	}
	if len(hashes) == 0 {
		return fmt.Errorf("empty bundle manifest")
	}
	for rel, want := range hashes {
		if !filepath.IsLocal(rel) {
			return fmt.Errorf("unsafe bundle path %q", rel)
		}
		// Legacy seals may include mutable Finder metadata. Preserve the seal
		// while checking every evidence file against its original digest.
		if filepath.Base(rel) == ".DS_Store" {
			continue
		}
		path := filepath.Join(root, rel)
		info, e := os.Lstat(path)
		if e != nil {
			return e
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("not a regular file: %s", rel)
		}
		got, e := DigestFile(path)
		if e != nil {
			return e
		}
		if got != want {
			return fmt.Errorf("bundle digest mismatch: %s", rel)
		}
	}
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if filepath.Base(rel) == ".DS_Store" {
			if d.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("symlink in bundle")
			}
			return nil
		}
		if rel != "checksums.json" {
			if _, ok := hashes[filepath.ToSlash(rel)]; !ok {
				return fmt.Errorf("unsealed file: %s", rel)
			}
		}
		return nil
	})
}
func Load(root string) (Bundle, error) {
	var b Bundle
	if err := Verify(root); err != nil {
		return b, err
	}
	if err := ReadJSON(filepath.Join(root, "manifest.json"), &b.Manifest); err != nil {
		return b, err
	}
	if err := verifyArchivedTools(b.Manifest.Lock, root); err != nil {
		return b, err
	}
	for _, r := range b.Manifest.Lock.Runtimes {
		if err := validateELFContract(r); err != nil {
			return b, err
		}
	}
	if err := validatePilotPlan(b.Manifest.Lock); err != nil {
		return b, err
	}
	if err := ValidateHostEvidence(b.Manifest); err != nil {
		return b, err
	}
	if a := b.Manifest.Lock.Analyzer; a != nil {
		if err := a.validate(); err != nil {
			return b, err
		}
		seen := map[string]json.RawMessage{}
		for _, w := range b.Manifest.Lock.Workloads {
			digest, err := hex.DecodeString(w.SHA256)
			if err != nil || len(digest) != sha256.Size {
				return b, fmt.Errorf("invalid admission artifact digest")
			}
			if data, ok := seen[w.SHA256]; ok {
				if err := validateArtifactABI(data, w.ABI); err != nil {
					return b, err
				}
				continue
			}
			// Offline loading checks saved evidence, never executes local programs.
			data, err := os.ReadFile(filepath.Join(root, "validation", w.SHA256+".json"))
			if err != nil {
				return b, err
			}
			if err := a.validateResult(data, w.SHA256); err != nil {
				return b, err
			}
			if err := validateArtifactABI(data, w.ABI); err != nil {
				return b, err
			}
			seen[w.SHA256] = data
			b.Admission = append(b.Admission, summarizeAdmission(a, w.SHA256, data))
		}
	} else {
		seen := map[string]bool{}
		for _, w := range b.Manifest.Lock.Workloads {
			if !seen[w.SHA256] {
				b.Admission = append(b.Admission, ArtifactAdmission{SHA256: w.SHA256, Status: "not_recorded", Reason: "legacy lock has no independent analyzer", FeatureStatus: "not_recorded"})
				seen[w.SHA256] = true
			}
		}
	}
	paths, e := filepath.Glob(filepath.Join(root, "trials", "*.json"))
	if e != nil {
		return b, e
	}
	sort.Strings(paths)
	for _, path := range paths {
		var t Trial
		if e := ReadJSON(path, &t); e != nil {
			return b, e
		}
		if t.Status == "ok" {
			if err := agent.ValidateNUMAIsolation(b.Manifest.Lock.Options.Resources, t.Isolation, "response_end_before_cleanup"); err != nil {
				return b, fmt.Errorf("trial %s: %w", t.ID, err)
			}
		}
		if t.CodeImage != nil {
			if t.Profile != "code" || t.Status != "ok" || t.Block < 0 {
				return b, fmt.Errorf("native code image outside successful code trial")
			}
			module := ""
			for _, w := range b.Manifest.Lock.Workloads {
				if w.ID == t.Workload {
					module = w.SHA256
					break
				}
			}
			if err := t.CodeImage.Validate(module); err != nil {
				return b, err
			}
		}
		if t.CPUProfile != nil {
			if t.Profile != "profiling" || t.Block < 0 {
				return b, fmt.Errorf("CPU profile outside measured profiling trial")
			}
			module := ""
			for _, w := range b.Manifest.Lock.Workloads {
				if w.ID == t.Workload {
					module = w.SHA256
					break
				}
			}
			if err := t.CPUProfile.Validate(module); err != nil {
				return b, err
			}
		}
		b.Trials = append(b.Trials, t)
	}
	if err := ValidatePartitionTrialEvidence(b); err != nil {
		return b, err
	}
	if err := ValidateCheckpointEvidence(root, b); err != nil {
		return b, err
	}
	if err := ValidateContinuationEvidence(root, b); err != nil {
		return b, err
	}
	if err := ValidateProcessSnapshotEvidence(root, b); err != nil {
		return b, err
	}
	if err := ValidateSnapshotDensityBundle(root, b); err != nil {
		return b, err
	}
	if err := ValidateGuestDensityEvidence(root, b); err != nil {
		return b, err
	}
	if err := ValidateSustainedEvidence(b); err != nil {
		return b, err
	}
	if err := ValidateTierEvidence(b); err != nil {
		return b, err
	}
	if err := ValidateEngineTraceEvidence(b); err != nil {
		return b, err
	}
	if err := ValidateMaterializationEvidence(root, b); err != nil {
		return b, err
	}
	if err := ValidateCodeLifetimeEvidence(root, b); err != nil {
		return b, err
	}
	if err := ValidateRustAllocatorEvidence(b); err != nil {
		return b, err
	}
	return b, nil
}
