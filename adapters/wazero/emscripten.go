package main

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
)

const emscriptenErrnoNosys = -52

type emscriptenHost struct {
	assertion string
}

func instantiateCommandHost(ctx context.Context, runtime wazero.Runtime, abi string, emscripten *emscriptenHost) error {
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, runtime); err != nil {
		return err
	}
	if abi == "emscripten" {
		return instantiateEmscriptenHost(ctx, runtime, emscripten)
	}
	return nil
}

// This host profile is deliberately stdio-only. Filesystem syscall imports are
// fail-closed; corpus files can enter through pinned stdin, never ambient paths.
func instantiateEmscriptenHost(ctx context.Context, runtime wazero.Runtime, host *emscriptenHost) error {
	b := runtime.NewHostModuleBuilder("env")
	b.NewFunctionBuilder().WithFunc(func(ctx context.Context, module api.Module, code uint32) {
		_ = module.CloseWithExitCode(ctx, code)
		panic(sys.NewExitError(code))
	}).Export("exit")
	b.NewFunctionBuilder().WithFunc(func() { panic(fmt.Errorf("Emscripten abort")) }).Export("abort")
	b.NewFunctionBuilder().WithFunc(func(_ context.Context, module api.Module, assertion, file, line, _ uint32) {
		if host != nil {
			host.assertion = fmt.Sprintf("Emscripten assertion failed: %q at %s:%d", emscriptenCString(module, assertion), emscriptenCString(module, file), line)
		}
	}).Export("__assert_fail")
	b.NewFunctionBuilder().WithFunc(func(uint32) uint32 { return 0 }).Export("emscripten_resize_heap")
	b.NewFunctionBuilder().WithFunc(func(_ context.Context, module api.Module, dst, src, n uint32) uint32 {
		mem, ok := module.Memory().Read(0, module.Memory().Size())
		if !ok || !validEmscriptenRange(mem, dst, n) || !validEmscriptenRange(mem, src, n) {
			panic(fmt.Errorf("Emscripten memcpy out of bounds"))
		}
		copy(mem[dst:dst+n], mem[src:src+n])
		return dst
	}).Export("emscripten_memcpy_big")
	b.NewFunctionBuilder().WithFunc(func(uint32) {}).Export("setTempRet0")
	for _, name := range []string{"__sys_open", "__sys_fcntl64", "__sys_ioctl"} {
		b.NewFunctionBuilder().WithFunc(func(uint32, uint32, uint32) int32 { return emscriptenErrnoNosys }).Export(name)
	}
	for _, name := range []string{"__sys_chmod", "__sys_fchmod", "__sys_lstat64", "__sys_stat64", "__sys_rename", "__clock_gettime"} {
		b.NewFunctionBuilder().WithFunc(func(uint32, uint32) int32 { return emscriptenErrnoNosys }).Export(name)
	}
	b.NewFunctionBuilder().WithFunc(func(_ context.Context, module api.Module, fd, ptr uint32) int32 {
		mem, ok := module.Memory().Read(0, module.Memory().Size())
		if !ok {
			return emscriptenErrnoNosys
		}
		return emscriptenFstat64(mem, fd, ptr)
	}).Export("__sys_fstat64")
	for _, name := range []string{"__sys_readlink", "__sys_fchown32", "__sys_chown32"} {
		b.NewFunctionBuilder().WithFunc(func(uint32, uint32, uint32) int32 { return emscriptenErrnoNosys }).Export(name)
	}
	b.NewFunctionBuilder().WithFunc(func(uint32) int32 { return emscriptenErrnoNosys }).Export("__sys_umask")
	b.NewFunctionBuilder().WithFunc(func(uint32) int32 { return emscriptenErrnoNosys }).Export("__sys_unlink")
	b.NewFunctionBuilder().WithFunc(func(uint32, uint32, uint32, uint32, uint32) uint32 { return 70 }).Export("emscripten_fd_seek")
	b.NewFunctionBuilder().WithFunc(func(uint32, uint32, uint32, uint32, uint32, uint32) int32 { return emscriptenErrnoNosys }).Export("splice")
	for _, name := range []string{"__sys_fstatat64", "__sys_openat", "__sys_prlimit64"} {
		b.NewFunctionBuilder().WithFunc(func(uint32, uint32, uint32, uint32) int32 { return emscriptenErrnoNosys }).Export(name)
	}
	b.NewFunctionBuilder().WithFunc(func(uint32, uint32, uint32, uint32, uint32, uint32, uint32) uint32 { return 0 }).Export("__sys_fadvise64_64")
	for _, name := range []string{"__sys_fchdir", "__sys_chdir"} {
		b.NewFunctionBuilder().WithFunc(func(uint32) int32 { return emscriptenErrnoNosys }).Export(name)
	}
	b.NewFunctionBuilder().WithFunc(func(uint32, uint32) int32 { return emscriptenErrnoNosys }).Export("__sys_ugetrlimit")
	b.NewFunctionBuilder().WithFunc(func(_ context.Context, module api.Module) uint32 { return module.Memory().Size() }).Export("emscripten_get_heap_max")
	b.NewFunctionBuilder().WithFunc(func(uint32, uint32, uint32) int32 { return emscriptenErrnoNosys }).Export("__sys_getdents64")
	b.NewFunctionBuilder().WithFunc(func(uint32) uint32 { return 1 }).Export("time")
	b.NewFunctionBuilder().WithFunc(func() uint32 { return 0 }).Export("clock")
	b.NewFunctionBuilder().WithFunc(func(a, b uint32) float64 { return float64(int32(a) - int32(b)) }).Export("difftime")
	b.NewFunctionBuilder().WithFunc(func(_ context.Context, module api.Module, _ uint32, ptr uint32) uint32 {
		if !module.Memory().Write(ptr, make([]byte, 44)) {
			panic(fmt.Errorf("Emscripten localtime_r out of bounds"))
		}
		return ptr
	}).Export("localtime_r")
	b.NewFunctionBuilder().WithFunc(func(uint32, uint32, uint32, uint32) uint32 { return 0 }).Export("strftime")
	b.NewFunctionBuilder().WithFunc(func(_ context.Context, module api.Module, ptr, _ uint32) uint32 {
		if ptr != 0 && !module.Memory().Write(ptr, make([]byte, 8)) {
			panic(fmt.Errorf("Emscripten gettimeofday out of bounds"))
		}
		return 0
	}).Export("gettimeofday")
	_, err := b.Instantiate(ctx)
	return err
}

func validEmscriptenRange(mem []byte, offset, size uint32) bool {
	return uint64(offset)+uint64(size) <= uint64(len(mem))
}

func emscriptenCString(module api.Module, ptr uint32) string {
	if module.Memory() == nil {
		return "<no memory>"
	}
	for size := uint32(0); size < 4096; size++ {
		addr := uint64(ptr) + uint64(size)
		if addr > 0xffffffff {
			return "<out of bounds>"
		}
		b, ok := module.Memory().Read(uint32(addr), 1)
		if !ok {
			return "<out of bounds>"
		}
		if b[0] == 0 {
			data, _ := module.Memory().Read(ptr, size)
			return string(data)
		}
	}
	return "<unterminated>"
}

func emscriptenFstat64(mem []byte, fd, ptr uint32) int32 {
	if fd > 2 || !validEmscriptenRange(mem, ptr, 88) {
		return emscriptenErrnoNosys
	}
	clear(mem[ptr : ptr+88])
	binary.LittleEndian.PutUint32(mem[ptr+8:], fd+1)
	binary.LittleEndian.PutUint32(mem[ptr+12:], 0x2000|0666)
	binary.LittleEndian.PutUint32(mem[ptr+16:], 1)
	binary.LittleEndian.PutUint32(mem[ptr+48:], 4096)
	binary.LittleEndian.PutUint32(mem[ptr+80:], fd+1)
	return 0
}

func prepareEmscriptenMain(ctx context.Context, module api.Module, args []string) (api.Function, uint32, error) {
	ctors := module.ExportedFunction("__wasm_call_ctors")
	stackAlloc := module.ExportedFunction("stackAlloc")
	main := module.ExportedFunction("main")
	if ctors == nil || stackAlloc == nil || main == nil || len(main.Definition().ParamTypes()) != 2 || len(main.Definition().ResultTypes()) > 1 {
		return nil, 0, fmt.Errorf("unsupported Emscripten lifecycle exports")
	}
	if _, err := ctors.Call(ctx); err != nil {
		return nil, 0, fmt.Errorf("Emscripten constructors: %w", err)
	}
	ptrs := make([]uint32, len(args))
	for i, arg := range args {
		allocated, err := stackAlloc.Call(ctx, uint64(len(arg)+1))
		if err != nil || len(allocated) != 1 || allocated[0] > 0xffffffff {
			return nil, 0, fmt.Errorf("Emscripten stackAlloc argument %d: %w", i, err)
		}
		ptrs[i] = uint32(allocated[0])
		if !module.Memory().Write(ptrs[i], append([]byte(arg), 0)) {
			return nil, 0, fmt.Errorf("Emscripten argument %d allocation out of bounds", i)
		}
	}
	allocated, err := stackAlloc.Call(ctx, uint64((len(args)+1)*4))
	if err != nil || len(allocated) != 1 || allocated[0] > 0xffffffff {
		return nil, 0, fmt.Errorf("Emscripten stackAlloc argv: %w", err)
	}
	argv := uint32(allocated[0])
	argvBytes := make([]byte, (len(args)+1)*4)
	for i, ptr := range ptrs {
		binary.LittleEndian.PutUint32(argvBytes[i*4:], ptr)
	}
	if !module.Memory().Write(argv, argvBytes) {
		return nil, 0, fmt.Errorf("Emscripten argv allocation out of bounds")
	}
	return main, argv, nil
}
