package main

import (
	"fmt"
	"github.com/wasmbench/wasmbench/protocol"
)

func (a *adapter) registerHostImports() error {
	switch a.prep.Workload.HostProfile {
	case "":
		return nil
	case "identity-v1":
		_, err := a.engine.NewHostModuleBuilder("wasmbench").NewFunctionBuilder().WithFunc(func(v uint32) uint32 { return v }).Export("identity").Instantiate(ctx)
		return err
	case protocol.AssemblyScriptAbortProfile:
		_, err := a.engine.NewHostModuleBuilder("env").NewFunctionBuilder().WithFunc(func(message, file, line, column uint32) {
			panic(protocol.AssemblyScriptAbort(message, file, line, column))
		}).Export("abort").Instantiate(ctx)
		return err
	default:
		return fmt.Errorf("unsupported host profile")
	}
}
