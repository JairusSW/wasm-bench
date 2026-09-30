use wasmtime::{Caller, Linker, Result, bail, format_err as anyhow};
use wasmtime_wasi::I32Exit;
use wasmtime_wasi::p1::WasiP1Ctx;

const NOSYS: i32 = -52;

fn memory(caller: &mut Caller<'_, WasiP1Ctx>) -> Result<wasmtime::Memory> {
    caller
        .get_export("memory")
        .and_then(|export| export.into_memory())
        .ok_or_else(|| anyhow!("Emscripten memory export missing"))
}

// The profile intentionally has no filesystem access. These imports match the
// pinned Wago command corpus; unsupported syscalls return Emscripten ENOSYS.
pub(super) fn add_to_linker(linker: &mut Linker<WasiP1Ctx>) -> Result<()> {
    linker.func_wrap(
        "env",
        "exit",
        |_: Caller<'_, WasiP1Ctx>, code: i32| -> Result<()> { Err(I32Exit(code).into()) },
    )?;
    linker.func_wrap("env", "abort", || -> Result<()> {
        bail!("Emscripten abort")
    })?;
    linker.func_wrap(
        "env",
        "__assert_fail",
        |_: i32, _: i32, _: i32, _: i32| -> Result<()> { bail!("Emscripten assertion failed") },
    )?;
    linker.func_wrap("env", "emscripten_resize_heap", |_: i32| -> i32 { 0 })?;
    linker.func_wrap(
        "env",
        "emscripten_memcpy_big",
        |mut caller: Caller<'_, WasiP1Ctx>, dst: i32, src: i32, count: i32| -> Result<i32> {
            let mem = memory(&mut caller)?;
            let data = mem.data_mut(&mut caller);
            let (dst, src, count) = (
                dst as u32 as usize,
                src as u32 as usize,
                count as u32 as usize,
            );
            let source = data
                .get(
                    src..src
                        .checked_add(count)
                        .ok_or_else(|| anyhow!("memcpy overflow"))?,
                )
                .ok_or_else(|| anyhow!("memcpy source out of bounds"))?
                .to_vec();
            let target = data
                .get_mut(
                    dst..dst
                        .checked_add(count)
                        .ok_or_else(|| anyhow!("memcpy overflow"))?,
                )
                .ok_or_else(|| anyhow!("memcpy destination out of bounds"))?;
            target.copy_from_slice(&source);
            Ok(dst as i32)
        },
    )?;
    linker.func_wrap("env", "setTempRet0", |_: i32| {})?;
    for name in ["__sys_open", "__sys_fcntl64", "__sys_ioctl"] {
        linker.func_wrap("env", name, |_: i32, _: i32, _: i32| -> i32 { NOSYS })?;
    }
    for name in [
        "__sys_chmod",
        "__sys_fchmod",
        "__sys_lstat64",
        "__sys_stat64",
        "__sys_rename",
        "__clock_gettime",
    ] {
        linker.func_wrap("env", name, |_: i32, _: i32| -> i32 { NOSYS })?;
    }
    linker.func_wrap(
        "env",
        "__sys_fstat64",
        |mut caller: Caller<'_, WasiP1Ctx>, fd: i32, ptr: i32| -> i32 {
            if !(0..=2).contains(&fd) {
                return NOSYS;
            }
            let Ok(mem) = memory(&mut caller) else {
                return NOSYS;
            };
            let Some(end) = (ptr as u32 as usize).checked_add(88) else {
                return NOSYS;
            };
            let Some(buf) = mem.data_mut(&mut caller).get_mut(ptr as u32 as usize..end) else {
                return NOSYS;
            };
            buf.fill(0);
            buf[8..12].copy_from_slice(&(fd as u32 + 1).to_le_bytes());
            buf[12..16].copy_from_slice(&(0x2000u32 | 0o666).to_le_bytes());
            buf[16..20].copy_from_slice(&1u32.to_le_bytes());
            buf[48..52].copy_from_slice(&4096u32.to_le_bytes());
            buf[80..84].copy_from_slice(&(fd as u32 + 1).to_le_bytes());
            0
        },
    )?;
    for name in ["__sys_readlink", "__sys_fchown32", "__sys_chown32"] {
        linker.func_wrap("env", name, |_: i32, _: i32, _: i32| -> i32 { NOSYS })?;
    }
    for name in ["__sys_umask", "__sys_unlink", "__sys_fchdir", "__sys_chdir"] {
        linker.func_wrap("env", name, |_: i32| -> i32 { NOSYS })?;
    }
    linker.func_wrap(
        "env",
        "emscripten_fd_seek",
        |_: i32, _: i32, _: i32, _: i32, _: i32| -> i32 { 70 },
    )?;
    linker.func_wrap(
        "env",
        "splice",
        |_: i32, _: i32, _: i32, _: i32, _: i32, _: i32| -> i32 { NOSYS },
    )?;
    for name in ["__sys_fstatat64", "__sys_openat", "__sys_prlimit64"] {
        linker.func_wrap("env", name, |_: i32, _: i32, _: i32, _: i32| -> i32 {
            NOSYS
        })?;
    }
    linker.func_wrap(
        "env",
        "__sys_fadvise64_64",
        |_: i32, _: i32, _: i32, _: i32, _: i32, _: i32, _: i32| -> i32 { 0 },
    )?;
    linker.func_wrap("env", "__sys_ugetrlimit", |_: i32, _: i32| -> i32 { NOSYS })?;
    linker.func_wrap(
        "env",
        "emscripten_get_heap_max",
        |mut caller: Caller<'_, WasiP1Ctx>| -> Result<i32> {
            let mem = memory(&mut caller)?;
            Ok(mem.data_size(&caller) as i32)
        },
    )?;
    linker.func_wrap("env", "__sys_getdents64", |_: i32, _: i32, _: i32| -> i32 {
        NOSYS
    })?;
    linker.func_wrap("env", "time", |_: i32| -> i32 { 1 })?;
    linker.func_wrap("env", "clock", || -> i32 { 0 })?;
    linker.func_wrap("env", "difftime", |a: i32, b: i32| -> f64 {
        (i64::from(a) - i64::from(b)) as f64
    })?;
    linker.func_wrap(
        "env",
        "localtime_r",
        |mut caller: Caller<'_, WasiP1Ctx>, _: i32, ptr: i32| -> Result<i32> {
            memory(&mut caller)?.write(&mut caller, ptr as u32 as usize, &[0; 44])?;
            Ok(ptr)
        },
    )?;
    linker.func_wrap("env", "strftime", |_: i32, _: i32, _: i32, _: i32| -> i32 {
        0
    })?;
    linker.func_wrap(
        "env",
        "gettimeofday",
        |mut caller: Caller<'_, WasiP1Ctx>, ptr: i32, _: i32| -> Result<i32> {
            if ptr != 0 {
                memory(&mut caller)?.write(&mut caller, ptr as u32 as usize, &[0; 8])?;
            }
            Ok(0)
        },
    )?;
    Ok(())
}
