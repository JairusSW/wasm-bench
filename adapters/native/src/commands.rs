use super::*;
use anyhow::anyhow;
use base64::Engine as _;
use std::{
    io::Read,
    path::{Path, PathBuf},
};
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
    ensure!(
        w["abi"] == "wasi-command",
        "unsupported: WASI command required"
    );
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

impl Adapter {
    pub(super) fn run_command(&self, req: &Value, input: &mut impl BufRead) -> Result<Value> {
        let prep = self
            .prep
            .as_ref()
            .ok_or_else(|| anyhow!("prepare required"))?;
        let workload = &prep["workload"];
        validate(workload)?;
        let command = &workload["command"];
        let run = &req["run"];
        let scenario = field(run, "scenario")?;
        ensure!(
            ["compile", "instantiate", "first-call", "steady"].contains(&scenario),
            "unsupported: command scenario"
        );
        ensure!(
            ["timing", "memory"].contains(&field(prep, "profile")?),
            "unsupported: command run profile"
        );
        let phases = run["phase_barriers"].as_bool().unwrap_or(false);
        ensure!(
            !phases || prep["profile"] == "memory" && scenario != "steady",
            "unsupported: command phase barriers"
        );
        let count = |name: &str, minimum: u64, maximum: u64| -> Result<usize> {
            Ok(run[name]
                .as_u64()
                .filter(|n| *n >= minimum && *n <= maximum)
                .ok_or_else(|| anyhow!("invalid command {name}"))? as usize)
        };
        let samples = count("samples", 1, 100000)?;
        count("operations", 1, 1000000)?;
        let requested_warmup = count("warmup", 0, 100000)?;
        let warmup = if scenario == "steady" {
            requested_warmup
        } else {
            0
        };
        // Files, hashes and stdin setup are outside every timing window.
        let directory = stage(command)?;
        let stdin = if string(&command["stdin_file"]).is_empty() {
            data(&command["stdin"])?
        } else {
            fs::read(directory.path().join(string(&command["stdin_file"])))?
        };
        let argv: Vec<String> = command["argv"]
            .as_array()
            .unwrap()
            .iter()
            .map(|v| string(v).to_owned())
            .collect();
        let root = directory
            .path()
            .to_str()
            .ok_or_else(|| anyhow!("non-UTF8 fixture directory"))?;
        let limit = command["output_limit_bytes"].as_u64().unwrap() as usize;
        let shared = if scenario == "compile" {
            None
        } else {
            Some(embedding::WasiModule::compile(&self.bytes)?)
        };
        let stages = match scenario {
            "compile" => ["before_compile", "compiled", "released"],
            "instantiate" => ["before_instantiate", "instantiated", "instance_released"],
            _ => [
                "before_first_call",
                "first_call_returned",
                "first_call_released",
            ],
        };
        let mut output = Vec::new();
        for index in 0..samples + warmup {
            if phases && scenario == "compile" {
                barrier(input, &req["id"], index, stages[0])?;
            }
            let mut elapsed = 0u128;
            let owned = if shared.is_none() {
                let begin = Instant::now();
                let module = embedding::WasiModule::compile(&self.bytes)?;
                elapsed = begin.elapsed().as_nanos();
                Some(module)
            } else {
                None
            };
            if phases && scenario == "compile" {
                barrier(input, &req["id"], index, stages[1])?;
            }
            if phases && scenario == "instantiate" {
                barrier(input, &req["id"], index, stages[0])?;
            }
            let begin = Instant::now();
            let instance = shared
                .as_ref()
                .or(owned.as_ref())
                .unwrap()
                .instantiate(&argv, root, &stdin, limit)?;
            if scenario == "instantiate" {
                elapsed = begin.elapsed().as_nanos();
            }
            if phases && scenario == "instantiate" {
                barrier(input, &req["id"], index, stages[1])?;
            }
            if phases && scenario == "first-call" {
                barrier(input, &req["id"], index, stages[0])?;
            }
            let begin = Instant::now();
            let exit = instance.run()?;
            if ["first-call", "steady"].contains(&scenario) {
                elapsed = begin.elapsed().as_nanos();
            }
            if phases && scenario == "first-call" {
                barrier(input, &req["id"], index, stages[1])?;
            }
            let stdout = instance.output(false)?;
            let stderr = instance.output(true)?;
            let raw = digest(&stdout);
            let stderr_hash = digest(&stderr);
            let normalizer = string(&command["stdout_normalize"]);
            let canonical = digest(&normalize_stdout(normalizer, &stdout)?);
            ensure!(
                exit as u64 == command["exit_code"].as_u64().unwrap()
                    && (string(&command["stdout_sha256"]).is_empty()
                        || canonical == string(&command["stdout_sha256"]))
                    && (string(&command["stderr_sha256"]).is_empty()
                        || stderr_hash == string(&command["stderr_sha256"])),
                "incorrect result: command exit/output oracle mismatch"
            );
            let mut result = json!({"exit_code":exit,"stdout_sha256":raw,"stderr_sha256":stderr_hash,"stdout_bytes":stdout.len(),"stderr_bytes":stderr.len()});
            if !normalizer.is_empty() {
                result["stdout_oracle_sha256"] = json!(canonical);
            }
            let guest_bytes = instance.memory_bytes();
            drop(instance);
            drop(owned);
            if phases {
                barrier(input, &req["id"], index, stages[2])?;
            }
            let mut sample = json!({"index":index,"warmup":index<warmup,"elapsed_ns":u64::try_from(elapsed)?,"operations":1,"sample_type":"individual_operation","verified":true,"command_result":result});
            if prep["profile"] == "memory" {
                sample["observations"] = json!([{"metric":"guest.memory.logical","definition_version":1,"value":guest_bytes,"unit":"bytes","scope":"guest_linear_memory","phase":format!("{scenario}/command_verified"),"collector":format!("{RUNTIME} embedding memory API"),"collector_version":embedding::version(),"quality":"engine_reported","profile":"memory","status":"available","normalization_denominator":"instance"}]);
            }
            output.push(sample);
        }
        Ok(json!({"samples":output}))
    }
}
