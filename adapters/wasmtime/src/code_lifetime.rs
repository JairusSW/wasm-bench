//! Diagnostic executable-text publication, not allocation or physical residency.
//!
//! No ordinary engine installs this hook. The initial qualification uses pinned
//! Wasmtime's cache-maintenance crate and the same Linux RW -> RX discipline as
//! its normal publisher. This internal crate is unsupported outside Wasmtime;
//! changes of engine version require requalification, not semver assumptions.
use rustix::mm::{MprotectFlags, mprotect};
use std::{collections::BTreeMap, sync::Mutex, time::Instant};
use wasmtime::{CustomCodeMemory, Result, bail, format_err};

#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Event {
    pub sequence: u64,
    pub elapsed_ns: u64,
    pub publication: u64,
    pub kind: &'static str,
    pub address: usize,
    pub capacity: usize,
    pub active_capacity: usize,
    pub cumulative_published_capacity: usize,
}

#[derive(Default)]
struct Ledger {
    active: BTreeMap<usize, (u64, usize)>,
    events: Vec<Event>,
    next_publication: u64,
    active_capacity: usize,
    cumulative: usize,
}

pub struct Publisher {
    alignment: usize,
    bti: bool,
    origin: Instant,
    ledger: Mutex<Ledger>,
}

impl Publisher {
    pub fn new() -> Result<Self> {
        #[cfg(target_arch = "aarch64")]
        let bti = std::arch::is_aarch64_feature_detected!("bti");
        #[cfg(not(target_arch = "aarch64"))]
        let bti = false;
        if !cfg!(any(target_arch = "aarch64", target_arch = "x86_64")) {
            bail!("native code lifetime publisher requires Linux arm64/amd64");
        }
        Ok(Self {
            alignment: rustix::param::page_size(),
            bti,
            origin: Instant::now(),
            ledger: Mutex::new(Ledger::default()),
        })
    }

    /// Attach only through this method: the public callback omits the engine's
    /// branch-protection flag. Pin code generation and page protection together.
    pub fn configure(self: &std::sync::Arc<Self>, config: &mut wasmtime::Config) {
        #[cfg(target_arch = "aarch64")]
        // SAFETY: use_bti is a pinned AArch64 boolean ISA setting. It is enabled
        // only when this host reports BTI; both backends are execution-tested.
        unsafe {
            config.cranelift_flag_set("use_bti", if self.bti { "true" } else { "false" });
        }
        config.with_custom_code_memory(Some(self.clone()));
    }

    pub fn events(&self) -> Result<Vec<Event>> {
        Ok(self
            .ledger
            .lock()
            .map_err(|_| format_err!("publication ledger poisoned"))?
            .events
            .clone())
    }

    fn check_range(&self, address: usize, length: usize) -> Result<()> {
        if address == 0
            || length == 0
            || address % self.alignment != 0
            || length % self.alignment != 0
            || address.checked_add(length).is_none()
        {
            bail!("invalid executable-text publication range");
        }
        Ok(())
    }

    fn elapsed(&self) -> Result<u64> {
        self.origin
            .elapsed()
            .as_nanos()
            .try_into()
            .map_err(|_| format_err!("publication clock overflow"))
    }

    fn checkpoint(&self, stage: &str) -> Result<serde_json::Value> {
        let ledger = self
            .ledger
            .lock()
            .map_err(|_| format_err!("publication ledger poisoned"))?;
        Ok(
            serde_json::json!({"stage":stage,"elapsed_ns":self.elapsed()?,"event_count":ledger.events.len(),"active_capacity":ledger.active_capacity,"cumulative_published_capacity":ledger.cumulative}),
        )
    }
}

pub fn validate(prep: &serde_json::Value, request: &serde_json::Value) -> Result<()> {
    use serde_json::json;
    if request["scenario"] != "code-lifetime" {
        bail!("explicit code lifetime scenario required");
    }
    let mut materialized = request.clone();
    materialized["scenario"] = json!("compile-materialized");
    crate::native_code::validate_materialized(prep, &materialized)?;
    let w = &prep["workload"];
    if w["host_profile"].as_str().is_some_and(|s| !s.is_empty())
        || !w["continuation"].is_null()
        || w["oracle"]["output_pointer_export"]
            .as_str()
            .is_some_and(|s| !s.is_empty())
        || w["oracle"]["expected"]
            .as_array()
            .is_none_or(|v| v.is_empty())
        || request["sustained_duration_ns"].as_i64().unwrap_or(0) != 0
        || request["sustained_post_collection"] == true
    {
        bail!(
            "code lifetime requires a stateless exact-result core diagnostic without host imports or compound state"
        );
    }
    Ok(())
}

/// One verified compile/instance lifetime. All publication and ownership clocks
/// are diagnostic. No Sample elapsed value is returned as headline latency.
pub fn run(adapter: &crate::Adapter, request: &serde_json::Value) -> Result<serde_json::Value> {
    use serde_json::{Value, json};
    use std::sync::Arc;
    use wasmtime::{Config, Engine, Module, Strategy};
    let prep = adapter
        .prep
        .as_ref()
        .ok_or_else(|| format_err!("prepare required"))?;
    validate(prep, request)?;
    let publisher = Arc::new(Publisher::new()?);
    let mut config = Config::new();
    config.strategy(if adapter.winch {
        Strategy::Winch
    } else {
        Strategy::Cranelift
    });
    config.parallel_compilation(false);
    crate::features::configure(&mut config, adapter.winch);
    publisher.configure(&mut config);
    let engine = Engine::new(&config)?;
    let module = Module::new(&engine, &adapter.bytes)?;
    // Exclude host imports: their lazily generated bridges add separate regions
    // and require a separate attribution/ownership contract, not guesswork.
    if module.imports().len() != 0 {
        bail!("code lifetime module must have no imports");
    }
    let compiled = publisher.checkpoint("compiled")?;
    let published = publisher.events()?;
    if published.len() != 1 || published[0].kind != "published" {
        bail!("code lifetime requires one attributed module publication");
    }
    let image_address = module.text().as_ptr() as usize;
    let image_offset = image_address
        .checked_sub(published[0].address)
        .ok_or_else(|| format_err!("native image outside publication"))?;
    if image_offset > published[0].capacity
        || module.text().len() > published[0].capacity - image_offset
    {
        bail!("native image outside publication");
    }
    let mut output = crate::native_code::inspect(&module, &adapter.bytes, adapter.winch)?;
    if output["code_image"].is_null()
        || output["code_image"]["functions"]
            .as_array()
            .is_none_or(|f| f.is_empty() || f.len() > 10000)
    {
        bail!("code lifetime image exceeds attribution/transport contract");
    }
    let clone = module.clone();
    let mut state = adapter.prepare_state(&engine, &module)?;
    state
        .function
        .call(&mut state.store, &state.args, &mut state.results)?;
    let results = state.results.clone();
    let before = adapter.verify(&mut state, &results)?;
    let verified = publisher.checkpoint("instance_verified")?;
    drop(module);
    drop(clone);
    state
        .function
        .call(&mut state.store, &state.args, &mut state.results)?;
    let results = state.results.clone();
    let after = adapter.verify(&mut state, &results)?;
    let module_dropped = publisher.checkpoint("module_handles_dropped")?;
    drop(state);
    let store_dropped = publisher.checkpoint("store_dropped")?;
    drop(engine);
    let engine_dropped = publisher.checkpoint("engine_dropped")?;
    let events: Vec<Value> = publisher.events()?.into_iter().map(|e| json!({"sequence":e.sequence,"elapsed_ns":e.elapsed_ns,"publication":e.publication,"kind":e.kind,"address":e.address.to_string(),"capacity":e.capacity,"active_capacity":e.active_capacity,"cumulative_published_capacity":e.cumulative_published_capacity})).collect();
    if events.len() != 2
        || events[1]["kind"] != "unpublished"
        || events[1]["active_capacity"] != 0
        || compiled["event_count"] != 1
        || verified["event_count"] != 1
        || module_dropped["event_count"] != 1
        || store_dropped["event_count"] != 2
        || engine_dropped["event_count"] != 2
    {
        bail!("code lifetime ownership sequence differs from qualified contract");
    }
    output["code_lifetime"] = json!({"version":1,"collector":"Wasmtime/CustomCodeMemory","collector_version":"46.0.1","scope":"published_executable_text_capacity","quality":"engine_callback","module_sha256":output["code_image"]["module_sha256"],"image_sha256":output["code_image"]["sha256"],"backend":output["code_image"]["backend"],"architecture":output["code_image"]["architecture"],"page_size":publisher.alignment,"bti":publisher.bti,"publication":1,"image_offset":image_offset,"release_policy":"drop_module_handles_then_store_then_engine","result_before_drop":before,"result_after_drop":after,"events":events,"checkpoints":[compiled,verified,module_dropped,store_dropped,engine_dropped]});
    Ok(output)
}

impl CustomCodeMemory for Publisher {
    fn required_alignment(&self) -> usize {
        self.alignment
    }

    fn publish_executable(&self, ptr: *const u8, len: usize) -> Result<()> {
        let address = ptr as usize;
        self.check_range(address, len)?;
        let mut ledger = self
            .ledger
            .lock()
            .map_err(|_| format_err!("publication ledger poisoned"))?;
        if ledger
            .active
            .iter()
            .any(|(&start, &(_, length))| address < start + length && start < address + len)
        {
            bail!("overlapping executable-text publication");
        }
        let capacity = ledger
            .active_capacity
            .checked_add(len)
            .ok_or_else(|| format_err!("active capacity overflow"))?;
        let cumulative = ledger
            .cumulative
            .checked_add(len)
            .ok_or_else(|| format_err!("cumulative capacity overflow"))?;
        let publication = ledger
            .next_publication
            .checked_add(1)
            .ok_or_else(|| format_err!("publication identity overflow"))?;
        // Reserve event storage before permissions change. Wasmtime guarantees
        // a valid aligned region. Map insertion can still allocate; OOM aborts
        // the adapter rather than returning successful, incomplete evidence.
        ledger.events.try_reserve(1)?;
        unsafe {
            wasmtime_internal_jit_icache_coherence::clear_cache(ptr.cast(), len)?;
            let flags = MprotectFlags::READ | MprotectFlags::EXEC;
            let flags = if self.bti {
                MprotectFlags::from_bits_retain(flags.bits() | 0x10)
            } else {
                flags
            };
            mprotect(ptr.cast_mut().cast(), len, flags)?;
        }
        if let Err(error) = wasmtime_internal_jit_icache_coherence::pipeline_flush_mt() {
            unsafe {
                mprotect(
                    ptr.cast_mut().cast(),
                    len,
                    MprotectFlags::READ | MprotectFlags::WRITE,
                )?;
            }
            return Err(error.into());
        }
        let elapsed_ns = match self.elapsed() {
            Ok(time) => time,
            Err(error) => {
                unsafe {
                    mprotect(
                        ptr.cast_mut().cast(),
                        len,
                        MprotectFlags::READ | MprotectFlags::WRITE,
                    )?;
                }
                return Err(error);
            }
        };
        ledger.active.insert(address, (publication, len));
        ledger.next_publication = publication;
        ledger.active_capacity = capacity;
        ledger.cumulative = cumulative;
        let sequence = ledger.events.len() as u64;
        ledger.events.push(Event {
            sequence,
            elapsed_ns,
            publication,
            kind: "published",
            address,
            capacity: len,
            active_capacity: capacity,
            cumulative_published_capacity: cumulative,
        });
        Ok(())
    }

    fn unpublish_executable(&self, ptr: *const u8, len: usize) -> Result<()> {
        // Wasmtime 46 marks empty CodeMemory published without calling the
        // publish hook, then invokes this hook with length zero on drop.
        // There is no executable range or event to retire in that case.
        if len == 0 {
            return Ok(());
        }
        let address = ptr as usize;
        self.check_range(address, len)?;
        let mut ledger = self
            .ledger
            .lock()
            .map_err(|_| format_err!("publication ledger poisoned"))?;
        let (publication, length) = *ledger
            .active
            .get(&address)
            .ok_or_else(|| format_err!("unknown executable-text retirement"))?;
        if length != len {
            bail!("executable-text retirement length mismatch");
        }
        ledger.events.try_reserve(1)?;
        let elapsed_ns = self.elapsed()?;
        // Wasmtime guarantees no code is executing from this region now.
        unsafe {
            mprotect(
                ptr.cast_mut().cast(),
                len,
                MprotectFlags::READ | MprotectFlags::WRITE,
            )?;
        }
        ledger.active.remove(&address);
        ledger.active_capacity -= len;
        let active_capacity = ledger.active_capacity;
        let cumulative = ledger.cumulative;
        let sequence = ledger.events.len() as u64;
        ledger.events.push(Event {
            sequence,
            elapsed_ns,
            publication,
            kind: "unpublished",
            address,
            capacity: len,
            active_capacity,
            cumulative_published_capacity: cumulative,
        });
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::Arc;
    use wasmtime::{Config, Engine, Instance, Module, Store, Strategy};

    fn engine(winch: bool, publisher: Arc<Publisher>) -> Result<Engine> {
        let mut config = Config::new();
        config.strategy(if winch {
            Strategy::Winch
        } else {
            Strategy::Cranelift
        });
        config.parallel_compilation(false);
        crate::features::configure(&mut config, winch);
        publisher.configure(&mut config);
        Engine::new(&config)
    }

    #[test]
    fn actual_module_and_store_ownership_controls_retirement() -> Result<()> {
        let wasm = hex::decode(
            "0061736d010000000105016000017f030201000707010372756e00000a0601040041070b",
        )?;
        for winch in [false, true] {
            let publisher = Arc::new(Publisher::new()?);
            let engine = engine(winch, publisher.clone())?;
            let module = Module::new(&engine, &wasm)?;
            let clone = module.clone();
            let published = publisher.events()?;
            assert_eq!(published.len(), 1);
            assert_eq!(published[0].kind, "published");
            assert!(published[0].capacity >= module.text().len());
            let maps = std::fs::read_to_string("/proc/self/maps")?;
            let mapping = maps
                .lines()
                .find(|line| {
                    let range = line.split_whitespace().next().unwrap();
                    let (start, end) = range.split_once('-').unwrap();
                    let start = usize::from_str_radix(start, 16).unwrap();
                    let end = usize::from_str_radix(end, 16).unwrap();
                    start <= published[0].address && published[0].address < end
                })
                .expect("published executable region must exist");
            assert_eq!(
                mapping.split_whitespace().nth(1).unwrap(),
                "r-xp",
                "publication enforces W^X"
            );
            assert!(
                publisher
                    .publish_executable(published[0].address as *const u8, published[0].capacity)
                    .is_err()
            );
            assert!(
                publisher
                    .unpublish_executable(
                        published[0].address as *const u8,
                        published[0].capacity + publisher.alignment
                    )
                    .is_err()
            );
            assert_eq!(
                publisher.events()?,
                published,
                "invalid callbacks do not mutate evidence or permissions"
            );
            let mut store = Store::new(&engine, ());
            let instance = Instance::new(&mut store, &module, &[])?;
            let run = instance.get_typed_func::<(), i32>(&mut store, "run")?;
            assert_eq!(run.call(&mut store, ())?, 7);
            drop(module);
            drop(clone);
            assert_eq!(
                publisher.events()?.len(),
                1,
                "Store still owns executable code"
            );
            assert_eq!(
                run.call(&mut store, ())?,
                7,
                "code stays executable after Module handles drop"
            );
            drop(store);
            let events = publisher.events()?;
            assert_eq!(events.len(), 2);
            assert_eq!(events[1].kind, "unpublished");
            assert_eq!(events[1].publication, events[0].publication);
            assert_eq!(events[1].address, events[0].address);
            assert_eq!(events[1].active_capacity, 0);
            assert_eq!(events[1].cumulative_published_capacity, events[0].capacity);
            assert!(events[1].elapsed_ns >= events[0].elapsed_ns);
            drop(engine);
            assert_eq!(
                publisher.events()?.len(),
                2,
                "engine lifetime is not module lifetime"
            );
        }
        Ok(())
    }

    #[test]
    fn repeated_compile_drop_accumulates_publication_not_retention() -> Result<()> {
        let wasm = hex::decode("0061736d010000000105016000017f030201000a0601040041070b")?;
        for winch in [false, true] {
            let publisher = Arc::new(Publisher::new()?);
            let engine = engine(winch, publisher.clone())?;
            for iteration in 0..4 {
                let module = Module::new(&engine, &wasm)?;
                let events = publisher.events()?;
                assert_eq!(events.len(), iteration * 2 + 1);
                assert_eq!(events.last().unwrap().publication, iteration as u64 + 1);
                drop(module);
                let events = publisher.events()?;
                assert_eq!(events.last().unwrap().active_capacity, 0);
                assert!(events.last().unwrap().cumulative_published_capacity > 0);
            }
            let events = publisher.events()?;
            let emitted: usize = events
                .iter()
                .filter(|e| e.kind == "published")
                .map(|e| e.capacity)
                .sum();
            assert_eq!(
                events.last().unwrap().cumulative_published_capacity,
                emitted
            );
        }
        Ok(())
    }

    #[test]
    fn invalid_callbacks_do_not_emit_events() -> Result<()> {
        let publisher = Publisher::new()?;
        assert!(publisher.publish_executable(std::ptr::null(), 0).is_err());
        assert!(
            publisher
                .unpublish_executable(publisher.alignment as *const u8, publisher.alignment)
                .is_err()
        );
        assert!(publisher.events()?.is_empty());
        Ok(())
    }

    #[test]
    fn simultaneous_modules_keep_distinct_live_publications() -> Result<()> {
        let wasm = hex::decode("0061736d010000000105016000017f030201000a0601040041070b")?;
        for winch in [false, true] {
            let publisher = Arc::new(Publisher::new()?);
            let engine = engine(winch, publisher.clone())?;
            let first = Module::new(&engine, &wasm)?;
            let second = Module::new(&engine, &wasm)?;
            let events = publisher.events()?;
            assert_eq!(events.len(), 2);
            assert_ne!(events[0].address, events[1].address);
            assert_ne!(events[0].publication, events[1].publication);
            assert_eq!(
                events[1].active_capacity,
                events[0].capacity + events[1].capacity
            );
            drop(first);
            let retired = publisher.events()?;
            assert_eq!(retired[2].publication, events[0].publication);
            assert_eq!(retired[2].active_capacity, events[1].capacity);
            drop(second);
            let retired = publisher.events()?;
            assert_eq!(retired[3].active_capacity, 0);
            assert_eq!(
                retired[3].cumulative_published_capacity,
                events[1].cumulative_published_capacity
            );
        }
        Ok(())
    }

    #[test]
    fn invalid_and_empty_modules_do_not_invent_code_publications() -> Result<()> {
        for winch in [false, true] {
            let publisher = Arc::new(Publisher::new()?);
            let engine = engine(winch, publisher.clone())?;
            assert!(Module::new(&engine, b"invalid wasm").is_err());
            assert!(publisher.events()?.is_empty());
            let empty = Module::new(&engine, b"\0asm\x01\0\0\0")?;
            assert_eq!(empty.text().len(), 0);
            drop(empty);
            assert!(publisher.events()?.is_empty());
        }
        Ok(())
    }

    #[test]
    fn diagnostic_wire_output_binds_image_and_verified_ownership() -> Result<()> {
        use serde_json::json;
        let wasm = hex::decode(
            "0061736d010000000105016000017f030201000707010372756e00000a0601040041070b",
        )?;
        let prep = json!({"profile":"code","workload":{"abi":"core","reset":"stateless","export":"run","args":[],"oracle":{"kind":"exact_u64","expected":["7"]}}});
        let request = json!({"scenario":"code-lifetime","samples":1,"operations":1,"warmup":0});
        for winch in [false, true] {
            let mut adapter = crate::Adapter {
                prep: Some(prep.clone()),
                bytes: wasm.clone(),
                winch,
            };
            let output = run(&adapter, &request)?;
            let l = &output["code_lifetime"];
            assert_eq!(l["image_sha256"], output["code_image"]["sha256"]);
            assert_eq!(l["module_sha256"], output["code_image"]["module_sha256"]);
            assert_eq!(l["result_before_drop"], json!(["7"]));
            assert_eq!(l["result_after_drop"], json!(["7"]));
            assert_eq!(l["events"].as_array().unwrap().len(), 2);
            assert!(l["events"][0]["address"].is_string());
            assert_eq!(l["checkpoints"].as_array().unwrap().len(), 5);
            assert_eq!(l["checkpoints"][4]["active_capacity"], 0);
            assert!(
                output["samples"].is_null(),
                "diagnostic event clocks are not timing samples"
            );
            adapter.prep.as_mut().unwrap()["workload"]["oracle"]["expected"] = json!(["8"]);
            assert!(
                run(&adapter, &request).is_err(),
                "wrong oracle cannot return lifecycle success"
            );
        }
        let mut timing = prep.clone();
        timing["profile"] = json!("timing");
        assert!(validate(&timing, &request).is_err());
        for (key, value) in [
            ("samples", json!(2)),
            ("warmup", json!(1)),
            ("operations", json!(2)),
            ("phase_barriers", json!(true)),
        ] {
            let mut r = request.clone();
            r[key] = value;
            assert!(validate(&prep, &r).is_err());
        }
        Ok(())
    }
}
