package experiment

import (
	"debug/macho"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Only Mach-O loader resolution is qualified here. System shared-cache images
// remain OS prerequisites; third-party absolute loads are exact host files, not
// silently rebound to a different dylib with the same basename.
func pinNativeDependencies(r *Runtime) error {
	if len(r.Command) == 0 {
		return fmt.Errorf("empty command")
	}
	file, err := macho.Open(r.Command[0])
	if err != nil {
		return pinELFDependencies(r)
	} // ELF is qualified separately; other formats retain the declared-file contract.
	file.Close()
	r.NativeDependencyPolicy = "macho-third-party-closure-v1; system shared-cache libraries excluded; absolute loads remain exact host prerequisites"
	r.HostFiles = map[string]string{}
	seen := map[string]bool{}
	mainDir := filepath.Dir(r.Command[0])
	expand := func(value, loader string) string {
		if strings.HasPrefix(value, "@loader_path/") {
			return filepath.Join(filepath.Dir(loader), strings.TrimPrefix(value, "@loader_path/"))
		}
		if strings.HasPrefix(value, "@executable_path/") {
			return filepath.Join(mainDir, strings.TrimPrefix(value, "@executable_path/"))
		}
		return value
	}
	var visit func(string, bool, []string) error
	visit = func(path string, relocated bool, inherited []string) error {
		key := fmt.Sprintf("%t:%s", relocated, path)
		if seen[key] {
			return nil
		}
		if len(seen) >= 4096 {
			return fmt.Errorf("native dependency closure exceeds 4096 files")
		}
		seen[key] = true
		m, err := macho.Open(path)
		if err != nil {
			return err
		}
		defer m.Close()
		search := []string{}
		for _, load := range m.Loads {
			if rp, ok := load.(*macho.Rpath); ok {
				search = append(search, expand(rp.Path, path))
			}
		}
		search = append(search, inherited...)
		imports, err := m.ImportedLibraries()
		if err != nil {
			return err
		}
		for _, name := range imports {
			// These are supplied by the compatible macOS shared cache, not ordinary
			// distributable files. Do not pretend a missing on-disk image is zero bytes.
			if strings.HasPrefix(name, "/usr/lib/") || strings.HasPrefix(name, "/System/Library/") {
				continue
			}
			target := expand(name, path)
			if strings.HasPrefix(name, "@rpath/") {
				target = ""
				for _, dir := range search {
					candidate := filepath.Join(dir, strings.TrimPrefix(name, "@rpath/"))
					if info, e := os.Stat(candidate); e == nil && info.Mode().IsRegular() {
						target = candidate
						break
					}
				}
				if target == "" {
					return fmt.Errorf("unresolved native dependency %s from %s", name, path)
				}
			}
			if !filepath.IsAbs(target) {
				return fmt.Errorf("unsupported native load path %s", name)
			}
			target = filepath.Clean(target)
			digest, err := DigestFile(target)
			if err != nil {
				return err
			}
			moves := relocated && strings.HasPrefix(name, "@")
			if moves {
				r.Files[target] = digest
			} else {
				r.HostFiles[target] = digest
			}
			if err = visit(target, moves, search); err != nil {
				return err
			}
		}
		return nil
	}
	return visit(r.Command[0], true, nil)
}
