use super::*;
use base64::Engine as _;
use bytes::Bytes;
use std::{
    io::Read,
    path::{Path, PathBuf},
    pin::Pin,
    sync::{
        Arc, Mutex,
        atomic::{AtomicU64, Ordering},
    },
    task::{Context, Poll},
    time::Duration,
};
use tokio::io::AsyncWrite;
use wasmtime::Linker;
use wasmtime_wasi::cli::{InputFile, IsTerminal, StdoutStream};
use wasmtime_wasi::clocks::{HostMonotonicClock, HostWallClock};
use wasmtime_wasi::p1::{self, WasiP1Ctx};
use wasmtime_wasi::p2::{OutputStream, Pollable, StreamError, StreamResult, pipe::MemoryInputPipe};
use wasmtime_wasi::{DirPerms, FilePerms, I32Exit, WasiCtxBuilder};

const INLINE_LIMIT: usize = 1 << 20;
const FILE_LIMIT: u64 = 256 << 20;

pub(super) fn string(v: &Value) -> &str {
    v.as_str().unwrap_or("")
}
fn digest(b: &[u8]) -> String {
    hex::encode(Sha256::digest(b))
}
fn normalize_stdout(normalizer: &str, bytes: &[u8]) -> Result<Vec<u8>> {
    match normalizer {
        "" => Ok(bytes.to_vec()),
        "llvm-ir-preds" => {
            let needle = b"; preds =";
            let mut normalized = Vec::with_capacity(bytes.len());
            let mut index = 0;
            while index < bytes.len() {
                if bytes[index] == b' ' {
                    let start = index;
                    while index < bytes.len() && bytes[index] == b' ' {
                        index += 1;
                    }
                    if bytes[index..].starts_with(needle) {
                        normalized.push(b' ');
                    } else {
                        normalized.extend_from_slice(&bytes[start..index]);
                    }
                } else {
                    normalized.push(bytes[index]);
                    index += 1;
                }
            }
            Ok(normalized)
        }
        _ => bail!("unsupported stdout normalization"),
    }
}
fn valid_digest(s: &str) -> bool {
    s.len() == 64
        && s.bytes()
            .all(|b| b.is_ascii_digit() || (b'a'..=b'f').contains(&b))
}
pub(super) fn data(v: &Value) -> Result<Vec<u8>> {
    let s = string(v);
    if s.len() > (INLINE_LIMIT + 2) / 3 * 4 {
        bail!("command input byte budget exceeded")
    }
    Ok(base64::engine::general_purpose::STANDARD.decode(s)?)
}
pub(super) fn guest_path(s: &str) -> bool {
    !s.is_empty()
        && !s.contains(['\\', '\0'])
        && s.split('/').all(|p| !p.is_empty() && p != "." && p != "..")
}

pub(super) fn validate(w: &Value) -> Result<()> {
    let c = &w["command"];
    if !["", "llvm-ir-preds"].contains(&string(&c["stdout_normalize"])) {
        bail!("unsupported stdout normalization")
    }
    if !c.is_object()
        || (w["abi"] != "wasi-command" && w["abi"] != "component" && w["abi"] != "emscripten")
        || w["oracle"]["kind"] != "exact_command"
        || w["export"]
            != if w["abi"] == "emscripten" {
                "main"
            } else {
                "_start"
            }
        || w["reset"] != "fresh_instance_per_sample"
        || (if w["abi"] == "emscripten" {
            w["host_profile"] != "emscripten-stdio-v1"
        } else {
            w["host_profile"] != "wasi-preview1-readonly-v1"
                && !(w["abi"] == "component"
                    && [
                        "wasi-preview2-readonly-v1",
                        "wasi-preview2-temporary-filesystem-v1",
                    ]
                    .contains(&w["host_profile"].as_str().unwrap_or("")))
        })
        || !w["input"].is_null()
        || !w["vectors"].is_null()
        || !string(&w["initialize"]).is_empty()
        || !string(&w["oracle"]["output_pointer_export"]).is_empty()
        || [&w["args"], &w["oracle"]["expected"], &w["oracle"]["memory"]]
            .iter()
            .any(|v| !v.is_null() && v.as_array().is_none_or(|a| !a.is_empty()))
    {
        bail!("unsupported or ambiguous command contract")
    }
    let argv = c["argv"]
        .as_array()
        .ok_or_else(|| anyhow!("missing argv"))?;
    let limit = c["output_limit_bytes"].as_u64().unwrap_or(0);
    if argv.is_empty()
        || argv.len() > 4096
        || string(&argv[0]).is_empty()
        || limit == 0
        || limit > 64 << 20
        || c["exit_code"].as_u64().is_none_or(|v| v > u32::MAX as u64)
    {
        bail!("invalid command argv/output budget")
    }
    let mut total = data(&c["stdin"])?.len();
    for arg in argv {
        let s = arg.as_str().ok_or_else(|| anyhow!("invalid argv"))?;
        if s.contains('\0') {
            bail!("NUL in argv")
        }
        total += s.len();
    }
    let mut has_oracle = false;
    for key in ["stdout_sha256", "stderr_sha256"] {
        let h = string(&c[key]);
        if !h.is_empty() {
            if !valid_digest(h) {
                bail!("invalid command digest")
            }
            has_oracle = true;
        }
    }
    if !has_oracle {
        bail!("command needs exact output oracle")
    }
    let mut external = 0u64;
    if !c["files"].is_null() {
        let files = c["files"]
            .as_object()
            .ok_or_else(|| anyhow!("invalid files"))?;
        if files.len() > 1024 {
            bail!("too many command files")
        }
        for (name, f) in files {
            if !guest_path(name) || !valid_digest(string(&f["sha256"])) {
                bail!("invalid command fixture")
            }
            let bytes = data(&f["data"])?;
            let path = string(&f["path"]);
            let size = f["size"].as_u64().unwrap_or(0);
            if path.is_empty() {
                if digest(&bytes) != string(&f["sha256"])
                    || (size != 0 && size != bytes.len() as u64)
                {
                    bail!("command fixture digest/size mismatch")
                }
            } else {
                // The controller resolves bundle paths before preparation.
                if !Path::new(path).is_absolute()
                    || path.contains('\0')
                    || !bytes.is_empty()
                    || size > FILE_LIMIT
                {
                    bail!("invalid command file reference")
                }
                external += size;
            }
            total += name.len() + path.len() + bytes.len();
            for (i, _) in name.match_indices('/') {
                if files.contains_key(&name[..i]) {
                    bail!("command file/directory collision")
                }
            }
        }
    }
    let stdin = string(&c["stdin_file"]);
    if !stdin.is_empty() && (c["files"].get(stdin).is_none() || !data(&c["stdin"])?.is_empty()) {
        bail!("invalid command stdin file")
    }
    if total > INLINE_LIMIT || external > FILE_LIMIT {
        bail!("command input byte budget exceeded")
    }
    Ok(())
}

pub(super) fn stage(c: &Value) -> Result<tempfile::TempDir> {
    let dir = tempfile::Builder::new()
        .prefix("wasmbench-command-")
        .tempdir()?;
    if let Some(files) = c["files"].as_object() {
        for (name, f) in files {
            let path = dir.path().join(name);
            fs::create_dir_all(path.parent().unwrap())?;
            let mut out = fs::OpenOptions::new()
                .write(true)
                .create_new(true)
                .open(path)?;
            if string(&f["path"]).is_empty() {
                out.write_all(&data(&f["data"])?)?;
            } else {
                let path = PathBuf::from(string(&f["path"]));
                // Reject special files before opening; verify the opened handle too.
                if !fs::metadata(&path)?.is_file() {
                    bail!("command fixture type mismatch")
                }
                let input = fs::File::open(path)?;
                let info = input.metadata()?;
                let size = f["size"].as_u64().unwrap_or(0);
                if !info.is_file() || info.len() != size {
                    bail!("command fixture size/type mismatch")
                }
                let mut input = input.take(size + 1);
                let mut hash = Sha256::new();
                let mut copied = 0u64;
                let mut buf = [0u8; 65536];
                loop {
                    let n = input.read(&mut buf)?;
                    if n == 0 {
                        break;
                    }
                    hash.update(&buf[..n]);
                    out.write_all(&buf[..n])?;
                    copied += n as u64;
                }
                if copied != size || hex::encode(hash.finalize()) != string(&f["sha256"]) {
                    bail!("command fixture digest/size mismatch")
                }
            }
        }
    }
    Ok(dir)
}

#[derive(Default)]
struct Captured {
    bytes: Vec<u8>,
    overflow: bool,
}
#[derive(Clone)]
pub(super) struct Capture {
    state: Arc<Mutex<Captured>>,
    limit: usize,
}
impl Capture {
    pub(super) fn new(limit: usize) -> Self {
        Self {
            state: Arc::default(),
            limit,
        }
    }
    fn append(&self, bytes: &[u8]) -> io::Result<usize> {
        let mut s = self.state.lock().unwrap();
        if bytes.len() > self.limit - s.bytes.len() {
            s.overflow = true;
            return Err(io::Error::other("command output budget exceeded"));
        }
        s.bytes.extend_from_slice(bytes);
        Ok(bytes.len())
    }
    pub(super) fn evidence(&self) -> Result<(String, usize)> {
        let s = self.state.lock().unwrap();
        if s.overflow {
            bail!("command output budget exceeded")
        }
        Ok((digest(&s.bytes), s.bytes.len()))
    }
    fn oracle_digest(&self, normalizer: &str) -> Result<Option<String>> {
        if normalizer.is_empty() {
            return Ok(None);
        }
        let s = self.state.lock().unwrap();
        if s.overflow {
            bail!("command output budget exceeded")
        }
        Ok(Some(digest(&normalize_stdout(normalizer, &s.bytes)?)))
    }
}
impl IsTerminal for Capture {
    fn is_terminal(&self) -> bool {
        false
    }
}
impl StdoutStream for Capture {
    fn async_stream(&self) -> Box<dyn AsyncWrite + Send + Sync> {
        Box::new(self.clone())
    }
    fn p2_stream(&self) -> Box<dyn OutputStream> {
        Box::new(self.clone())
    }
}
#[async_trait::async_trait]
impl Pollable for Capture {
    async fn ready(&mut self) {}
}
impl OutputStream for Capture {
    fn write(&mut self, bytes: Bytes) -> StreamResult<()> {
        self.append(&bytes)
            .map(|_| ())
            .map_err(|e| StreamError::LastOperationFailed(anyhow!(e)))
    }
    fn flush(&mut self) -> StreamResult<()> {
        Ok(())
    }
    // Always allow a write attempt so even an ignored error records overflow.
    fn check_write(&mut self) -> StreamResult<usize> {
        Ok(65536)
    }
}
impl AsyncWrite for Capture {
    fn poll_write(self: Pin<&mut Self>, _: &mut Context<'_>, b: &[u8]) -> Poll<io::Result<usize>> {
        Poll::Ready(self.append(b))
    }
    fn poll_flush(self: Pin<&mut Self>, _: &mut Context<'_>) -> Poll<io::Result<()>> {
        Poll::Ready(Ok(()))
    }
    fn poll_shutdown(self: Pin<&mut Self>, _: &mut Context<'_>) -> Poll<io::Result<()>> {
        Poll::Ready(Ok(()))
    }
}

// Synthetic clocks advance one millisecond per read, independently per sample.
pub(super) struct Clock(pub(super) AtomicU64);
impl HostWallClock for Clock {
    fn resolution(&self) -> Duration {
        Duration::from_micros(1)
    }
    fn now(&self) -> Duration {
        Duration::from_nanos(self.0.fetch_add(1_000_000, Ordering::Relaxed))
    }
}
impl HostMonotonicClock for Clock {
    fn resolution(&self) -> u64 {
        1
    }
    fn now(&self) -> u64 {
        self.0.fetch_add(1_000_000, Ordering::Relaxed)
    }
}

impl Adapter {
    pub(super) fn run_command(
        &self,
        r: &Value,
        barrier: &mut dyn FnMut(u64, &str) -> Result<()>,
    ) -> Result<Value> {
        let w = self.workload()?;
        validate(w)?;
        let c = &w["command"];
        let scenario = string(&r["scenario"]);
        let profile = string(&self.prep.as_ref().unwrap()["profile"]);
        let phased = r["phase_barriers"].as_bool().unwrap_or(false);
        let samples = r["samples"].as_u64().unwrap_or(0);
        let requested_warmup = r["warmup"].as_u64().unwrap_or(0);
        if !["compile", "instantiate", "first-call", "steady", "teardown"].contains(&scenario)
            || !["timing", "memory"].contains(&profile)
            || (phased
                && (!["compile", "instantiate", "first-call", "teardown"].contains(&scenario)
                    || profile != "memory"))
            || samples == 0
            || samples > 100000
            || requested_warmup > 100000
            || r["operations"].as_u64().unwrap_or(0) == 0
        {
            bail!("unsupported command scenario/profile/batch")
        }
        let warmup = if scenario == "steady" {
            requested_warmup
        } else {
            0
        };
        let dir = stage(c)?;
        let runtime = || -> Result<_> {
            let engine = self.engine()?;
            let mut linker = Linker::<WasiP1Ctx>::new(&engine);
            p1::add_to_linker_sync(&mut linker, |ctx| ctx)?;
            if w["abi"] == "emscripten" {
                emscripten::add_to_linker(&mut linker)?;
            }
            Ok((engine, linker))
        };
        let shared_runtime = if scenario == "teardown" {
            None
        } else {
            Some(runtime()?)
        };
        let shared = if scenario == "compile" || scenario == "teardown" {
            None
        } else {
            Some(Module::new(
                &shared_runtime.as_ref().unwrap().0,
                &self.bytes,
            )?)
        };
        let argv: Vec<&str> = c["argv"].as_array().unwrap().iter().map(string).collect();
        let mut out = Vec::with_capacity((samples + warmup) as usize);
        for i in 0..samples + warmup {
            let owned_runtime = if scenario == "teardown" {
                Some(runtime()?)
            } else {
                None
            };
            let (engine, linker) = owned_runtime.as_ref().or(shared_runtime.as_ref()).unwrap();
            if phased && scenario == "compile" {
                barrier(i, "before_compile")?;
            }
            let mut elapsed = 0u64;
            let module = if let Some(module) = &shared {
                module.clone()
            } else {
                let start = Instant::now();
                let module = Module::new(engine, &self.bytes)?;
                elapsed = start.elapsed().as_nanos() as u64;
                module
            };
            if phased && scenario == "compile" {
                barrier(i, "compiled")?;
            }
            let stdout = Capture::new(c["output_limit_bytes"].as_u64().unwrap() as usize);
            let stderr = Capture::new(stdout.limit);
            let mut builder = WasiCtxBuilder::new();
            builder
                .args(&argv)
                .stdout(stdout.clone())
                .stderr(stderr.clone())
                .secure_random(wasmtime_wasi::random::Deterministic::new(vec![0]))
                .wall_clock(Clock(AtomicU64::new(1_640_995_200_000_000_000)))
                .monotonic_clock(Clock(AtomicU64::new(0)));
            if string(&c["stdin_file"]).is_empty() {
                builder.stdin(MemoryInputPipe::new(data(&c["stdin"])?));
            } else {
                builder.stdin(InputFile::new(fs::File::open(
                    dir.path().join(string(&c["stdin_file"])),
                )?));
            }
            if w["abi"] != "emscripten" && c["files"].as_object().is_some_and(|m| !m.is_empty()) {
                builder.preopened_dir(dir.path(), "/", DirPerms::READ, FilePerms::READ)?;
            }
            let mut store = Store::new(engine, builder.build_p1());
            if phased && scenario == "instantiate" {
                barrier(i, "before_instantiate")?;
            }
            let start = Instant::now();
            let instance = linker.instantiate(&mut store, &module)?;
            if scenario == "instantiate" {
                elapsed = start.elapsed().as_nanos() as u64;
            }
            if phased && scenario == "instantiate" {
                barrier(i, "instantiated")?;
            }
            let emscripten_main = if w["abi"] == "emscripten" {
                let ctors = instance.get_typed_func::<(), ()>(&mut store, "__wasm_call_ctors")?;
                ctors.call(&mut store, ())?;
                let stack_alloc = instance.get_typed_func::<i32, i32>(&mut store, "stackAlloc")?;
                let memory = instance
                    .get_memory(&mut store, "memory")
                    .ok_or_else(|| anyhow!("Emscripten memory export missing"))?;
                let argv_ptr = stack_alloc.call(&mut store, ((argv.len() + 1) * 4) as i32)?;
                let mut ptrs = Vec::with_capacity(argv.len() + 1);
                for arg in &argv {
                    let ptr = stack_alloc.call(&mut store, (arg.len() + 1) as i32)?;
                    memory.write(&mut store, ptr as usize, arg.as_bytes())?;
                    memory.write(&mut store, ptr as usize + arg.len(), &[0])?;
                    ptrs.push(ptr.to_le_bytes());
                }
                ptrs.push(0i32.to_le_bytes());
                for (n, ptr) in ptrs.iter().enumerate() {
                    memory.write(&mut store, argv_ptr as usize + n * 4, ptr)?;
                }
                Some((
                    instance.get_typed_func::<(i32, i32), i32>(&mut store, "main")?,
                    argv_ptr,
                ))
            } else {
                None
            };
            let wasi_start = if emscripten_main.is_none() {
                Some(instance.get_typed_func::<(), ()>(&mut store, "_start")?)
            } else {
                None
            };
            if phased && scenario == "first-call" {
                barrier(i, "before_first_call")?;
            }
            let start = Instant::now();
            let result = if let Some((main, argv_ptr)) = emscripten_main {
                main.call(&mut store, (argv.len() as i32, argv_ptr))
            } else {
                wasi_start.unwrap().call(&mut store, ()).map(|_| 0)
            };
            if scenario == "first-call" || scenario == "steady" {
                elapsed = start.elapsed().as_nanos() as u64;
            }
            if phased && scenario == "first-call" {
                barrier(i, "first_call_returned")?;
            }
            let exit_code = match result {
                Ok(code) => code as u32,
                Err(e) => match e.downcast_ref::<I32Exit>() {
                    Some(exit) => exit.0 as u32,
                    None => return Err(e),
                },
            };
            let (stdout_hash, stdout_bytes) = stdout.evidence()?;
            let (stderr_hash, stderr_bytes) = stderr.evidence()?;
            let stdout_oracle_hash = stdout.oracle_digest(string(&c["stdout_normalize"]))?;
            if exit_code as u64 != c["exit_code"].as_u64().unwrap()
                || (!string(&c["stdout_sha256"]).is_empty()
                    && stdout_oracle_hash.as_deref().unwrap_or(&stdout_hash)
                        != string(&c["stdout_sha256"]))
                || (!string(&c["stderr_sha256"]).is_empty()
                    && stderr_hash != string(&c["stderr_sha256"]))
            {
                bail!(
                    "incorrect result: command exit={} expected_exit={} stdout_raw={} stdout_oracle={} expected_stdout={} stderr_raw={} expected_stderr={}",
                    exit_code,
                    c["exit_code"],
                    stdout_hash,
                    stdout_oracle_hash.as_deref().unwrap_or(&stdout_hash),
                    string(&c["stdout_sha256"]),
                    stderr_hash,
                    string(&c["stderr_sha256"]),
                )
            }
            let mut result = json!({"exit_code":exit_code,"stdout_sha256":stdout_hash,"stderr_sha256":stderr_hash,"stdout_bytes":stdout_bytes,"stderr_bytes":stderr_bytes});
            if let Some(hash) = stdout_oracle_hash {
                result["stdout_oracle_sha256"] = json!(hash);
            }
            // Store owns the WASI context, table, preopens, and stdin descriptor.
            if phased && scenario == "teardown" {
                barrier(i, "before_teardown")?;
            }
            let release_start = Instant::now();
            drop(store);
            drop(module);
            drop(owned_runtime);
            if scenario == "teardown" {
                elapsed = release_start.elapsed().as_nanos() as u64;
            }
            if phased {
                barrier(
                    i,
                    if scenario == "teardown" {
                        "torn_down"
                    } else if scenario == "instantiate" {
                        "instance_released"
                    } else if scenario == "first-call" {
                        "first_call_released"
                    } else {
                        "released"
                    },
                )?;
            }
            out.push(json!({"index":i,"warmup":i < warmup,"elapsed_ns":elapsed,"operations":1,"sample_type":"individual_operation","verified":true,"command_result":result}));
        }
        Ok(json!({"samples":out}))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    static HOST_TEST_LOCK: Mutex<()> = Mutex::new(());
    fn workload() -> Value {
        json!({"abi":"wasi-command","export":"_start","reset":"fresh_instance_per_sample",
            "host_profile":"wasi-preview1-readonly-v1","oracle":{"kind":"exact_command"},
            "command":{"argv":["test","arg"],"stdin":"YWJj","exit_code":7,
                "stdout_sha256":digest(b"abc"),"stderr_sha256":digest(b"err"),"output_limit_bytes":16}})
    }
    #[test]
    fn llvm_ir_pred_normalizer_matches_the_declared_canonicalization() {
        let raw = b"block:       ; preds = %entry\n";
        let normalized = normalize_stdout("llvm-ir-preds", raw).unwrap();
        assert_eq!(normalized, b"block: ; preds = %entry\n");
        assert_ne!(digest(raw), digest(&normalized));
        assert_eq!(normalize_stdout("", raw).unwrap(), raw);
        assert!(normalize_stdout("future-normalizer", raw).is_err());
    }
    #[test]
    fn command_lifecycle_and_oracles() {
        let _guard = HOST_TEST_LOCK.lock().unwrap();
        for winch in [false, true] {
            for scenario in ["compile", "instantiate", "first-call", "steady", "teardown"] {
                for mode in [
                    "correct",
                    "memory",
                    "file",
                    "wrong-file",
                    "stdout",
                    "stderr",
                    "exit",
                    "overflow",
                    "short-input",
                    "mixed",
                    "phase",
                ] {
                    let dir = tempfile::tempdir().unwrap();
                    let mut w = workload();
                    if mode == "file" || mode == "wrong-file" {
                        let path = dir.path().join("input");
                        fs::write(&path, if mode == "file" { b"abc" } else { b"bad" }).unwrap();
                        w["command"]["stdin"] = json!("");
                        w["command"]["stdin_file"] = json!("input");
                        w["command"]["files"] =
                            json!({"input":{"path":path,"size":3,"sha256":digest(b"abc")}});
                    }
                    match mode {
                        "stdout" => w["command"]["stdout_sha256"] = json!(digest(b"")),
                        "stderr" => w["command"]["stderr_sha256"] = json!(digest(b"")),
                        "exit" => w["command"]["exit_code"] = json!(0),
                        "overflow" => w["command"]["output_limit_bytes"] = json!(1),
                        "short-input" => w["command"]["stdin"] = json!(""),
                        "mixed" => w["args"] = json!([1]),
                        _ => (),
                    }
                    let adapter = Adapter {
                        prep: Some(
                            json!({"workload":w,"profile":if mode == "memory" {"memory"} else {"timing"}}),
                        ),
                        bytes: include_bytes!("../../wazero/testdata/command.wasm").to_vec(),
                        winch,
                    };
                    let r = json!({"scenario":scenario,"samples":2,"operations":5,"warmup":1,"phase_barriers":mode == "phase"});
                    let result = adapter.run_command(&r, &mut |_, _| panic!("unexpected barrier"));
                    if !["correct", "memory", "file"].contains(&mode) {
                        assert!(result.is_err(), "accepted {winch}/{scenario}/{mode}");
                        continue;
                    }
                    let result =
                        result.unwrap_or_else(|e| panic!("{winch}/{scenario}/{mode}: {e:#}"));
                    let samples = result["samples"].as_array().unwrap();
                    assert_eq!(samples.len(), if scenario == "steady" { 3 } else { 2 });
                    for sample in samples {
                        assert_eq!(sample["verified"], true);
                        assert_eq!(sample["operations"], 1);
                        assert_eq!(sample["command_result"]["exit_code"], 7);
                        assert_eq!(sample["command_result"]["stdout_bytes"], 3);
                        assert_eq!(sample["command_result"]["stderr_bytes"], 3);
                    }
                }
            }
        }
    }
    #[test]
    fn capture_overflow_is_sticky_and_shared() {
        let output = Capture::new(3);
        let clone = output.clone();
        assert_eq!(clone.append(b"abc").unwrap(), 3);
        assert!(output.append(b"x").is_err());
        assert!(clone.evidence().is_err());
        assert_eq!(output.state.lock().unwrap().bytes, b"abc");
    }
    #[test]
    fn fixture_contract_rejects_ambiguity() {
        for name in ["../x", "/x", "a//b", "a/./b", "a\\b", "."] {
            let mut w = workload();
            w["command"]["files"] = json!({name:{"data":"YWJj","sha256":digest(b"abc")}});
            assert!(validate(&w).is_err(), "accepted {name}");
        }
        let mut w = workload();
        w["command"]["files"] = json!({"a":{"sha256":digest(b"")},"a/b":{"sha256":digest(b"")}});
        assert!(validate(&w).is_err());
        w = workload();
        w["command"]["stdin_file"] = json!("missing");
        assert!(validate(&w).is_err());
    }

    #[test]
    fn readonly_files_random_and_clocks() {
        let _guard = HOST_TEST_LOCK.lock().unwrap();
        for winch in [false, true] {
            let mut w = workload();
            w["command"]["files"] = json!({"input":{"data":"YWJj","sha256":digest(b"abc")}});
            w["command"]["exit_code"] = json!(0);
            w["command"]["stderr_sha256"] = json!(digest(b""));
            let a = Adapter {
                prep: Some(json!({"workload":w,"profile":"timing"})),
                bytes: include_bytes!("../testdata/command-files.wasm").to_vec(),
                winch,
            };
            let r = json!({"scenario":"first-call","samples":3,"operations":1,"warmup":0});
            a.run_command(&r, &mut |_, _| panic!("unexpected barrier"))
                .unwrap();
        }
    }

    #[test]
    fn emscripten_stdio_main_and_exit_oracle() {
        let mut w = workload();
        w["abi"] = json!("emscripten");
        w["export"] = json!("main");
        w["host_profile"] = json!("emscripten-stdio-v1");
        assert!(validate(&w).is_ok());
        let mut mismatched = w.clone();
        mismatched["host_profile"] = json!("wasi-preview1-readonly-v1");
        assert!(validate(&mismatched).is_err());
        w["command"]["argv"] = json!(["f"]);
        w["command"]["stdin"] = json!("");
        w["command"]["exit_code"] = json!(0);
        w["command"]["stdout_sha256"] = json!(digest(b""));
        w["command"]["stderr_sha256"] = json!(digest(b""));
        let adapter = Adapter {
            prep: Some(json!({"workload":w,"profile":"timing"})),
            bytes: include_bytes!("../../wazero/testdata/emscripten-command.wasm").to_vec(),
            winch: false,
        };
        let result = adapter
            .run_command(
                &json!({"scenario":"first-call","samples":1,"operations":1,"warmup":0}),
                &mut |_, _| panic!("unexpected barrier"),
            )
            .unwrap();
        assert_eq!(result["samples"][0]["command_result"]["exit_code"], 0);
    }

    #[cfg(target_os = "linux")]
    #[test]
    fn owned_context_releases_descriptors_between_samples() {
        let _guard = HOST_TEST_LOCK.lock().unwrap();
        // A leaked stdin or preopen per invocation would accumulate across this
        // batch. Check the live process at each release barrier, not only after
        // the entire run has dropped its engine and temporary fixture root.
        for winch in [false, true] {
            let source = tempfile::tempdir().unwrap();
            let path = source.path().join("input");
            fs::write(&path, b"abc").unwrap();
            let mut w = workload();
            w["command"]["stdin"] = json!("");
            w["command"]["stdin_file"] = json!("input");
            w["command"]["files"] = json!({"input":{"path":path,"size":3,"sha256":digest(b"abc")}});
            let a = Adapter {
                prep: Some(json!({"workload":w,"profile":"memory"})),
                bytes: include_bytes!("../../wazero/testdata/command.wasm").to_vec(),
                winch,
            };
            let r = json!({"scenario":"compile","samples":32,"operations":1,"warmup":0,"phase_barriers":true});
            let mut counts = Vec::new();
            a.run_command(&r, &mut |_, stage| {
                if stage == "released" {
                    counts.push(fs::read_dir("/proc/self/fd")?.count());
                }
                Ok(())
            })
            .unwrap();
            assert_eq!(counts.len(), 32);
            assert!(counts[31] <= counts[0] + 2, "descriptor growth: {counts:?}");
        }
    }
}
