package experiment

import (
	"fmt"
	"path"
	"sort"
	"strings"
	"unicode"
)

type recordedPath struct {
	windows bool
	volume  string
	parts   []string
}

// Recorded tool paths are provenance, not paths to open on the reader's host.
// Parse both producer dialects without delegating to the reader's filepath OS.
// Device namespaces, drive-relative paths and noncanonical aliases are refused.
func parseRecordedPath(source string) (recordedPath, error) {
	var result recordedPath
	if source == "" || len(source) > 32768 || strings.ContainsAny(source, "\x00\r\n") {
		return result, fmt.Errorf("invalid recorded tool path")
	}
	if strings.HasPrefix(source, "/") && !strings.HasPrefix(source, "//") {
		if path.Clean(source) != source || strings.Contains(source, "\\") || source == "/" {
			return result, fmt.Errorf("noncanonical POSIX tool path %q", source)
		}
		result.parts = strings.Split(strings.TrimPrefix(source, "/"), "/")
		return result, nil
	}
	result.windows = true
	value := strings.ReplaceAll(source, "\\", "/")
	var tail string
	if len(value) >= 3 && ((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')) && value[1:3] == ":/" {
		result.volume, tail = strings.ToLower(value[:2]), value[3:]
	} else if strings.HasPrefix(value, "//") {
		parts := strings.Split(value[2:], "/")
		if len(parts) < 3 || parts[0] == "" || parts[1] == "" || parts[0] == "?" || parts[0] == "." {
			return recordedPath{}, fmt.Errorf("invalid recorded UNC tool path %q", source)
		}
		result.volume, tail = "//"+recordedWindowsIdentity(parts[0])+"/"+recordedWindowsIdentity(parts[1]), strings.Join(parts[2:], "/")
		for _, component := range parts[:2] {
			if !portableWindowsComponent(component) {
				return recordedPath{}, fmt.Errorf("invalid recorded UNC volume")
			}
		}
	} else {
		return recordedPath{}, fmt.Errorf("recorded tool path must be absolute: %q", source)
	}
	if tail == "" || path.Clean(tail) != tail {
		return recordedPath{}, fmt.Errorf("noncanonical recorded Windows tool path %q", source)
	}
	result.parts = strings.Split(tail, "/")
	for _, component := range result.parts {
		if !portableWindowsComponent(component) {
			return recordedPath{}, fmt.Errorf("invalid recorded Windows path component")
		}
	}
	return result, nil
}

func portableWindowsComponent(value string) bool {
	if value == "" || value == "." || value == ".." || strings.HasSuffix(value, ".") || strings.HasSuffix(value, " ") || strings.ContainsAny(value, `<>:"|?*`) {
		return false
	}
	for _, r := range value {
		if r < 32 {
			return false
		}
	}
	base := strings.ToUpper(strings.SplitN(value, ".", 2)[0])
	if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" {
		return false
	}
	if len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9' {
		return false
	}
	return true
}

func recordedPathAbsolute(source string) bool {
	_, err := parseRecordedPath(source)
	return err == nil
}

func recordedPOSIXPath(source string) bool {
	p, err := parseRecordedPath(source)
	return err == nil && !p.windows
}

func recordedPathLooksAbsolute(source string) bool {
	return strings.HasPrefix(source, "/") || strings.HasPrefix(source, `\\`) ||
		(len(source) >= 3 && source[1] == ':' && (source[2] == '/' || source[2] == '\\'))
}

func recordedPathBase(source string) (string, error) {
	p, err := parseRecordedPath(source)
	if err != nil {
		return "", err
	}
	return p.parts[len(p.parts)-1], nil
}

// A conservative Unicode simple-fold key matches the EqualFold comparisons
// used for common Windows directory prefixes without a quadratic collision scan.
func recordedWindowsIdentity(value string) string {
	var key strings.Builder
	for _, r := range value {
		minimum := r
		for next := unicode.SimpleFold(r); next != r; next = unicode.SimpleFold(next) {
			if next < minimum {
				minimum = next
			}
		}
		key.WriteRune(minimum)
	}
	return key.String()
}

func portableArchiveRelativePaths(keys []string) (map[string]string, error) {
	parsed := map[string]recordedPath{}
	groups := map[string][]string{}
	var windows bool
	for i, source := range keys {
		p, err := parseRecordedPath(source)
		if err != nil {
			return nil, err
		}
		if i == 0 {
			windows = p.windows
		} else if windows != p.windows {
			return nil, fmt.Errorf("mixed producer tool path dialects")
		}
		parsed[source] = p
		groups[p.volume] = append(groups[p.volume], source)
	}
	volumes := []string{}
	for volume := range groups {
		volumes = append(volumes, volume)
	}
	sort.Strings(volumes)
	result := map[string]string{}
	for index, volume := range volumes {
		files := groups[volume]
		first := parsed[files[0]].parts
		common := len(first) - 1
		for _, source := range files[1:] {
			parts := parsed[source].parts
			common = min(common, len(parts)-1)
			for i := 0; i < common; i++ {
				equal := first[i] == parts[i]
				if windows {
					equal = strings.EqualFold(first[i], parts[i])
				}
				if !equal {
					common = i
					break
				}
			}
		}
		seen := map[string]bool{}
		for _, source := range files {
			rel := strings.Join(parsed[source].parts[common:], "/")
			if len(volumes) > 1 {
				rel = path.Join(fmt.Sprintf("volume-%d", index), rel)
			}
			identity := rel
			if windows {
				identity = recordedWindowsIdentity(rel)
			}
			if seen[identity] {
				return nil, fmt.Errorf("colliding recorded tool archive paths")
			}
			seen[identity] = true
			result[source] = rel
		}
	}
	return result, nil
}
