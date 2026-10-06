package publish

// The site transport is a projection of a verified Dataset. It performs no
// analysis and leaves the sealed report and legacy exports unchanged.
import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"unicode/utf8"

	"github.com/wasmbench/wasmbench/experiment"
)

const SiteExportVersion = "site-v2"
const SiteChunkBytes = 256 * 1024
const SiteResourceBytes = 64 * 1024 * 1024
const SiteFragmentBytes = 120 * 1024
const SiteInventoryObjects = 512
const SiteInventoryPages = 512

type SiteInventory struct {
	SHA256       string `json:"sha256"`
	Bytes        int    `json:"bytes"`
	Objects      int    `json:"objects"`
	ContentBytes int64  `json:"contentBytes"`
}

type siteInventoryPage struct {
	Schema  int          `json:"schema"`
	Objects []SiteObject `json:"objects"`
}

type SiteExporterIdentity struct {
	Format        string            `json:"format"`
	BinarySHA256  string            `json:"binarySha256"`
	GoVersion     string            `json:"goVersion,omitempty"`
	Module        string            `json:"module,omitempty"`
	ModuleVersion string            `json:"moduleVersion,omitempty"`
	Build         map[string]string `json:"build,omitempty"`
}

type SiteObject struct {
	SHA256 string `json:"sha256"`
	Bytes  int    `json:"bytes"`
	Kind   string `json:"kind"`
}
type SiteManifest struct {
	Schema             int                   `json:"schema"`
	Format             string                `json:"format"`
	ReportID           string                `json:"reportId"`
	SourceReportSHA256 string                `json:"sourceReportSha256"`
	SourceSealSHA256   string                `json:"sourceSealSha256"`
	Exporter           string                `json:"exporter"`
	Verification       string                `json:"verification"`
	Objects            []SiteObject          `json:"objects"`
	InventoryPages     []SiteInventory       `json:"inventoryPages,omitempty"`
	ExporterIdentity   *SiteExporterIdentity `json:"exporterIdentity,omitempty"`
}
type SiteRecord struct {
	Kind string          `json:"kind"`
	ID   string          `json:"id"`
	Data json.RawMessage `json:"data"`
}

// Exact executable identity is separate from the runner that collected the data.
// Build settings are restricted to provenance fields, excluding local build paths.
func siteExporterIdentity() (*SiteExporterIdentity, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	digest, err := experiment.DigestFile(executable)
	if err != nil {
		return nil, err
	}
	identity := &SiteExporterIdentity{Format: SiteExportVersion, BinarySHA256: digest}
	if info, ok := debug.ReadBuildInfo(); ok {
		identity.GoVersion = info.GoVersion
		identity.Module = info.Main.Path
		identity.ModuleVersion = info.Main.Version
		settings := map[string]string{}
		for _, setting := range info.Settings {
			switch setting.Key {
			case "GOOS", "GOARCH", "vcs", "vcs.revision", "vcs.time", "vcs.modified":
				settings[setting.Key] = setting.Value
			}
		}
		identity.Build = settings
	}
	return identity, nil
}

func siteHash(b []byte) string       { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func siteJSON(v any) ([]byte, error) { return json.Marshal(v) }
func siteID(v any) (string, error) {
	b, e := siteJSON(v)
	if e != nil {
		return "", e
	}
	return siteHash(b), nil
}

// ExportSite verifies with the installed, trusted builder. It never executes
// the report's archived executable. Historical builder mismatches fail closed.
func ExportSite(report, out string) error {
	if err := reportOutsideInputs([]string{report}, out); err != nil {
		return err
	}
	if err := VerifyReport(report); err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(report, "data.json"))
	if err != nil {
		return err
	}
	seal, err := os.ReadFile(filepath.Join(report, "checksums.json"))
	if err != nil {
		return err
	}
	var d Dataset
	if err = json.Unmarshal(data, &d); err != nil {
		return err
	}
	if d.Bundle.Manifest.Kind != "measurement" || d.Bundle.Manifest.Lock.Options.Profile != "timing" || d.Bundle.Manifest.Lock.Options.Check {
		return fmt.Errorf("site export requires a complete timing measurement report")
	}
	bundles := []experiment.Bundle{}
	if d.MemorySource != nil && d.MemorySource.Profile == "memory" {
		b, err := experiment.Load(filepath.Join(report, "raw-memory"))
		if err != nil {
			return err
		}
		bundles = append(bundles, b)
	}
	if d.CodeSource != nil {
		b, err := experiment.Load(filepath.Join(report, "code", "raw"))
		if err != nil {
			return err
		}
		bundles = append(bundles, b)
	}
	return writeSiteDataset(d, data, seal, out, bundles...)
}

func writeSiteDataset(d Dataset, data, seal []byte, out string, extra ...experiment.Bundle) (err error) {
	if err = os.Mkdir(out, 0755); err != nil {
		return err
	}
	// A failed export must not leave an apparently complete manifest.
	defer func() {
		if err != nil {
			_ = os.RemoveAll(out)
		}
	}()
	if err = os.Mkdir(filepath.Join(out, "objects"), 0755); err != nil {
		return err
	}
	exporter, err := siteExporterIdentity()
	if err != nil {
		return err
	}
	m := SiteManifest{Schema: 2, Format: SiteExportVersion, SourceReportSHA256: siteHash(data), SourceSealSHA256: siteHash(seal), Exporter: SiteExportVersion, Verification: "source-recomputed", Objects: []SiteObject{}, ExporterIdentity: exporter}
	m.ReportID, err = siteID([]string{d.Bundle.Manifest.ID, m.SourceReportSHA256, m.SourceSealSHA256})
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	var object func(string, any) (string, error)
	object = func(kind string, v any) (string, error) {
		b, e := siteJSON(v)
		if e != nil {
			return "", e
		}
		if len(b) > SiteChunkBytes {
			if kind != "evidence" || len(b) > SiteResourceBytes {
				return "", fmt.Errorf("%s object exceeds resource ceiling", kind)
			}
			references := []string{}
			for start := 0; start < len(b); {
				end := min(start+SiteFragmentBytes, len(b))
				// Each fragment is valid UTF-8, while JSON escape sequences may span
				// fragments. Reassembly concatenates text bytes before parsing JSON.
				for end < len(b) && !utf8.RuneStart(b[end]) {
					end--
				}
				fragment, e := object("evidence", map[string]any{"kind": "json-fragment", "schema": 1, "text": string(b[start:end])})
				if e != nil {
					return "", e
				}
				references = append(references, fragment)
				start = end
			}
			return object("evidence", map[string]any{"kind": "json-resource", "schema": 1, "encoding": "json-utf8", "bytes": len(b), "sha256": siteHash(b), "references": references})
		}
		id := siteHash(b)
		if !seen[id] {
			if len(m.Objects) >= SiteInventoryObjects*SiteInventoryPages {
				return "", fmt.Errorf("site export exceeds %d objects", SiteInventoryObjects*SiteInventoryPages)
			}
			if e = os.WriteFile(filepath.Join(out, "objects", id), b, 0644); e != nil {
				return "", e
			}
			m.Objects = append(m.Objects, SiteObject{id, len(b), kind})
			seen[id] = true
		}
		return id, nil
	}
	binary := func(b []byte) (string, error) {
		if len(b) > 16*1024*1024 {
			return "", fmt.Errorf("native bytes exceed producer contract")
		}
		id := siteHash(b)
		if !seen[id] {
			if len(m.Objects) >= SiteInventoryObjects*SiteInventoryPages {
				return "", fmt.Errorf("site inventory exceeds ceiling")
			}
			if e := os.WriteFile(filepath.Join(out, "objects", id), b, 0644); e != nil {
				return "", e
			}
			m.Objects = append(m.Objects, SiteObject{id, len(b), "binary"})
			seen[id] = true
		} else {
			for _, descriptor := range m.Objects {
				if descriptor.SHA256 == id && descriptor.Kind != "binary" {
					return "", fmt.Errorf("binary collides with JSON representation")
				}
			}
		}
		return id, nil
	}
	record := func(kind string, id string, v any) error {
		b, e := siteJSON(v)
		if e != nil {
			return e
		}
		_, e = object("record", SiteRecord{kind, id, b})
		return e
	}
	environment, err := siteID(d.Bundle.Manifest.Host)
	if err != nil {
		return err
	}
	if err = record("environment", environment, d.Bundle.Manifest.Host); err != nil {
		return err
	}
	configs, contracts := map[string]string{}, map[string]string{}
	tracks, definitions := map[string]string{}, map[string]string{}
	sourceRegistered := map[string]bool{}
	for _, c := range d.Bundle.Manifest.Lock.Runtimes {
		id, e := siteID(c)
		if e != nil {
			return e
		}
		configs[c.ID] = id
		backend := ""
		family := c.ID
		if c.Description != nil {
			backend = c.Description.Backend
			family = c.Description.Runtime
		}
		track := map[string]string{"runtime": family, "backend": backend, "sourceRuntimeId": c.ID}
		trackID, e := siteID(track)
		if e != nil {
			return e
		}
		tracks[c.ID] = trackID
		if err = record("track", trackID, track); err != nil {
			return err
		}
		if err = record("configuration", id, c); err != nil {
			return err
		}
	}
	for _, w := range d.Bundle.Manifest.Lock.Workloads {
		id, e := siteID(w)
		if e != nil {
			return e
		}
		contracts[w.ID] = id
		if err = record("workload", id, w); err != nil {
			return err
		}
	}
	for _, def := range d.Metrics {
		id, e := siteID(def)
		if e != nil {
			return e
		}
		if err = record("metric", id, def); err != nil {
			return err
		}
		definitions[def.Name] = id
		sourceRegistered[def.Name] = true
	}
	versions := map[string]json.RawMessage{}
	var source map[string]json.RawMessage
	if err = json.Unmarshal(data, &source); err != nil {
		return err
	}
	for k, v := range source {
		if len(k) >= 8 && k[len(k)-8:] == "_version" {
			versions[k] = v
		}
	}

	// Launch/block evidence lives in independently readable objects. Split arrays
	// without turning samples into new independent launches.
	chunks := func(v any) ([]string, error) {
		b, e := siteJSON(v)
		if e != nil {
			return nil, e
		}
		var rows []json.RawMessage
		if e = json.Unmarshal(b, &rows); e != nil {
			return nil, e
		}
		ids := []string{}
		part := []json.RawMessage{}
		size := 2
		flush := func() error {
			if len(part) == 0 {
				return nil
			}
			id, e := object("evidence", part)
			if e != nil {
				return e
			}
			ids = append(ids, id)
			part = nil
			size = 2
			return nil
		}
		for _, row := range rows {
			if len(row)+2 > SiteChunkBytes {
				if e = flush(); e != nil {
					return nil, e
				}
				id, e := object("evidence", []json.RawMessage{row})
				if e != nil {
					return nil, e
				}
				ids = append(ids, id)
				continue
			}
			if size+len(row)+1 > SiteChunkBytes {
				if e = flush(); e != nil {
					return nil, e
				}
			}
			part = append(part, row)
			size += len(row) + 1
		}
		if e = flush(); e != nil {
			return nil, e
		}
		return ids, nil
	}
	evidence := map[string][]string{}
	trialEvidence := map[string]string{}
	passContexts := []string{}
	for _, bundle := range append([]experiment.Bundle{d.Bundle}, extra...) {
		contextID, e := object("evidence", map[string]any{"kind": "pass-context", "manifest": bundle.Manifest, "admission": bundle.Admission})
		if e != nil {
			return e
		}
		passContexts = append(passContexts, contextID)
		for _, t := range bundle.Trials {
			samples, e := chunks(t.Samples)
			if e != nil {
				return e
			}
			observations, e := chunks(t.Observations)
			if e != nil {
				return e
			}
			adapterSamples, e := chunks(t.AdapterSamples)
			if e != nil {
				return e
			}
			phaseEvents, e := chunks(t.PhaseEvents)
			if e != nil {
				return e
			}
			raw, e := siteJSON(t)
			if e != nil {
				return e
			}
			var details map[string]json.RawMessage
			if e = json.Unmarshal(raw, &details); e != nil {
				return e
			}
			// Native image content needs its own binary transport; retain the measured
			// descriptor separately rather than copying base64 into JSON evidence.
			for _, field := range []string{"samples", "observations", "adapter_samples", "phase_events", "code_image"} {
				delete(details, field)
			}
			detailID, e := object("evidence", map[string]any{"kind": "trial-details", "data": details})
			if e != nil {
				return e
			}
			references := append([]string{contextID, detailID}, adapterSamples...)
			references = append(references, phaseEvents...)
			id, e := object("evidence", map[string]any{"reportId": m.ReportID, "passId": bundle.Manifest.ID, "trialId": t.ID, "block": t.Block, "profile": t.Profile, "scenario": t.Scenario, "status": t.Status, "reason": t.Reason, "samples": samples, "observations": observations, "passContext": contextID, "details": detailID, "adapterSamples": adapterSamples, "phaseEvents": phaseEvents, "references": references})
			if e != nil {
				return e
			}
			key := t.Runtime + "\x00" + t.Workload + "\x00" + t.Scenario + "\x00" + t.Profile
			evidence[key] = append(evidence[key], id)
			trialEvidence[bundle.Manifest.ID+"\x00"+t.ID] = id
		}
	}
	if err = record("report", m.ReportID, map[string]any{"runId": d.Bundle.Manifest.ID, "created": d.Bundle.Manifest.Created, "sourceReportSha256": m.SourceReportSHA256, "sourceSealSha256": m.SourceSealSHA256, "runnerSha256": d.Bundle.Manifest.Lock.RunnerSHA256, "passContexts": passContexts, "versions": versions, "headlineLatencyPolicy": d.LatencyPolicy, "memorySource": d.MemorySource, "codeSource": d.CodeSource, "verification": m.Verification}); err != nil {
		return err
	}
	addResult := func(runtime, workload, scenario, profile, metric, statistic, pass string, trials map[string]bool, summary any, refs []string) error {
		if configs[runtime] == "" || contracts[workload] == "" {
			return fmt.Errorf("result references unknown configuration or contract")
		}
		definitionStatus := "available"
		// Current code records contain a metric absent from their report registry.
		// This marker records that omission; it is not an invented definition.
		if metric == "native.code_size" && !sourceRegistered[metric] {
			definitionStatus = "unregistered"
			if definitions[metric] == "" {
				marker := map[string]any{"name": metric, "status": "unregistered", "reason": "code records use this metric but the source report registry omits its definition"}
				id, e := siteID(marker)
				if e != nil {
					return e
				}
				if err = record("metric", id, marker); err != nil {
					return err
				}
				definitions[metric] = id
			}
		}
		if definitions[metric] == "" {
			return fmt.Errorf("metric missing from source registry: %s", metric)
		}
		value := map[string]any{"reportId": m.ReportID, "environmentId": environment, "configurationId": configs[runtime], "trackId": tracks[runtime], "runtime": runtime, "contractId": contracts[workload], "workload": workload, "scenario": scenario, "profile": profile, "metric": metric, "metricDefinitionId": definitions[metric], "metricDefinitionStatus": definitionStatus, "statistic": statistic, "created": d.Bundle.Manifest.Created, "analysisVersion": d.AnalysisVersion, "summary": summary, "evidence": refs}
		method, e := siteMethod(append([]experiment.Bundle{d.Bundle}, extra...), pass, runtime, workload, scenario, profile, metric, statistic, trials)
		if e != nil {
			return e
		}
		value["measurementMethod"] = method
		value["measurementMethodId"], e = siteID(method)
		if e != nil {
			return e
		}
		id, e := siteID(value)
		if e != nil {
			return e
		}
		return record("result", id, value)
	}
	for _, s := range d.Summaries {
		b, e := siteJSON(s)
		if e != nil {
			return e
		}
		var compact map[string]json.RawMessage
		if e = json.Unmarshal(b, &compact); e != nil {
			return e
		}
		diagnostics := map[string]json.RawMessage{}
		for _, field := range []string{"launch_medians", "warmup_diagnostics"} {
			if value, ok := compact[field]; ok {
				diagnostics[field] = value
			}
			delete(compact, field)
		}
		refs := append([]string{}, evidence[s.Runtime+"\x00"+s.Workload+"\x00"+s.Scenario+"\x00"+s.Profile]...)
		diagnosticID, e := object("evidence", map[string]any{"kind": "summary-diagnostics", "reportId": m.ReportID, "runtime": s.Runtime, "workload": s.Workload, "scenario": s.Scenario, "profile": s.Profile, "data": diagnostics})
		if e != nil {
			return e
		}
		refs = append(refs, diagnosticID)
		// Preserve the producer's interval and unavailable reason byte-for-value.
		if err = addResult(s.Runtime, s.Workload, s.Scenario, s.Profile, "time.wall", "median_ns_per_operation", d.Bundle.Manifest.ID, nil, compact, refs); err != nil {
			return err
		}
	}
	for _, s := range d.MemoryStages {
		sourceRun, sourceProfile := s.SourceRun, s.SourceProfile
		if d.MemorySource != nil {
			if sourceRun == "" {
				sourceRun = d.MemorySource.ID
			}
			if sourceProfile == "" {
				sourceProfile = d.MemorySource.Profile
			}
		}
		refs, e := chunks(s.LaunchValues)
		if e != nil {
			return e
		}
		b, e := siteJSON(s)
		if e != nil {
			return e
		}
		var compact map[string]json.RawMessage
		if e = json.Unmarshal(b, &compact); e != nil {
			return e
		}
		delete(compact, "launch_values")
		delete(compact, "trial_ids")
		for _, trial := range s.Trials {
			if id := trialEvidence[sourceRun+"\x00"+trial]; id != "" {
				refs = append(refs, id)
			}
		}
		trialSet := map[string]bool{}
		for _, id := range s.Trials {
			trialSet[id] = true
		}
		if err = addResult(s.Runtime, s.Workload, s.Scenario, sourceProfile, s.Metric, "median_bytes", sourceRun, trialSet, compact, refs); err != nil {
			return err
		}
	}
	codeTrials := map[string]experiment.Trial{}
	for _, bundle := range extra {
		if d.CodeSource != nil && bundle.Manifest.ID == d.CodeSource.ID {
			for _, trial := range bundle.Trials {
				codeTrials[trial.ID] = trial
			}
		}
	}
	for _, c := range d.CodeRecords {
		// Size is meaningful even when no raw image was exported in this transport.
		codeJSON, e := siteJSON(c)
		if e != nil {
			return e
		}
		var codeValue map[string]json.RawMessage
		if e = json.Unmarshal(codeJSON, &codeValue); e != nil {
			return e
		}
		if c.SizeBytes != nil && *c.SizeBytes > 1<<53-1 {
			codeValue["size_bytes"], e = siteJSON(strconv.FormatUint(*c.SizeBytes, 10))
			if e != nil {
				return e
			}
		}
		descriptor := map[string]any{"reportId": m.ReportID, "record": codeValue, "measurementAvailable": c.ImageBytes != nil || c.SizeBytes != nil, "content": map[string]string{"status": "unavailable", "reason": "native bytes are not exported by site-v2 yet"}, "inspection": map[string]string{"status": "unavailable"}}
		// Reuse the native export admission rules. Withheld or size-only records
		// retain unavailable content even when a diagnostic trial exists.
		if trial, ok := codeTrials[c.Trial]; ok && c.Status == "available" {
			if trial.Runtime != c.Runtime || trial.Workload != c.Workload || trial.Profile != "code" || trial.Scenario != "compile" {
				return fmt.Errorf("native trial identity differs")
			}
			exported := nativeRecord(trial, c.Index)
			if exported.Status == "available" && exported.Image != nil && trial.CodeImage != nil {
				module := ""
				for _, workload := range d.Bundle.Manifest.Lock.Workloads {
					if workload.ID == c.Workload {
						module = workload.SHA256
					}
				}
				if e := trial.CodeImage.Validate(module); e != nil {
					return e
				}
				hash, e := binary(trial.CodeImage.Data)
				if e != nil {
					return e
				}
				if c.ImageBytes == nil || *c.ImageBytes != len(trial.CodeImage.Data) {
					return fmt.Errorf("native image size differs from report")
				}
				image := *exported.Image
				functions, e := chunks(image.Functions)
				if e != nil {
					return e
				}
				image.Functions = nil
				metadata, e := object("evidence", map[string]any{"kind": "native-image-metadata", "reportId": m.ReportID, "passId": d.CodeSource.ID, "trialId": trial.ID, "image": image, "functions": functions, "references": functions})
				if e != nil {
					return e
				}
				descriptor["content"] = map[string]any{"status": "available", "sha256": hash, "bytes": len(trial.CodeImage.Data), "mediaType": "application/octet-stream"}
				status := "unavailable"
				if len(functions) > 0 {
					status = "available"
				}
				descriptor["inspection"] = map[string]any{"status": status, "metadata": metadata, "functionAttribution": image.FunctionAttribution, "disassembly": map[string]string{"status": "unavailable", "reason": "offline derivative not exported"}}
				descriptor["target"] = map[string]string{"architecture": image.Architecture, "backend": image.Backend}
				descriptor["interpretation"] = nativeExportInterpretation
				if image.Version == 3 {
					descriptor["interpretation"] = nativeMaterializedExportInterpretation
				}
			}
		}
		descriptorJSON, e := siteJSON(descriptor)
		if e != nil {
			return e
		}
		if len(descriptorJSON)+512 > 10*1024 {
			return fmt.Errorf("artifact descriptor exceeds 10 KiB")
		}
		id, e := siteID(descriptor)
		if e != nil {
			return e
		}
		if err = record("artifact", id, descriptor); err != nil {
			return err
		}
		if c.SizeBytes != nil || c.ImageBytes != nil {
			var size any
			metric := "native.code_image"
			if c.SizeBytes != nil {
				metric = "native.code_size"
				size = *c.SizeBytes
				if *c.SizeBytes > 1<<53-1 {
					size = strconv.FormatUint(*c.SizeBytes, 10)
				}
			} else {
				size = *c.ImageBytes
			}
			summary := map[string]any{"status": "available", "size_bytes": size, "size_note": c.SizeNote, "artifactId": id, "source_run": d.CodeSource, "trial_id": c.Trial}
			refs := []string{}
			if d.CodeSource != nil {
				if trial := trialEvidence[d.CodeSource.ID+"\x00"+c.Trial]; trial != "" {
					refs = append(refs, trial)
				}
			}
			pass := ""
			if d.CodeSource != nil {
				pass = d.CodeSource.ID
			}
			if err = addResult(c.Runtime, c.Workload, "compile", "code", metric, "size_bytes", pass, map[string]bool{c.Trial: true}, summary, refs); err != nil {
				return err
			}
		}
	}
	// Small exports retain their original manifest encoding. Large exports list
	// independently verified inventory pages rather than embedding every object.
	// Page byte/count commitments permit consumers to reserve the full declared
	// payload before they have downloaded the inventory itself.
	if len(m.Objects) > SiteInventoryObjects {
		objects := m.Objects
		m.Objects = []SiteObject{}
		for start := 0; start < len(objects); start += SiteInventoryObjects {
			end := min(start+SiteInventoryObjects, len(objects))
			page := siteInventoryPage{Schema: 1, Objects: objects[start:end]}
			bytes, e := siteJSON(page)
			if e != nil {
				return e
			}
			if len(bytes) > SiteChunkBytes {
				return fmt.Errorf("inventory page exceeds ceiling")
			}
			digest := siteHash(bytes)
			// An inventory occupies the same immutable namespace as payload objects.
			// Never overwrite any previously emitted representation.
			if seen[digest] {
				return fmt.Errorf("inventory digest collides with payload")
			}
			if e = os.WriteFile(filepath.Join(out, "objects", digest), bytes, 0644); e != nil {
				return e
			}
			seen[digest] = true
			var contentBytes int64
			for _, descriptor := range page.Objects {
				contentBytes += int64(descriptor.Bytes)
			}
			m.InventoryPages = append(m.InventoryPages, SiteInventory{SHA256: digest, Bytes: len(bytes), Objects: len(page.Objects), ContentBytes: contentBytes})
		}
	}
	b, err := siteJSON(m)
	if err != nil {
		return err
	}
	if len(b) > SiteChunkBytes {
		return fmt.Errorf("manifest exceeds ceiling")
	}
	return os.WriteFile(filepath.Join(out, "manifest.json"), b, 0644)
}
