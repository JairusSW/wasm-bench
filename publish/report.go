package publish

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/json"
	"fmt"
	"github.com/wasmbench/wasmbench/analysis"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/metrics"
	"html/template"
	"os"
	"path/filepath"
)

//go:embed report.html
var assets embed.FS

type Dataset struct {
	Publication                    *PublicationReceipt          `json:"publication,omitempty"`
	SnapshotDensityViews           []SnapshotDensityView        `json:"snapshot_density_views,omitempty"`
	SnapshotDensityViewVersion     string                       `json:"snapshot_density_view_version,omitempty"`
	SnapshotDensityCurves          []analysis.ScalingCurve      `json:"snapshot_density_curves,omitempty"`
	SnapshotDensityAnalysisVersion string                       `json:"snapshot_density_analysis_version,omitempty"`
	SnapshotMemoryViews            []SnapshotMemoryView         `json:"snapshot_memory_views,omitempty"`
	SnapshotMemoryViewVersion      string                       `json:"snapshot_memory_view_version,omitempty"`
	CodeLifetimeExportVersion      string                       `json:"code_lifetime_export_version"`
	EngineTraceExportVersion       string                       `json:"engine_trace_export_version"`
	BuilderArchiveVersion          string                       `json:"report_builder_archive_version,omitempty"`
	Pareto                         []ParetoPoint                `json:"pareto"`
	ParetoVersion                  string                       `json:"pareto_analysis_version"`
	SustainedSessions              []analysis.SustainedSession  `json:"sustained_sessions,omitempty"`
	SustainedVersion               string                       `json:"sustained_analysis_version,omitempty"`
	Throughput                     []analysis.ThroughputSummary `json:"throughput"`
	ThroughputVersion              string                       `json:"throughput_analysis_version"`
	LatencyPolicy                  analysis.LatencyPolicy       `json:"headline_latency_policy"`
	SampleExportVersion            string                       `json:"sample_export_version"`
	CPUStacks                      []analysis.CPUStackProfile   `json:"cpu_stacks"`
	CounterDisplay                 []CounterDisplayRow          `json:"counter_display"`
	CounterDisplayVersion          string                       `json:"counter_display_version"`
	Schema                         int                          `json:"schema"`
	AnalysisVersion                string                       `json:"analysis_version"`
	Bundle                         experiment.Bundle            `json:"bundle"`
	ArtifactStructures             []ArtifactStructure          `json:"artifact_structures"`
	Summaries                      []analysis.Summary           `json:"summaries"`
	Metrics                        []metrics.Definition         `json:"metrics"`
	Scenarios                      []metrics.Scenario           `json:"scenarios"`
	Scaling                        []analysis.ScalingCurve      `json:"scaling"`
	MemoryScaling                  []analysis.ScalingCurve      `json:"memory_scaling,omitempty"`
	MemoryScalingTrials            []ScalingTrialLink           `json:"memory_scaling_trials,omitempty"`
	ScalingVersion                 string                       `json:"scaling_analysis_version"`
	BreakEven                      *analysis.BreakEvenReport    `json:"break_even,omitempty"`
	MemoryTimelines                []analysis.MemoryTimeline    `json:"memory_timelines"`
	PairedMemoryTimelines          []analysis.MemoryTimeline    `json:"paired_memory_timelines,omitempty"`
	PairedDensityFootprints        []analysis.DensityFootprint  `json:"paired_density_footprints,omitempty"`
	MemoryEvidenceVersion          string                       `json:"memory_evidence_version,omitempty"`
	MemoryTimelineVersion          string                       `json:"memory_timeline_analysis_version"`
	MemoryStages                   []MemoryStage                `json:"memory_stages,omitempty"`
	MemorySource                   *MemorySource                `json:"memory_source,omitempty"`
	CodeRecords                    []CodeRecord                 `json:"code_records,omitempty"`
	CodeSource                     *CodeSource                  `json:"code_source,omitempty"`
	PhaseCPU                       []PhaseCPUStage              `json:"phase_cpu,omitempty"`
	PhaseCPUVersion                string                       `json:"phase_cpu_version"`
	RendererSHA256                 string                       `json:"renderer_sha256"`
	DensityFootprints              []analysis.DensityFootprint  `json:"density_footprints"`
	DensityFootprintVersion        string                       `json:"density_footprint_analysis_version"`
}

func Report(run, out string) error {
	return ReportWithMemory(run, "", out)
}

func ReportWithMemory(run, memoryRun, out string) error {
	return ReportWithPasses(run, memoryRun, "", out)
}

func ReportWithPasses(run, memoryRun, codeRun, out string) error {
	return reportWithPasses(run, memoryRun, codeRun, out, nil)
}

func reportWithPasses(run, memoryRun, codeRun, out string, publication *publicationInput) error {
	inputs := []string{run}
	if memoryRun != "" {
		inputs = append(inputs, memoryRun)
	}
	if codeRun != "" {
		inputs = append(inputs, codeRun)
	}
	if err := reportOutsideInputs(inputs, out); err != nil {
		return err
	}
	d, e := buildReportDataset(run, memoryRun, codeRun)
	if e != nil {
		return e
	}
	b := d.Bundle
	if publication != nil {
		d.Publication = publication.Receipt
	}
	if e = os.MkdirAll(filepath.Dir(out), 0755); e != nil {
		return e
	}
	if e = os.Mkdir(out, 0755); e != nil {
		return e
	}
	if e = experiment.WriteJSON(filepath.Join(out, "data.json"), d); e != nil {
		return e
	}
	if e = ExportParquet(b, filepath.Join(out, "samples.parquet")); e != nil {
		return e
	}
	if e = ExportThroughput(b.Manifest.ID, d.Throughput, filepath.Join(out, "throughput.parquet")); e != nil {
		return e
	}
	if e = ExportObservations(b, filepath.Join(out, "observations.parquet")); e != nil {
		return e
	}
	if e = ExportCounters(b, filepath.Join(out, "counters.parquet")); e != nil {
		return e
	}
	if e = exportProfiles(b, out); e != nil {
		return e
	}
	if e = exportEngineTraces(b, out); e != nil {
		return e
	}
	if e = ExportEngineTraces(b, filepath.Join(out, "engine-events.parquet")); e != nil {
		return e
	}
	if e = ExportCodeLifetimes(b, filepath.Join(out, "code-lifetimes.parquet")); e != nil {
		return e
	}
	if e = os.CopyFS(filepath.Join(out, "raw"), os.DirFS(run)); e != nil {
		return e
	}
	if publication != nil {
		if err := writePublicationEvidence(out, publication); err != nil {
			return err
		}
	}
	if memoryRun != "" {
		memory, err := experiment.Load(memoryRun)
		if err != nil {
			return err
		}
		if err := ExportParquet(memory, filepath.Join(out, "memory-samples.parquet")); err != nil {
			return err
		}
		if err := ExportObservations(memory, filepath.Join(out, "memory-observations.parquet")); err != nil {
			return err
		}
		if e = os.CopyFS(filepath.Join(out, "raw-memory"), os.DirFS(memoryRun)); e != nil {
			return e
		}
	}
	if codeRun != "" {
		if e = ExportNativeCode(codeRun, filepath.Join(out, "code")); e != nil {
			return e
		}
	}
	html, e := renderReportHTML(d)
	if e != nil {
		return e
	}
	f, e := os.OpenFile(filepath.Join(out, "index.html"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if e != nil {
		return e
	}
	if _, e = f.Write(html); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	if err := archiveReportBuilder(out, d); err != nil {
		return err
	}
	if err := experiment.Seal(out); err != nil {
		return err
	}
	return VerifyReport(out)
}

func buildReportDataset(run, memoryRun, codeRun string) (Dataset, error) {
	if err := metrics.Validate(); err != nil {
		return Dataset{}, fmt.Errorf("metric/scenario registry: %w", err)
	}
	b, e := experiment.Load(run)
	if e != nil {
		return Dataset{}, e
	}
	if b.Manifest.Kind != "measurement" {
		return Dataset{}, fmt.Errorf("correctness-only bundle is not performance data")
	}
	asset, err := assets.ReadFile("report.html")
	if err != nil {
		return Dataset{}, err
	}
	d := Dataset{Schema: 1, AnalysisVersion: analysis.Version, Bundle: b, Summaries: analysis.Summarize(b), Metrics: metrics.Registry, Scenarios: metrics.Scenarios, PhaseCPUVersion: PhaseCPUVersion, RendererSHA256: fmt.Sprintf("%x", sha256.Sum256(asset))}
	d.ArtifactStructures, e = artifactStructures(run, b.Admission)
	if e != nil {
		return Dataset{}, e
	}
	d.LatencyPolicy = analysis.HeadlineLatencyPolicy(b.Manifest)
	d.BuilderArchiveVersion = ReportBuilderVersion
	d.Throughput, d.ThroughputVersion = analysis.Throughput(b), analysis.ThroughputVersion
	d.SampleExportVersion = SampleExportVersion
	d.EngineTraceExportVersion = EngineTraceExportVersion
	d.CodeLifetimeExportVersion = CodeLifetimeExportVersion
	d.CounterDisplay = CounterDisplay(b)
	d.CPUStacks = analysis.CPUStacks(b)
	d.CounterDisplayVersion = CounterDisplayVersion
	d.Scaling = analysis.Scaling(b)
	d.ScalingVersion = analysis.ScalingVersion
	d.SnapshotDensityCurves, e = analysis.SnapshotDensity(b)
	if e != nil {
		return Dataset{}, e
	}
	if len(d.SnapshotDensityCurves) > 0 {
		d.SnapshotDensityAnalysisVersion = analysis.SnapshotDensityAnalysisVersion
	}
	d.SnapshotDensityViews, e = snapshotDensityViews(b)
	if e != nil {
		return Dataset{}, e
	}
	if len(d.SnapshotDensityViews) > 0 {
		d.SnapshotDensityViewVersion = SnapshotDensityViewVersion
	}
	d.MemoryTimelines = analysis.MemoryTimelines(b)
	d.SnapshotMemoryViews, e = snapshotMemoryViews(b, "raw", nil)
	if e != nil {
		return Dataset{}, e
	}
	if len(d.SnapshotMemoryViews) > 0 {
		d.SnapshotMemoryViewVersion = SnapshotMemoryViewVersion
	}
	d.MemoryTimelineVersion = analysis.MemoryTimelineVersion
	d.SustainedSessions = analysis.SustainedSessions(b)
	if len(d.SustainedSessions) > 0 {
		d.SustainedVersion = analysis.SustainedVersion
	}
	if memoryRun != "" {
		memory, err := experiment.Load(memoryRun)
		if err != nil {
			return Dataset{}, fmt.Errorf("memory run: %w", err)
		}
		matched, err := matchingMemoryCells(b, memory)
		if err != nil {
			return Dataset{}, err
		}
		d.MemoryStages = memoryStages(memory, matched)
		snapshotViews, err := snapshotMemoryViews(memory, "raw-memory", matched)
		if err != nil {
			return Dataset{}, err
		}
		d.SnapshotMemoryViews = append(d.SnapshotMemoryViews, snapshotViews...)
		if len(snapshotViews) > 0 {
			d.SnapshotMemoryViewVersion = SnapshotMemoryViewVersion
		}
		d.MemoryScaling, d.MemoryScalingTrials = pairedMemoryScaling(memory, matched)
		d.PairedMemoryTimelines, d.PairedDensityFootprints = pairedMemoryTimelines(memory, matched)
		d.MemoryEvidenceVersion = MemoryEvidenceVersion
		for _, session := range analysis.SustainedSessions(memory) {
			if matched[session.Runtime+"\x00"+session.Workload] {
				d.SustainedSessions = append(d.SustainedSessions, session)
				d.SustainedVersion = analysis.SustainedVersion
			}
		}
		d.PhaseCPU = phaseCPUStages(memory, matched)
		d.MemorySource = &MemorySource{ID: memory.Manifest.ID, Profile: "memory", Note: "Separate memory-profile run; joining requires matching host identity, resource policy, protocol, effective runtime configuration, runtime binaries, and workload contracts. Stage memory values are not timing-pass measurements."}
	} else if b.Manifest.Lock.Options.Profile == "memory" {
		d.PhaseCPU = phaseCPUStages(b, nil)
	}
	if codeRun != "" {
		code, err := experiment.Load(codeRun)
		if err != nil {
			return Dataset{}, fmt.Errorf("code run: %w", err)
		}
		matched, err := matchingPassCells(b, code, "code")
		if err != nil {
			return Dataset{}, err
		}
		d.CodeRecords = pairedCodeRecords(code, matched)
		d.CodeSource = &CodeSource{ID: code.Manifest.ID, Profile: "code", Note: "Separate code-profile pass joined only on matching host, resource/protocol policy, runtime binary/configuration and complete workload contract. Raw native images may contain wrappers and data; they are not guest instruction counts."}
	}
	d.DensityFootprints = analysis.DensityFootprints(d.MemoryTimelines)
	d.Pareto = paretoPoints(b, d.Summaries, d.MemoryStages)
	d.ParetoVersion = ParetoVersion
	d.DensityFootprintVersion = analysis.DensityFootprintVersion
	if result, err := analysis.BreakEven(b, nil, "", ""); err == nil {
		d.BreakEven = &result
	}
	return d, nil
}

func renderReportHTML(d Dataset) ([]byte, error) {
	t, err := template.ParseFS(assets, "report.html")
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(d)
	if err != nil {
		return nil, err
	}
	var output bytes.Buffer
	// encoding/json escapes HTML-sensitive characters, including </script>.
	if err := t.Execute(&output, struct{ Data template.JS }{template.JS(data)}); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

// VerifyReport checks the report seal, its copied input bundles, and the
// versioned analysis that was embedded in the page's dataset.
func VerifyReport(root string) error {
	if err := experiment.Verify(root); err != nil {
		return err
	}
	var recorded Dataset
	if err := experiment.ReadJSON(filepath.Join(root, "data.json"), &recorded); err != nil {
		return err
	}
	memory := ""
	if recorded.MemorySource != nil {
		memory = filepath.Join(root, "raw-memory")
	} else if _, err := os.Stat(filepath.Join(root, "raw-memory")); err == nil {
		return fmt.Errorf("unrecorded memory bundle in report")
	} else if !os.IsNotExist(err) {
		return err
	}
	code := ""
	if recorded.CodeSource != nil {
		if err := VerifyNativeCode(filepath.Join(root, "code")); err != nil {
			return err
		}
		code = filepath.Join(root, "code", "raw")
	} else if _, err := os.Stat(filepath.Join(root, "code")); err == nil {
		return fmt.Errorf("unrecorded code bundle in report")
	} else if !os.IsNotExist(err) {
		return err
	}
	recomputed, err := buildReportDataset(filepath.Join(root, "raw"), memory, code)
	if err != nil {
		return err
	}
	// Recorded-key verification establishes archive consistency, not reader trust.
	recomputed.Publication = recorded.Publication
	if err := checkPublicationFiles(root, &recomputed); err != nil {
		return err
	}
	a, err := json.Marshal(recorded)
	if err != nil {
		return err
	}
	c, err := json.Marshal(recomputed)
	if err != nil {
		return err
	}
	if !bytes.Equal(a, c) {
		if recorded.BuilderArchiveVersion != "" && (recorded.RendererSHA256 != recomputed.RendererSHA256 || recorded.AnalysisVersion != recomputed.AnalysisVersion) {
			return fmt.Errorf("report dataset differs from current analysis/renderer; for a trusted report archive, explicitly use verify-report --recorded-builder to execute its original builder")
		}
		return fmt.Errorf("report dataset differs from copied raw evidence and current analysis")
	}
	html, err := renderReportHTML(recomputed)
	if err != nil {
		return err
	}
	recordedHTML, err := os.ReadFile(filepath.Join(root, "index.html"))
	if err != nil {
		return err
	}
	if !bytes.Equal(recordedHTML, html) {
		return fmt.Errorf("report page differs from its dataset and pinned renderer")
	}
	if _, err := verifyReportBuilder(root, recorded); err != nil {
		return err
	}
	if err := verifyEngineTraceExports(recomputed.Bundle, root); err != nil {
		return err
	}
	if err := verifyEngineTraceParquet(recomputed.Bundle, root); err != nil {
		return err
	}
	if err := verifyCodeLifetimeParquet(recomputed.Bundle, root); err != nil {
		return err
	}
	if memory != "" {
		b, err := experiment.Load(memory)
		if err != nil {
			return err
		}
		if err := verifyMemoryExports(root, b); err != nil {
			return err
		}
	}
	return nil
}
