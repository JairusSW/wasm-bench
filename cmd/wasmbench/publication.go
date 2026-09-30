package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/wasmbench/wasmbench/analysis"
	"github.com/wasmbench/wasmbench/experiment"
)

func publicationAudit(b experiment.Bundle, pilotPath string) analysis.PublicationAudit {
	return publicationAuditWithQualification(b, pilotPath, analysis.QualificationEvidence{})
}

func publicationAuditWithQualification(b experiment.Bundle, pilotPath string, qualification analysis.QualificationEvidence) analysis.PublicationAudit {
	evidence := analysis.PilotEvidence{}
	p, err := analysis.DecodePilotPlan(b.Manifest.Lock.PilotPlan)
	if err != nil {
		evidence.Error = err.Error()
	} else {
		if pilotPath == "" {
			pilotPath = p.SourceBundle
		}
		if pilotPath == "" {
			evidence.Error = "pilot source path absent"
		} else {
			source, err := experiment.Load(pilotPath)
			if err != nil {
				evidence.Error = err.Error()
			} else {
				digest, err := experiment.DigestFile(filepath.Join(pilotPath, "checksums.json"))
				if err != nil {
					evidence.Error = err.Error()
				} else {
					evidence.Bundle = &source
					evidence.ChecksumsSHA256 = digest
				}
			}
		}
	}
	return analysis.AuditPublicationWithQualification(b, evidence, qualification)
}

func publicationCheck(args []string) error {
	f := flags("publication-check")
	path := f.String("run", "", "sealed confirmation bundle")
	pilot := f.String("pilot-run", "", "relocated sealed pilot bundle (otherwise recorded source path)")
	qualification := f.String("qualification", "", "operator-signed dedicated-host qualification JSON")
	key := f.String("qualification-key", "", "separately trusted operator public key JSON; never trust a key from the envelope alone")
	out := f.String("out", "", "new audit JSON file (stdout if omitted)")
	if err := f.Parse(args); err != nil {
		return err
	}
	if *path == "" || f.NArg() != 0 {
		return fmt.Errorf("publication-check requires --run and no positional arguments")
	}
	if *out != "" {
		inputs := []string{*path}
		if *pilot != "" {
			inputs = append(inputs, *pilot)
		}
		if err := experiment.OutputOutsideBundles(*out, inputs...); err != nil {
			return err
		}
	}
	b, err := experiment.Load(*path)
	if err != nil {
		return err
	}
	if *out != "" && *pilot == "" {
		if p, e := analysis.DecodePilotPlan(b.Manifest.Lock.PilotPlan); e == nil && p.SourceBundle != "" {
			if e := experiment.OutputOutsideBundles(*out, p.SourceBundle); e != nil && !os.IsNotExist(e) {
				return e
			}
		}
	}
	a := publicationAuditWithQualification(b, *pilot, loadQualificationEvidence(*path, *qualification, *key))
	if *out != "" {
		if err := experiment.WriteJSON(*out, a); err != nil {
			return err
		}
	} else {
		if err := output(a); err != nil {
			return err
		}
	}
	return a.Err()
}
