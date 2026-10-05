//! Adapter-only finite stdin and bounded stdout/stderr for the public WASI builder.
use super::wasi_config_t;
use std::{
    io,
    pin::Pin,
    sync::{Arc, Mutex},
    task::{Context, Poll},
};
use tokio::io::{AsyncRead, AsyncSeek, AsyncWrite, ReadBuf};
use wasmer_wasix::virtual_fs::{FsError, VirtualFile};

#[derive(Debug)]
struct State {
    bytes: Vec<u8>,
    position: usize,
    limit: usize,
    input: bool,
    overflow: bool,
}
#[derive(Debug, Clone)]
struct Stream(Arc<Mutex<State>>);
impl Stream {
    fn new(bytes: Vec<u8>, limit: usize, input: bool) -> Self {
        Self(Arc::new(Mutex::new(State {
            bytes,
            position: 0,
            limit,
            input,
            overflow: false,
        })))
    }
}
impl AsyncRead for Stream {
    fn poll_read(
        self: Pin<&mut Self>,
        _: &mut Context<'_>,
        out: &mut ReadBuf<'_>,
    ) -> Poll<io::Result<()>> {
        let mut s = self.0.lock().unwrap();
        if !s.input {
            return Poll::Ready(Err(io::ErrorKind::PermissionDenied.into()));
        }
        let n = out
            .remaining()
            .min(s.bytes.len().saturating_sub(s.position));
        out.put_slice(&s.bytes[s.position..s.position + n]);
        s.position += n;
        Poll::Ready(Ok(()))
    }
}
impl AsyncWrite for Stream {
    fn poll_write(
        self: Pin<&mut Self>,
        _: &mut Context<'_>,
        bytes: &[u8],
    ) -> Poll<io::Result<usize>> {
        let mut s = self.0.lock().unwrap();
        if s.input {
            return Poll::Ready(Err(io::ErrorKind::PermissionDenied.into()));
        }
        if bytes.len() > s.limit.saturating_sub(s.bytes.len()) {
            s.overflow = true;
            return Poll::Ready(Err(io::ErrorKind::WriteZero.into()));
        }
        s.bytes.extend_from_slice(bytes);
        Poll::Ready(Ok(bytes.len()))
    }
    fn poll_flush(self: Pin<&mut Self>, _: &mut Context<'_>) -> Poll<io::Result<()>> {
        Poll::Ready(Ok(()))
    }
    fn poll_shutdown(self: Pin<&mut Self>, _: &mut Context<'_>) -> Poll<io::Result<()>> {
        Poll::Ready(Ok(()))
    }
}
impl AsyncSeek for Stream {
    fn start_seek(self: Pin<&mut Self>, _: io::SeekFrom) -> io::Result<()> {
        Err(io::ErrorKind::Unsupported.into())
    }
    fn poll_complete(self: Pin<&mut Self>, _: &mut Context<'_>) -> Poll<io::Result<u64>> {
        Poll::Ready(Err(io::ErrorKind::Unsupported.into()))
    }
}
impl VirtualFile for Stream {
    fn last_accessed(&self) -> u64 {
        0
    }
    fn last_modified(&self) -> u64 {
        0
    }
    fn created_time(&self) -> u64 {
        0
    }
    fn size(&self) -> u64 {
        self.0.lock().unwrap().bytes.len() as u64
    }
    fn set_len(&mut self, _: u64) -> Result<(), FsError> {
        Err(FsError::PermissionDenied)
    }
    fn unlink(&mut self) -> Result<(), FsError> {
        Err(FsError::PermissionDenied)
    }
    fn poll_read_ready(self: Pin<&mut Self>, _: &mut Context<'_>) -> Poll<io::Result<usize>> {
        let s = self.0.lock().unwrap();
        Poll::Ready(if s.input {
            Ok(s.bytes.len().saturating_sub(s.position))
        } else {
            Err(io::ErrorKind::PermissionDenied.into())
        })
    }
    fn poll_write_ready(self: Pin<&mut Self>, _: &mut Context<'_>) -> Poll<io::Result<usize>> {
        let s = self.0.lock().unwrap();
        Poll::Ready(if s.input {
            Err(io::ErrorKind::PermissionDenied.into())
        } else {
            Ok(s.limit.saturating_sub(s.bytes.len()))
        })
    }
}

pub struct Streams {
    stdout: Stream,
    stderr: Stream,
}
fn guarded<T>(fallback: T, f: impl FnOnce() -> T) -> T {
    std::panic::catch_unwind(std::panic::AssertUnwindSafe(f)).unwrap_or(fallback)
}
/// Construct the SDK's public WASI builder with a single runtime worker.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn wasmbench_wasi_config_new(
    program: *const std::ffi::c_char,
) -> Option<Box<wasi_config_t>> {
    guarded(None, || {
        if program.is_null() { return None; }
        let name = unsafe { std::ffi::CStr::from_ptr(program) }.to_str().ok()?;
        let runtime = tokio::runtime::Builder::new_multi_thread()
            .worker_threads(1)
            .max_blocking_threads(1)
            .enable_all()
            .build().ok()?;
        let builder = {
            let _guard = runtime.enter();
            wasmer_wasix::WasiEnv::builder(name)
                .fs(wasmer_wasix::default_fs_backing())
                .setup_fs(Box::new(|_, fs| {
                    // The preopen builder trims '/', but the root resolver
                    // follows the '/' key. Normalize our single fixture mount.
                    // The SDK virtual root initially delegates all rights.
                    // Limit it to the capabilities of our read-only preopen.
                    // Follow the SDK lock order: descriptor map before inode.
                    let preopens = fs.preopen_fds.read().map_err(|_| "WASI preopen lock poisoned")?;
                    let fixture_fds: Vec<_> = preopens.iter().copied().filter(|fd| *fd != 3).collect();
                    if fixture_fds.len() != 1 { return Err("WASI fixture requires exactly one preopen".into()); }
                    let mut descriptors = fs.fd_map.write().map_err(|_| "WASI descriptor lock poisoned")?;
                    let fixture = descriptors.get(fixture_fds[0]).ok_or("WASI fixture descriptor missing")?;
                    let rights = fixture.inner.rights;
                    let inheriting = fixture.inner.rights_inheriting;
                    let virtual_root = descriptors.get_mut(3).ok_or("WASI virtual root descriptor missing")?;
                    virtual_root.rights &= rights;
                    virtual_root.rights_inheriting &= inheriting;
                    let mut root = fs.root_inode.write();
                    let wasmer_wasix::fs::Kind::Root { entries } = &mut *root else {
                        return Err("WASI fixture root is not a directory root".into());
                    };
                    if entries.contains_key("/") {
                        return Err("WASI fixture root alias already exists".into());
                    }
                    let fixture = entries.remove("").ok_or("WASI fixture root alias missing")?;
                    entries.insert("/".into(), fixture);
                    Ok(())
                }))
        };
        Some(Box::new(wasi_config_t {
            inherit_stdout: true,
            inherit_stderr: true,
            inherit_stdin: true,
            builder,
            runtime: Some(runtime),
        }))
    })
}

/// Caller owns the returned handle; the builder owns separate Arc-backed streams.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn wasmbench_wasi_config_stdio(
    config: Option<&mut wasi_config_t>,
    input: *const u8,
    len: usize,
    limit: usize,
) -> *mut Streams {
    guarded(std::ptr::null_mut(), || {
        let Some(config) = config else {
            return std::ptr::null_mut();
        };
        if len > 256 * 1024 * 1024
            || (len > 0 && input.is_null())
            || limit == 0
            || limit > 64 * 1024 * 1024
        {
            return std::ptr::null_mut();
        }
        let bytes = if len == 0 {
            Vec::new()
        } else {
            unsafe { std::slice::from_raw_parts(input, len) }.to_vec()
        };
        let stdout = Stream::new(Vec::new(), limit, false);
        let stderr = Stream::new(Vec::new(), limit, false);
        config
            .builder
            .set_stdin(Box::new(Stream::new(bytes, len, true)));
        config.builder.set_stdout(Box::new(stdout.clone()));
        config.builder.set_stderr(Box::new(stderr.clone()));
        // wasi_env_new must retain the explicitly supplied files, not replace them with Pipe.
        config.inherit_stdout = true;
        config.inherit_stderr = true;
        Box::into_raw(Box::new(Streams { stdout, stderr }))
    })
}
#[unsafe(no_mangle)]
pub unsafe extern "C" fn wasmbench_wasi_stdio_delete(streams: *mut Streams) {
    if !streams.is_null() {
        drop(unsafe { Box::from_raw(streams) })
    }
}
#[unsafe(no_mangle)]
pub unsafe extern "C" fn wasmbench_wasi_stdio_overflow(streams: Option<&Streams>) -> bool {
    guarded(true, || {
        streams.is_none_or(|s| {
            s.stdout.0.lock().unwrap().overflow || s.stderr.0.lock().unwrap().overflow
        })
    })
}
/// Null buffer with zero capacity queries length. A short/invalid buffer returns usize::MAX.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn wasmbench_wasi_stdio_copy(
    streams: Option<&Streams>,
    fd: u32,
    out: *mut u8,
    capacity: usize,
) -> usize {
    guarded(usize::MAX, || {
        let Some(streams) = streams else {
            return usize::MAX;
        };
        let stream = match fd {
            1 => &streams.stdout,
            2 => &streams.stderr,
            _ => return usize::MAX,
        };
        let s = stream.0.lock().unwrap();
        if out.is_null() && capacity == 0 {
            return s.bytes.len();
        }
        if out.is_null() || capacity < s.bytes.len() {
            return usize::MAX;
        }
        unsafe { std::ptr::copy_nonoverlapping(s.bytes.as_ptr(), out, s.bytes.len()) };
        s.bytes.len()
    })
}
#[unsafe(no_mangle)]
pub unsafe extern "C" fn wasmbench_wasi_config_readonly_dir(
    config: Option<&mut wasi_config_t>,
    dir: *const std::ffi::c_char,
) -> bool {
    guarded(false, || {
        let Some(config) = config else { return false };
        if dir.is_null() {
            return false;
        }
        let Ok(path) = unsafe { std::ffi::CStr::from_ptr(dir) }.to_str() else {
            return false;
        };
        config
            .builder
            .add_preopen_build(|p| {
                p.directory(path)
                    .alias("/")
                    .read(true)
                    .write(false)
                    .create(false)
            })
            .is_ok()
    })
}

/// Initialize through the public WASI API without its C wrapper's unwrap.
/// A failed initialization is reported to the embedding rather than aborting it.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn wasmbench_wasi_initialize_instance(
    env: Option<&mut super::wasi_env_t>,
    store: Option<&mut super::super::store::wasm_store_t>,
    instance: Option<&super::super::instance::wasm_instance_t>,
) -> bool {
    guarded(false, || {
        let (Some(env), Some(store), Some(instance)) = (env, store, instance) else {
            return false;
        };
        let mut store = unsafe { store.inner.store_mut() };
        match env.inner.initialize(&mut store, instance.inner.clone()) {
            Ok(()) => true,
            Err(error) => {
                crate::error::update_last_error(error);
                false
            }
        }
    })
}

/// Preserve typed proc_exit outcomes; ordinary guest traps remain errors.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn wasmbench_wasi_trap_exit_code(
    trap: Option<&super::super::trap::wasm_trap_t>,
    code: *mut u32,
) -> bool {
    guarded(false, || {
        if code.is_null() { return false; }
        let Some(trap) = trap else { return false; };
        let Some(wasmer_wasix::WasiError::Exit(exit)) = trap.inner.downcast_ref::<wasmer_wasix::WasiError>() else {
            return false;
        };
        unsafe { *code = i32::from(*exit) as u32; }
        true
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn config_uses_one_runtime_worker_and_rejects_null_names() {
        assert!(unsafe { wasmbench_wasi_config_new(std::ptr::null()) }.is_none());
        let name = std::ffi::CString::new("wasmbench").unwrap();
        let config = unsafe { wasmbench_wasi_config_new(name.as_ptr()) }.unwrap();
        assert_eq!(config.runtime.as_ref().unwrap().metrics().num_workers(), 1);
    }
    #[test]
    fn typed_exit_is_distinct_from_an_ordinary_trap() {
        let trap = super::super::super::trap::wasm_trap_t {
            inner: wasmer_api::RuntimeError::user(Box::new(wasmer_wasix::WasiError::Exit(7.into()))),
        };
        let mut code = 99;
        assert!(unsafe { wasmbench_wasi_trap_exit_code(Some(&trap), &mut code) });
        assert_eq!(code, 7);
        let ordinary = super::super::super::trap::wasm_trap_t {
            inner: wasmer_api::RuntimeError::new("guest trap"),
        };
        assert!(!unsafe { wasmbench_wasi_trap_exit_code(Some(&ordinary), &mut code) });
        assert_eq!(code, 7);
        assert!(!unsafe { wasmbench_wasi_trap_exit_code(Some(&trap), std::ptr::null_mut()) });
    }
    #[test]
    fn stdin_is_finite_and_cannot_be_written() {
        let mut file = Stream::new(b"abc".to_vec(), 3, true);
        let mut cx = Context::from_waker(std::task::Waker::noop());
        let mut bytes = [0; 8];
        let mut out = ReadBuf::new(&mut bytes);
        assert!(matches!(
            Pin::new(&mut file).poll_read(&mut cx, &mut out),
            Poll::Ready(Ok(()))
        ));
        assert_eq!(out.filled(), b"abc");
        let mut eof = ReadBuf::new(&mut bytes);
        assert!(matches!(
            Pin::new(&mut file).poll_read(&mut cx, &mut eof),
            Poll::Ready(Ok(()))
        ));
        assert!(eof.filled().is_empty());
        assert!(matches!(
            Pin::new(&mut file).poll_write(&mut cx, b"x"),
            Poll::Ready(Err(_))
        ));
    }
    #[test]
    fn output_retains_its_bound_and_reports_overflow() {
        let mut stdout = Stream::new(Vec::new(), 3, false);
        let stderr = Stream::new(Vec::new(), 3, false);
        let mut cx = Context::from_waker(std::task::Waker::noop());
        assert!(matches!(
            Pin::new(&mut stdout).poll_write(&mut cx, b"abc"),
            Poll::Ready(Ok(3))
        ));
        assert!(matches!(
            Pin::new(&mut stdout).poll_write(&mut cx, b"d"),
            Poll::Ready(Err(_))
        ));
        let streams = Streams { stdout, stderr };
        assert!(unsafe { wasmbench_wasi_stdio_overflow(Some(&streams)) });
        assert_eq!(
            unsafe { wasmbench_wasi_stdio_copy(Some(&streams), 1, std::ptr::null_mut(), 0) },
            3
        );
        let mut out = [0; 3];
        assert_eq!(
            unsafe { wasmbench_wasi_stdio_copy(Some(&streams), 1, out.as_mut_ptr(), 3) },
            3
        );
        assert_eq!(&out, b"abc");
        assert_eq!(
            unsafe { wasmbench_wasi_stdio_copy(Some(&streams), 1, out.as_mut_ptr(), 2) },
            usize::MAX
        );
    }
}
