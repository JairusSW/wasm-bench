package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"

	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/analysis"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/storage"
)

func pilotPlan(args []string) error {
	f := flags("pilot-plan")
	source := f.String("run", "", "sealed timing pilot bundle")
	out := f.String("out", "", "new JSON plan file (stdout if omitted)")
	target := f.Float64("relative-half-width", 0.05, "target relative CI half-width for launch medians; planning heuristic only")
	minimum := f.Int("min-launches", 6, "minimum fixed confirmation launches per cell")
	maximum := f.Int("max-launches", 100, "maximum acceptable fixed budget; exceeded estimates remain unresolved")
	if err := f.Parse(args); err != nil {
		return err
	}
	if *source == "" || f.NArg() != 0 {
		return fmt.Errorf("pilot-plan requires --run and no positional arguments")
	}
	b, err := experiment.Load(*source)
	if err != nil {
		return err
	}
	p, err := analysis.PlanPilot(b, *target, *minimum, *maximum)
	if err != nil {
		return err
	}
	p.SourceBundle, err = filepath.Abs(*source)
	if err != nil {
		return err
	}
	p.SourceChecksumsSHA256, err = experiment.DigestFile(filepath.Join(*source, "checksums.json"))
	if err != nil {
		return err
	}
	if *out != "" {
		return experiment.WriteJSON(*out, p)
	}
	return output(p)
}

func loadPilotDecision(path string) (analysis.PilotPlan, experiment.Bundle, error) {
	var p analysis.PilotPlan
	file, err := os.Open(path)
	if err != nil {
		return p, experiment.Bundle{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return p, experiment.Bundle{}, err
	}
	if info.Size() > 16<<20 {
		return p, experiment.Bundle{}, fmt.Errorf("pilot plan exceeds 16 MiB")
	}
	decoder := json.NewDecoder(io.LimitReader(file, 16<<20+1))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&p); err != nil {
		return p, experiment.Bundle{}, err
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return p, experiment.Bundle{}, fmt.Errorf("pilot plan must contain one JSON value")
	}
	if !filepath.IsAbs(p.SourceBundle) {
		return p, experiment.Bundle{}, fmt.Errorf("pilot source must be an absolute bundle path")
	}
	b, err := experiment.Load(p.SourceBundle)
	if err != nil {
		return p, b, err
	}
	digest, err := experiment.DigestFile(filepath.Join(p.SourceBundle, "checksums.json"))
	if err != nil {
		return p, b, err
	}
	if err := analysis.VerifyPilotDecision(p, b, digest); err != nil {
		return p, b, err
	}
	return p, b, nil
}

func pilotRun(ctx context.Context, args []string) error {
	f := flags("pilot-run")
	plan := f.String("plan", "", "fixed pilot decision JSON")
	out := f.String("out", "", "new confirmation bundle directory")
	if err := f.Parse(args); err != nil {
		return err
	}
	if *plan == "" || *out == "" || f.NArg() != 0 {
		return fmt.Errorf("pilot-run requires --plan and --out; no budget overrides")
	}
	p, b, err := loadPilotDecision(*plan)
	if err != nil {
		return err
	}
	host := agent.IdentifyHost()
	source := b.Manifest.Host
	if host.OS != source.OS || host.Arch != source.Arch || host.CPU != source.CPU || host.Kernel != source.Kernel || host.PageSize != source.PageSize || host.Hostname != source.Hostname || !reflect.DeepEqual(host.Environment, source.Environment) {
		return fmt.Errorf("pilot host/environment changed; collect a new pilot on the intended host")
	}
	lock := b.Manifest.Lock
	lock.Options.Launches = p.Launches
	lock.PilotPlan, err = json.Marshal(p)
	if err != nil {
		return err
	}
	path, err := experiment.Run(ctx, lock, p.SourceBundle, *out, func(s string) { fmt.Fprintln(os.Stderr, s) })
	if err != nil {
		return err
	}
	fmt.Println(path)
	result, err := experiment.Load(path)
	if err != nil {
		return err
	}
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	db, err := storage.Open(filepath.Join(root, ".wasmbench", "index.sqlite"))
	if err != nil {
		return err
	}
	err = db.AddRun(path)
	db.Close()
	if err != nil {
		return fmt.Errorf("confirmation saved but indexing failed: %w", err)
	}
	for _, t := range result.Trials {
		if t.Status != "ok" {
			return fmt.Errorf("confirmation contains %s outcomes; fixed budget evidence retained in %s", t.Status, path)
		}
	}
	return nil
}
