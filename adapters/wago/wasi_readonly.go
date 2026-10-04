package main

import (
	"fmt"
	"github.com/wago-org/wago"
	"github.com/wago-org/wasi/p1"
)

// The pinned provider's directoryReadRights | fileReadRights. Go's WASI
// libc requests extra write/sync/path rights for read-only file opens.
const readonlyP1Rights uint64 = (1 << 1) | (1 << 2) | (1 << 5) | (1 << 7) | (1 << 13) | (1 << 14) | (1 << 15) | (1 << 18) | (1 << 21) | (1 << 27)

func readonlyP1OpenOverrides(imports *wago.Imports) (*wago.Imports, error) {
	value, ok := imports.Lookup(p1.Module, "path_open")
	open, typed := value.(wago.CallerHostCallFunc)
	if !ok || !typed {
		return nil, fmt.Errorf("WASI provider path_open callback unavailable")
	}
	out := wago.NewImports()
	out.HostFunc(p1.Module, "path_open", wago.CallerHostCallFunc(func(caller wago.Caller, call wago.HostCall) {
		args, results := call.ParamSlots(), call.ResultSlots()
		if len(args) != 9 || len(results) != 1 {
			panic("invalid WASI path_open callback shape")
		}
		// CREAT/EXCL/TRUNC and APPEND/DSYNC/RSYNC/SYNC are mutation requests.
		if args[4]&13 != 0 || args[7]&27 != 0 {
			results[0] = 76
			return
		}
		base, inheriting := args[5], args[6]
		args[5] &= readonlyP1Rights
		args[6] &= readonlyP1Rights
		defer func() { args[5] = base; args[6] = inheriting }()
		open(caller, call)
	})).Params(wago.ValI32, wago.ValI32, wago.ValI32, wago.ValI32, wago.ValI32, wago.ValI64, wago.ValI64, wago.ValI32, wago.ValI32).Results(wago.ValI32)
	return out, nil
}
