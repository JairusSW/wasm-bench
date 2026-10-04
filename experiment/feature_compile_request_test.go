package experiment

import (
 "testing"
 "github.com/wasmbench/wasmbench/protocol"
)

func TestFeatureComponentCompilePreflightPreservesCompileScenario(t *testing.T) {
 w := protocol.Workload{ABI:"component", Oracle:protocol.Oracle{Kind:"component_compile_only"}}
 req := trialRequest(Options{Samples:3,Operations:7,Warmup:9,PhaseBarriers:true},w,"compile",-1)
 if req.Scenario!="compile" || req.Samples!=1 || req.Operations!=1 || req.Warmup!=0 || req.PhaseBarriers {t.Fatalf("invalid compile-only preflight: %+v",req)}
 w.ABI="core";w.Oracle.Kind="exact_u64"
 if req=trialRequest(Options{},w,"compile",-1);req.Scenario!="first-call" {t.Fatalf("scalar oracle preflight changed: %+v",req)}
}
