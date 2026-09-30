//! Dedicated diagnostic binary only. Counts successful requests routed through
//! Rust GlobalAlloc, not mmap, foreign allocators, usable size or physical RSS.
use super::*;
use std::alloc::{GlobalAlloc, Layout, System};
use std::cell::UnsafeCell;
use std::sync::atomic::{AtomicBool, Ordering};

#[derive(Clone, Copy, Default)]
struct Counts {
    allocated: u64,
    allocations: u64,
    freed: u64,
    outstanding: u64,
    peak: u64,
    active: bool,
    overflow: bool,
}
struct Tracker {
    locked: AtomicBool,
    counts: UnsafeCell<Counts>,
}
// All access to counts is serialized by the allocation-free spin lock.
unsafe impl Sync for Tracker {}
impl Tracker {
    const fn new() -> Self {
        Self {
            locked: AtomicBool::new(false),
            counts: UnsafeCell::new(Counts {
                allocated: 0,
                allocations: 0,
                freed: 0,
                outstanding: 0,
                peak: 0,
                active: false,
                overflow: false,
            }),
        }
    }
    fn with<T>(&self, f: impl FnOnce(&mut Counts) -> T) -> T {
        while self
            .locked
            .compare_exchange_weak(false, true, Ordering::Acquire, Ordering::Relaxed)
            .is_err()
        {
            std::hint::spin_loop();
        }
        // Closures below only do bounded integer accounting; never allocate or unwind.
        let result = f(unsafe { &mut *self.counts.get() });
        self.locked.store(false, Ordering::Release);
        result
    }
    fn add(value: &mut u64, amount: u64, overflow: &mut bool) {
        match value.checked_add(amount) {
            Some(n) => *value = n,
            None => {
                *overflow = true;
                *value = u64::MAX;
            }
        }
    }
    fn update(&self, allocated: u64, freed: u64, success: bool) {
        self.with(|c| {
            if success {
                Self::add(&mut c.allocated, allocated, &mut c.overflow);
                Self::add(&mut c.allocations, 1, &mut c.overflow);
            }
            Self::add(&mut c.freed, freed, &mut c.overflow);
            match c.outstanding.checked_sub(freed) {
                Some(n) => c.outstanding = n,
                None => {
                    c.outstanding = 0;
                    c.overflow = true;
                }
            }
            Self::add(&mut c.outstanding, allocated, &mut c.overflow);
            if c.active {
                c.peak = c.peak.max(c.outstanding);
            }
        });
    }
    fn start(&self) -> Counts {
        self.with(|c| {
            c.active = true;
            c.peak = c.outstanding;
            *c
        })
    }
    fn finish(&self) -> Counts {
        self.with(|c| {
            c.active = false;
            *c
        })
    }
}
static TRACKER: Tracker = Tracker::new();
struct TrackedSystem;
#[global_allocator]
static ALLOCATOR: TrackedSystem = TrackedSystem;
unsafe impl GlobalAlloc for TrackedSystem {
    unsafe fn alloc(&self, layout: Layout) -> *mut u8 {
        let p = unsafe { System.alloc(layout) };
        if !p.is_null() {
            TRACKER.update(layout.size() as u64, 0, true);
        }
        p
    }
    unsafe fn alloc_zeroed(&self, layout: Layout) -> *mut u8 {
        let p = unsafe { System.alloc_zeroed(layout) };
        if !p.is_null() {
            TRACKER.update(layout.size() as u64, 0, true);
        }
        p
    }
    unsafe fn dealloc(&self, ptr: *mut u8, layout: Layout) {
        TRACKER.update(0, layout.size() as u64, false);
        unsafe { System.dealloc(ptr, layout) };
    }
    unsafe fn realloc(&self, ptr: *mut u8, layout: Layout, new_size: usize) -> *mut u8 {
        let p = unsafe { System.realloc(ptr, layout, new_size) };
        // Failure preserves ownership and all counters. Successful realloc is
        // one new-size request and old-size logical release, even if in-place.
        if !p.is_null() {
            TRACKER.update(new_size as u64, layout.size() as u64, true);
        }
        p
    }
}

fn observations(before: Counts, after: Counts, phase: &str) -> Result<Vec<Value>> {
    if before.overflow || after.overflow {
        bail!("Rust allocator counters overflowed or became inconsistent");
    }
    let values = [
        (
            "host.rust.alloc.bytes",
            after.allocated - before.allocated,
            "bytes",
        ),
        (
            "host.rust.alloc.count",
            after.allocations - before.allocations,
            "count",
        ),
        ("host.rust.freed.bytes", after.freed - before.freed, "bytes"),
        ("host.rust.outstanding.start", before.outstanding, "bytes"),
        ("host.rust.outstanding.end", after.outstanding, "bytes"),
        ("host.rust.outstanding.observed_peak", after.peak, "bytes"),
    ];
    if values
        .iter()
        .any(|(_, value, _)| *value > 9_007_199_254_740_991)
    {
        bail!("Rust allocator values exceed exact JSON-number budget");
    }
    Ok(values.into_iter().map(|(metric,value,unit)| json!({"metric":metric,"definition_version":1,"value":value,"unit":unit,"scope":"allocations_routed_through_rust_global_allocator","phase":phase,"collector":"wasmbench Rust GlobalAlloc/System","collector_version":"rust-global-alloc-v1","quality":"instrumented","profile":"memory","status":"available","normalization_denominator":"single_embedding_api_operation"})).collect())
}

pub fn decorate(response: &mut Value) {
    let d = &mut response["description"];
    d["build"] = json!("Cargo.lock / release / native-allocator feature; diagnostic-only binary");
    d["capabilities"] = json!({"can_compile_separately":true,"can_instantiate_separately":true,"can_disable_code_cache":true,"can_measure_host_allocations":true,"requires_memory_profile":true,"can_observe_tiers":false,"can_export_native_code":false,"can_snapshot":false});
    d["scenarios"] = json!(["compile", "instantiate", "first-call", "steady"]);
    d["abis"] = json!(["core"]);
    d["phase_barrier_scenarios"] = json!(["compile", "instantiate", "first-call", "steady"]);
    d["capabilities"]["can_rust_allocator_release_windows"] = json!(true);
    d["capabilities"]["can_verify_float_bits_v1"] = json!(true);
    d["capabilities"]["can_float_phases"] = json!(true);
    d["capabilities"]["can_rust_allocator_phase_boundaries"] = json!(true);
    d["effective_configuration"]["allocator_phase_boundary_protocol"] =
        json!("rust-allocator-boundaries-v1");
    d["effective_configuration"]["allocator_release_policy"] = json!(
        "rust-logical-release-v1; after correctness verification: compile drops measured Module with engine retained; instantiate/first-call drop measured Store/Instance state (including argument/result buffers) and prepared import handles with module/engine retained; steady retains Store across samples and drops it on final sample with module/engine retained; counters exclude observation encoding and output buffering; no allocator purge or physical reclamation claim"
    );
    d["effective_configuration"]["allocator_instrumentation"] = json!(
        "rust-global-alloc-v1; successful layout requests routed through Rust GlobalAlloc/System across all threads; realloc counts new request and old logical release even in-place; excludes mmap/foreign allocators/usable sizes; serialized accounting update high-water, not physical peak; no forced purge; memory-only"
    );
}

fn release_observations(
    window: Option<(Counts, Counts, u64)>,
    scenario: &str,
) -> Result<Vec<Value>> {
    let phase = format!("{scenario}/logical_release_window");
    let mut values = match window {
        Some((before, after, elapsed)) => {
            if elapsed > 9_007_199_254_740_991 { bail!("release timer exceeds exact JSON-number budget"); }
            let mut values = observations(before, after, &phase)?;
            values.push(json!({"metric":"host.rust.elapsed","definition_version":1,"value":elapsed,"unit":"ns","scope":"allocations_routed_through_rust_global_allocator","phase":phase,"collector":"wasmbench Rust GlobalAlloc/System","collector_version":"rust-global-alloc-v1","quality":"instrumented","profile":"memory","status":"available","normalization_denominator":"single_embedding_api_operation"}));
            values
        }
        None => ["alloc.bytes","alloc.count","freed.bytes","outstanding.start","outstanding.end","outstanding.observed_peak","elapsed"].into_iter().map(|suffix| json!({"metric":format!("host.rust.{suffix}"),"definition_version":1,"unit":if suffix=="elapsed" {"ns"} else if suffix=="alloc.count" {"count"} else {"bytes"},"scope":"allocations_routed_through_rust_global_allocator","phase":phase,"collector":"wasmbench Rust GlobalAlloc/System","collector_version":"rust-global-alloc-v1","quality":"instrumented","profile":"memory","status":"not_applicable","reason":"steady Store retained across samples; release on final sample","normalization_denominator":"single_embedding_api_operation"})).collect(),
    };
    for value in &mut values {
        value["metric"] = json!(value["metric"].as_str().unwrap().replacen(
            "host.rust.",
            "host.rust.release.",
            1
        ));
        value["normalization_denominator"] = json!("single_logical_release_operation");
    }
    Ok(values)
}

pub fn run(
    adapter: &Adapter,
    r: &Value,
    barrier: &mut dyn FnMut(u64, &str) -> Result<()>,
) -> Result<Value> {
    let prep = adapter
        .prep
        .as_ref()
        .ok_or_else(|| anyhow!("prepare required"))?;
    let w = &prep["workload"];
    let scenario = r["scenario"].as_str().unwrap_or("");
    let samples = r["samples"].as_u64().unwrap_or(0);
    let warmup = r["warmup"].as_u64().unwrap_or(0);
    let phased = r["phase_barriers"] == true;
    if prep["profile"] != "memory"
        || w["abi"] != "core"
        || !["exact_u64", "float_bits_v1"].contains(&w["oracle"]["kind"].as_str().unwrap_or(""))
        || !(w["reset"] == "stateless"
            || (w["reset"] == "fresh_instance_per_sample" && scenario != "steady"))
        || !w["command"].is_null()
        || !w["vectors"].is_null()
        || !w["density"].is_null()
        || !w["checkpoint"].is_null()
        || !w["guest_density"].is_null()
        || samples == 0
        || samples > 10000
        || warmup != 0
        || r["operations"] != 1
        || !["compile", "instantiate", "first-call", "steady"].contains(&scenario)
    {
        return Ok(
            json!({"status":"unsupported","reason":"Rust allocator diagnostics require exact/float core workload, memory profile, single operations, no warmup; steady requires stateless reset, other stages allow fresh_instance_per_sample"}),
        );
    }
    let stages = match scenario {
        "compile" => [
            "before_compile",
            "compiled",
            "compile_release_entry",
            "compile_release_completed",
        ],
        "instantiate" => [
            "before_instantiate",
            "instantiated",
            "instantiate_release_entry",
            "instantiate_release_completed",
        ],
        "first-call" => [
            "before_first_call",
            "first_call_returned",
            "first_call_release_entry",
            "first_call_release_completed",
        ],
        "steady" => [
            "before_steady_batch",
            "steady_batch_returned",
            "steady_release_entry",
            "steady_release_decision_completed",
        ],
        _ => unreachable!(),
    };
    let engine = adapter.engine()?;
    let module = if scenario == "compile" {
        None
    } else {
        Some(Module::new(&engine, &adapter.bytes)?)
    };
    let mut held = if scenario == "steady" {
        Some(adapter.prepare_state(&engine, module.as_ref().unwrap())?)
    } else {
        None
    };
    let mut output = Vec::with_capacity(samples as usize);
    for index in 0..samples {
        // All setup and output buffer allocation occurs before the counter window.
        let mut state = if scenario == "first-call" {
            Some(adapter.prepare_state(&engine, module.as_ref().unwrap())?)
        } else {
            None
        };
        let mut store = if scenario == "instantiate" {
            Some(Store::new(&engine, ()))
        } else {
            None
        };
        let imports = if let Some(store) = &mut store {
            adapter.imports(store, module.as_ref().unwrap())?
        } else {
            vec![]
        };
        if phased {
            barrier(index, stages[0])?;
        }
        let before = TRACKER.start();
        let start = Instant::now();
        let operation = (|| -> Result<(Option<Module>, Option<Instance>)> {
            match scenario {
                "compile" => Ok((Some(Module::new(&engine, &adapter.bytes)?), None)),
                "instantiate" => Ok((
                    None,
                    Some(Instance::new(
                        store.as_mut().unwrap(),
                        module.as_ref().unwrap(),
                        &imports,
                    )?),
                )),
                _ => {
                    let s = if scenario == "steady" {
                        held.as_mut().unwrap()
                    } else {
                        state.as_mut().unwrap()
                    };
                    s.function.call(&mut s.store, &s.args, &mut s.results)?;
                    Ok((None, None))
                }
            }
        })();
        let elapsed = start.elapsed().as_nanos() as u64;
        let after = TRACKER.finish(); // Always close the window, including traps/errors.
        let (mut compiled, instance) = operation?;
        if phased {
            barrier(index, stages[1])?;
        }
        let mut result = vec![];
        if let Some(m) = compiled.as_ref() {
            adapter.check(&engine, m)?;
        } else if let Some(instance) = instance {
            state = Some(adapter.resolve_state(store.take().unwrap(), instance)?);
            let s = state.as_mut().unwrap();
            s.function.call(&mut s.store, &s.args, &mut s.results)?;
            let values = s.results.clone();
            result = adapter.verify(s, &values)?;
        } else {
            let s = if scenario == "steady" {
                held.as_mut().unwrap()
            } else {
                state.as_mut().unwrap()
            };
            let values = s.results.clone();
            result = adapter.verify(s, &values)?;
        }
        // Correctness and its temporary result copies have finished. The API
        // output remains held until this separate logical-resource drop window.
        // Keep counter snapshots on the stack; JSON encoding cannot pollute it.
        if phased {
            barrier(index, stages[2])?;
        }
        let release = if scenario != "steady" || index + 1 == samples {
            let release_before = TRACKER.start();
            let release_start = Instant::now();
            drop(compiled.take());
            drop(state.take());
            drop(store.take());
            drop(imports);
            if scenario == "steady" {
                drop(held.take());
            }
            let release_elapsed = release_start.elapsed().as_nanos() as u64;
            let release_after = TRACKER.finish();
            Some((release_before, release_after, release_elapsed))
        } else {
            None
        };
        if phased {
            barrier(index, stages[3])?;
        }
        let mut obs = observations(before, after, scenario)?;
        obs.extend(release_observations(release, scenario)?);
        output.push(json!({"index":index,"warmup":false,"elapsed_ns":elapsed,"operations":1,"sample_type":"individual_operation","verified":true,"result":result,"observations":obs}));
    }
    Ok(json!({"samples":output}))
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn barrier_transport_is_outside_both_allocator_windows() -> Result<()> {
        let bytes = hex::decode(
            "0061736d010000000105016000017f03020100070d010962656e63686d61726b00000a0601040041070b",
        )?;
        for winch in [false, true] {
            let adapter = Adapter {
                bytes: bytes.clone(),
                winch,
                prep: Some(
                    json!({"profile":"memory","workload":{"abi":"core","reset":"stateless","export":"benchmark","args":[],"oracle":{"kind":"exact_u64","expected":[7]}}}),
                ),
            };
            for scenario in ["compile", "instantiate", "first-call", "steady"] {
                for phased in [false, true] {
                    let mut events = Vec::with_capacity(8);
                    let response = run(
                        &adapter,
                        &json!({"scenario":scenario,"samples":2,"operations":1,"warmup":0,"phase_barriers":phased}),
                        &mut |index, stage| {
                            assert!(
                                !TRACKER.with(|counts| counts.active),
                                "transport entered allocator window"
                            );
                            // Model actual JSON/ack transport allocation and destruction.
                            std::hint::black_box(vec![0u8; 4096]);
                            events.push((index, stage.to_owned()));
                            Ok(())
                        },
                    )?;
                    assert_eq!(events.len(), if phased { 8 } else { 0 });
                    assert_eq!(response["samples"].as_array().unwrap().len(), 2);
                    assert_eq!(response["samples"][0]["verified"], true);
                    assert!(!TRACKER.with(|counts| counts.active));
                    if phased && scenario == "steady" {
                        assert_eq!(events[3].1, "steady_release_decision_completed");
                        assert_eq!(
                            response["samples"][0]["observations"][12]["status"],
                            "not_applicable"
                        );
                        assert_eq!(
                            response["samples"][1]["observations"][12]["status"],
                            "available"
                        );
                    }
                }
            }
        }
        Ok(())
    }
    #[test]
    fn release_window_and_retention_are_separate_domains() -> Result<()> {
        let tracker = Tracker::new();
        tracker.update(100, 0, true);
        let before = tracker.start();
        tracker.update(0, 60, false);
        let after = tracker.finish();
        let released = release_observations(Some((before, after, 0)), "compile")?;
        assert_eq!(released.len(), 7);
        assert_eq!(released[0]["metric"], "host.rust.release.alloc.bytes");
        assert_eq!(released[2]["value"], 60);
        assert_eq!(released[4]["value"], 40);
        assert_eq!(released[6]["value"], 0);
        for o in released {
            assert_eq!(o["phase"], "compile/logical_release_window");
            assert_eq!(
                o["normalization_denominator"],
                "single_logical_release_operation"
            );
        }
        let retained = release_observations(None, "steady")?;
        assert_eq!(retained.len(), 7);
        for o in retained {
            assert_eq!(o["status"], "not_applicable");
            assert!(o.get("value").is_none());
        }
        Ok(())
    }
    #[test]
    fn concurrent_accounting_keeps_all_worker_requests() {
        let tracker = std::sync::Arc::new(Tracker::new());
        let before = tracker.start();
        let workers: Vec<_> = (0..8)
            .map(|_| {
                let tracker = tracker.clone();
                std::thread::spawn(move || {
                    for _ in 0..1000 {
                        tracker.update(32, 0, true);
                        tracker.update(0, 32, false);
                    }
                })
            })
            .collect();
        for worker in workers {
            worker.join().unwrap();
        }
        let after = tracker.finish();
        assert_eq!(after.allocated - before.allocated, 256000);
        assert_eq!(after.allocations - before.allocations, 8000);
        assert_eq!(after.freed - before.freed, 256000);
        assert_eq!(after.outstanding, 0);
        assert!((32..=256).contains(&after.peak));
        assert!(!after.overflow);
    }
    #[test]
    fn accounting_tracks_reallocation_and_logical_release() {
        let tracker = Tracker::new();
        tracker.update(100, 0, true);
        let before = tracker.start();
        tracker.update(200, 100, true);
        tracker.update(0, 200, false);
        let after = tracker.finish();
        assert_eq!(after.allocated - before.allocated, 200);
        assert_eq!(after.allocations - before.allocations, 1);
        assert_eq!(after.freed - before.freed, 300);
        assert_eq!(after.outstanding, 0);
        assert_eq!(after.peak, 200);
        assert!(!after.overflow);
    }
    #[test]
    fn inconsistent_and_overflow_counters_are_unavailable_not_zero() {
        let tracker = Tracker::new();
        let before = tracker.start();
        tracker.update(0, 1, false);
        assert!(observations(before, tracker.finish(), "compile").is_err());
        let tracker = Tracker::new();
        tracker.update(u64::MAX, 0, true);
        let before = tracker.start();
        tracker.update(1, 0, true);
        assert!(observations(before, tracker.finish(), "compile").is_err());
    }
}
