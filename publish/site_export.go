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

	"github.com/wasmbench/wasmbench/experiment"
)

const SiteExportVersion = "site-v2"
const SiteChunkBytes = 256 * 1024

type SiteObject struct {
	SHA256 string `json:"sha256"`
	Bytes  int    `json:"bytes"`
	Kind   string `json:"kind"`
}
type SiteManifest struct {
	Schema             int          `json:"schema"`
	Format             string       `json:"format"`
	ReportID           string       `json:"reportId"`
	SourceReportSHA256 string       `json:"sourceReportSha256"`
	SourceSealSHA256   string       `json:"sourceSealSha256"`
	Exporter           string       `json:"exporter"`
	Verification       string       `json:"verification"`
	Objects            []SiteObject `json:"objects"`
}
type SiteRecord struct {
	Kind string          `json:"kind"`
	ID   string          `json:"id"`
	Data json.RawMessage `json:"data"`
}

// Exact executable identity is separate from the runner that collected the data.
// Build settings are restricted to provenance fields, excluding local build paths.
func siteExporterIdentity() (map[string]any, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	digest, err := experiment.DigestFile(executable)
	if err != nil {
		return nil, err
	}
	identity := map[string]any{"format": SiteExportVersion, "binarySha256": digest}
	if info, ok := debug.ReadBuildInfo(); ok {
		identity["goVersion"] = info.GoVersion
		identity["module"] = info.Main.Path
		identity["moduleVersion"] = info.Main.Version
		settings := map[string]string{}
		for _, setting := range info.Settings {
			switch setting.Key {
			case "GOOS", "GOARCH", "vcs", "vcs.revision", "vcs.time", "vcs.modified":
				settings[setting.Key] = setting.Value
			}
		}
		identity["build"] = settings
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
	m := SiteManifest{Schema: 2, Format: SiteExportVersion, SourceReportSHA256: siteHash(data), SourceSealSHA256: siteHash(seal), Exporter: SiteExportVersion, Verification: "source-recomputed", Objects: []SiteObject{}}
	m.ReportID, err = siteID([]string{d.Bundle.Manifest.ID, m.SourceReportSHA256, m.SourceSealSHA256})
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	object := func(kind string, v any) (string, error) {
		b, e := siteJSON(v)
		if e != nil {
			return "", e
		}
		if len(b) > SiteChunkBytes {
			return "", fmt.Errorf("%s object exceeds %d decoded bytes", kind, SiteChunkBytes)
		}
		id := siteHash(b)
		if !seen[id] {
			if len(m.Objects) >= 512 {
				return "", fmt.Errorf("site export object inventory exceeds 512; split the corpus job")
			}
			if e = os.WriteFile(filepath.Join(out, "objects", id), b, 0644); e != nil {
				return "", e
			}
			m.Objects = append(m.Objects, SiteObject{id, len(b), kind})
			seen[id] = true
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
				return nil, fmt.Errorf("evidence row exceeds chunk ceiling")
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
	if err = record("report", m.ReportID, map[string]any{"runId": d.Bundle.Manifest.ID, "created": d.Bundle.Manifest.Created, "sourceReportSha256": m.SourceReportSHA256, "sourceSealSha256": m.SourceSealSHA256, "runnerSha256": d.Bundle.Manifest.Lock.RunnerSHA256, "exporterIdentity": exporter, "passContexts": passContexts, "versions": versions, "headlineLatencyPolicy": d.LatencyPolicy, "memorySource": d.MemorySource, "codeSource": d.CodeSource, "verification": m.Verification}); err != nil {
		return err
	}
	addResult := func(runtime, workload, scenario, profile, metric, statistic string, summary any, refs []string) error {
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
		if err = addResult(s.Runtime, s.Workload, s.Scenario, s.Profile, "time.wall", "median_ns_per_operation", compact, refs); err != nil {
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
		if err = addResult(s.Runtime, s.Workload, s.Scenario, sourceProfile, s.Metric, "median_bytes", compact, refs); err != nil {
			return err
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
			if err = addResult(c.Runtime, c.Workload, "compile", "code", metric, "size_bytes", summary, refs); err != nil {
				return err
			}
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
