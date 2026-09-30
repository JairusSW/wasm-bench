//! Typed, host-import-free Component Model exports. This is not a WASI world.
use serde_json::{Value, json};
use wasmtime::{
    Engine, Result, Store, bail,
    component::{Component, Linker, Type, Val},
    ensure, format_err,
};

pub const POLICY: &str = "component-u64-v1";
pub const BOUNDARIES: &str = "host-import-free named component export with 0..16 u64 parameters and one u64 result; fresh Store/instance per sample; compile: resident bytes through Component::new including validation; instantiate: empty linker and prepared Store through instantiate including nested core start; first-call: checked Func::call and post_return; signature lookup and exact oracle verification outside timers; one operation; separate timing/memory passes; memory-only before/returned/released barriers; release drops Store and compile-owned Component outside timers, engine/linker and shared compiled component retained; no forced allocator reclamation; no counters/profiling or arbitrary WIT support";

pub fn validate(w: &Value) -> Result<()> {
    ensure!(
        w["abi"] == "component"
            && w["host_profile"] == POLICY
            && w["oracle"]["kind"] == "exact_u64"
            && w["reset"] == "fresh_instance_per_sample",
        "invalid component-u64-v1 contract"
    );
    ensure!(
        w["export"]
            .as_str()
            .is_some_and(|s| !s.is_empty() && s.len() <= 256),
        "component export required"
    );
    ensure!(
        w["work_unit"].as_str().is_some_and(|s| !s.is_empty())
            && w["units_per_invocation"].as_u64().is_some_and(|n| n > 0),
        "component work denominator required"
    );
    for field in [
        "command",
        "input",
        "vectors",
        "density",
        "checkpoint",
        "guest_density",
        "continuation",
        "process_snapshot",
        "snapshot_density",
    ] {
        ensure!(w[field].is_null(), "component-u64-v1 disallows {field}");
    }
    ensure!(
        w["initialize"].as_str().is_none_or(|s| s.is_empty()),
        "component initialization unsupported"
    );
    ensure!(
        w["oracle"]["float"].is_null()
            && w["oracle"]["expected_trap"]
                .as_str()
                .is_none_or(|s| s.is_empty())
            && w["oracle"]["output_pointer_export"]
                .as_str()
                .is_none_or(|s| s.is_empty()),
        "component oracle extensions unsupported"
    );
    let args = w["args"]
        .as_array()
        .ok_or_else(|| format_err!("component arguments required"))?;
    ensure!(args.len() <= 16, "too many component arguments");
    for arg in args {
        super::bits(arg)?;
    }
    let expected = w["oracle"]["expected"]
        .as_array()
        .ok_or_else(|| format_err!("component oracle required"))?;
    ensure!(expected.len() == 1, "one u64 component result required");
    super::bits(&expected[0])?;
    Ok(())
}

pub fn run(
    engine: &Engine,
    bytes: &[u8],
    prep: &Value,
    request: &Value,
    barrier: &mut dyn FnMut(u64, &str) -> Result<()>,
) -> Result<Value> {
    let w = &prep["workload"];
    validate(w)?;
    let scenario = request["scenario"].as_str().unwrap_or("");
    if !["compile", "instantiate", "first-call"].contains(&scenario)
        || !["timing", "memory"].contains(&prep["profile"].as_str().unwrap_or(""))
        || request["operations"] != 1
        || (request["phase_barriers"] == true && prep["profile"] != "memory")
        || request["warmup"].as_u64().unwrap_or(0) != 0
    {
        return Ok(
            json!({"status":"unsupported","reason":"component-u64-v1 requires one-operation timing/memory lifecycle without warmup; barriers require memory"}),
        );
    }
    let count = request["samples"].as_u64().unwrap_or(0);
    ensure!(
        count > 0 && count <= 100_000,
        "invalid component sample count"
    );
    let params: Vec<Val> = w["args"]
        .as_array()
        .unwrap()
        .iter()
        .map(|v| super::bits(v).map(Val::U64))
        .collect::<Result<_>>()?;
    let expected = super::bits(&w["oracle"]["expected"][0])?;
    let linker = Linker::<()>::new(engine); // Empty: imports cannot inherit ambient capabilities.
    let retained = if scenario == "compile" {
        None
    } else {
        Some(Component::new(engine, bytes)?)
    };
    let mut output = Vec::with_capacity(count as usize);
    let phased = request["phase_barriers"] == true;
    let stages = match scenario {
        "compile" => ["before_compile", "compiled", "released"],
        "instantiate" => ["before_instantiate", "instantiated", "instance_released"],
        _ => [
            "before_first_call",
            "first_call_returned",
            "first_call_released",
        ],
    };
    for index in 0..count {
        let mut elapsed = 0;
        let mut store = Store::new(engine, ());
        let mut compiled = None;
        let component = if let Some(component) = retained.as_ref() {
            component
        } else {
            if phased {
                barrier(index, stages[0])?;
            }
            let start = std::time::Instant::now();
            compiled = Some(Component::new(engine, bytes)?);
            elapsed = start.elapsed().as_nanos();
            if phased {
                barrier(index, stages[1])?;
            }
            compiled.as_ref().unwrap()
        };
        if phased && scenario == "instantiate" {
            barrier(index, stages[0])?;
        }
        let start = std::time::Instant::now();
        let instance = linker.instantiate(&mut store, component)?;
        if scenario == "instantiate" {
            elapsed = start.elapsed().as_nanos();
            if phased {
                barrier(index, stages[1])?;
            }
        }
        let func = instance
            .get_func(&mut store, w["export"].as_str().unwrap())
            .ok_or_else(|| format_err!("component export missing"))?;
        let ty = func.ty(&store);
        ensure!(
            ty.params().len() == params.len()
                && ty.params().all(|(_, t)| matches!(t, Type::U64))
                && ty.results().len() == 1
                && ty.results().all(|t| matches!(t, Type::U64)),
            "component signature must match u64 arguments and one u64 result"
        );
        let mut results = [Val::U64(0)];
        if phased && scenario == "first-call" {
            barrier(index, stages[0])?;
        }
        let start = std::time::Instant::now();
        func.call(&mut store, &params, &mut results)?;
        #[allow(deprecated)]
        func.post_return(&mut store)?;
        if scenario == "first-call" {
            elapsed = start.elapsed().as_nanos();
            if phased {
                barrier(index, stages[1])?;
            }
        }
        let Val::U64(actual) = results[0] else {
            bail!("non-u64 component result")
        };
        ensure!(
            actual == expected,
            "component oracle mismatch: expected {expected}, got {actual}"
        );
        drop(store);
        drop(compiled);
        if phased {
            barrier(index, stages[2])?;
        }
        output.push(json!({"index":index,"warmup":false,"elapsed_ns":u64::try_from(elapsed).unwrap_or(u64::MAX),"operations":1,"sample_type":"individual_operation","verified":true,"result":[actual.to_string()]}));
    }
    Ok(json!({"samples":output}))
}

#[cfg(all(test, feature = "component-fixtures"))]
mod tests {
    use super::*;
    fn run_unphased(engine: &Engine, bytes: &[u8], prep: &Value, request: &Value) -> Result<Value> {
        run(engine, bytes, prep, request, &mut |_, _| {
            panic!("unexpected phase barrier")
        })
    }
    fn prep() -> Value {
        json!({"profile":"timing","workload":{"abi":"component","host_profile":POLICY,"oracle":{"kind":"exact_u64","expected":["8"]},"reset":"fresh_instance_per_sample","export":"benchmark","args":["7"],"work_unit":"invocation","units_per_invocation":1}})
    }
    fn engine(winch: bool) -> Engine {
        let mut config = wasmtime::Config::new();
        config
            .wasm_component_model(true)
            .parallel_compilation(false)
            .strategy(if winch {
                wasmtime::Strategy::Winch
            } else {
                wasmtime::Strategy::Cranelift
            });
        Engine::new(&config).unwrap()
    }
    #[test]
    fn typed_calls_reset_and_verify_every_lifecycle_sample() {
        let bytes =
            wat::parse_str(include_str!("../../../recipes/fixtures/component-u64.wat")).unwrap();
        for winch in [false, true] {
            let engine = engine(winch);
            for scenario in ["compile", "instantiate", "first-call"] {
                let request = json!({"scenario":scenario,"operations":1,"samples":3,"warmup":0});
                let result = run_unphased(&engine, &bytes, &prep(), &request).unwrap();
                let samples = result["samples"].as_array().unwrap();
                assert_eq!(samples.len(), 3);
                assert!(
                    samples
                        .iter()
                        .all(|s| s["verified"] == true && s["result"] == json!(["8"]))
                );
            }
            for mode in [
                "wrong-type",
                "trap",
                "missing",
                "wrong-oracle",
                "wrong-arity",
            ] {
                let mut p = prep();
                match mode {
                    "wrong-oracle" => p["workload"]["oracle"]["expected"] = json!(["9"]),
                    "wrong-arity" => p["workload"]["args"] = json!([]),
                    _ => p["workload"]["export"] = json!(mode),
                }
                assert!(
                    run_unphased(
                        &engine,
                        &bytes,
                        &p,
                        &json!({"scenario":"first-call","operations":1,"samples":1})
                    )
                    .is_err(),
                    "{mode} accepted"
                );
            }
            let mut p = prep();
            p["workload"]["args"] = json!([u64::MAX.to_string()]);
            p["workload"]["oracle"]["expected"] = json!(["0"]);
            assert_eq!(
                run_unphased(
                    &engine,
                    &bytes,
                    &p,
                    &json!({"scenario":"first-call","operations":1,"samples":1})
                )
                .unwrap()["samples"][0]["result"],
                json!(["0"])
            );
        }
    }
    #[test]
    fn unsupported_requests_do_not_run_guest_code() {
        let engine = engine(false);
        for request in [
            json!({"scenario":"steady","operations":1,"samples":1}),
            json!({"scenario":"compile","operations":2,"samples":1}),
            json!({"scenario":"compile","operations":1,"samples":1,"phase_barriers":true}),
            json!({"scenario":"compile","operations":1,"samples":1,"warmup":1}),
        ] {
            assert_eq!(
                run_unphased(&engine, b"not wasm", &prep(), &request).unwrap()["status"],
                "unsupported"
            );
        }
    }

    #[test]
    fn unresolved_component_imports_never_receive_host_capabilities() {
        let source=include_str!("../../../recipes/fixtures/component-u64.wat").replacen("(component", "(component (type $host (func (param \"value\" u64) (result u64))) (import \"ambient\" (func (type $host)))",1);
        let bytes = wat::parse_str(source).unwrap();
        let result = run_unphased(
            &engine(false),
            &bytes,
            &prep(),
            &json!({"scenario":"first-call","operations":1,"samples":1}),
        );
        assert!(result.is_err(), "unresolved host import accepted");
    }

    #[test]
    fn memory_boundaries_cover_every_sample_on_both_backends() {
        let bytes =
            wat::parse_str(include_str!("../../../recipes/fixtures/component-u64.wat")).unwrap();
        let mut p = prep();
        p["profile"] = json!("memory");
        for winch in [false, true] {
            let engine = engine(winch);
            for (scenario, stages) in [
                ("compile", ["before_compile", "compiled", "released"]),
                (
                    "instantiate",
                    ["before_instantiate", "instantiated", "instance_released"],
                ),
                (
                    "first-call",
                    [
                        "before_first_call",
                        "first_call_returned",
                        "first_call_released",
                    ],
                ),
            ] {
                let mut events = Vec::new();
                let request =
                    json!({"scenario":scenario,"operations":1,"samples":3,"phase_barriers":true});
                let result = run(&engine, &bytes, &p, &request, &mut |index, stage| {
                    events.push((index, stage.to_owned()));
                    Ok(())
                })
                .unwrap();
                let expected: Vec<_> = (0..3)
                    .flat_map(|i| stages.map(|s| (i, s.to_owned())))
                    .collect();
                assert_eq!(events, expected);
                assert!(
                    result["samples"]
                        .as_array()
                        .unwrap()
                        .iter()
                        .all(|s| s["verified"] == true && s["result"] == json!(["8"]))
                );
                let mut unphased = request.clone();
                unphased["phase_barriers"] = json!(false);
                assert_eq!(
                    run_unphased(&engine, &bytes, &p, &unphased).unwrap()["samples"]
                        .as_array()
                        .unwrap()
                        .len(),
                    3
                );
            }
        }
    }

    #[test]
    fn failed_memory_barriers_and_oracles_preserve_only_completed_prefixes() {
        let bytes =
            wat::parse_str(include_str!("../../../recipes/fixtures/component-u64.wat")).unwrap();
        let engine = engine(false);
        let mut p = prep();
        p["profile"] = json!("memory");
        for (scenario, stages) in [
            ("compile", ["before_compile", "compiled", "released"]),
            (
                "instantiate",
                ["before_instantiate", "instantiated", "instance_released"],
            ),
            (
                "first-call",
                [
                    "before_first_call",
                    "first_call_returned",
                    "first_call_released",
                ],
            ),
        ] {
            let request =
                json!({"scenario":scenario,"operations":1,"samples":2,"phase_barriers":true});
            for stop in 0..3 {
                let mut events = Vec::new();
                let result = run(&engine, &bytes, &p, &request, &mut |index, stage| {
                    events.push((index, stage.to_owned()));
                    ensure!(stage != stages[stop], "collector rejected boundary");
                    Ok(())
                });
                assert!(
                    result
                        .unwrap_err()
                        .to_string()
                        .contains("collector rejected boundary")
                );
                assert_eq!(
                    events,
                    stages[..=stop]
                        .iter()
                        .map(|s| (0, s.to_string()))
                        .collect::<Vec<_>>()
                );
            }
            let mut wrong = p.clone();
            wrong["workload"]["oracle"]["expected"] = json!(["9"]);
            let mut events = Vec::new();
            assert!(
                run(&engine, &bytes, &wrong, &request, &mut |_, stage| {
                    events.push(stage.to_owned());
                    Ok(())
                })
                .is_err()
            );
            assert_eq!(events, stages[..2]);
        }
        let request =
            json!({"scenario":"compile","operations":1,"samples":1,"phase_barriers":true});
        let err = run(&engine, b"not wasm", &p, &request, &mut |_, _| {
            bail!("before compile rejected")
        })
        .unwrap_err();
        assert_eq!(err.to_string(), "before compile rejected");
        for profile in ["counters", "profiling", "code"] {
            p["profile"] = json!(profile);
            assert_eq!(
                run_unphased(&engine, b"not wasm", &p, &request).unwrap()["status"],
                "unsupported"
            );
        }
    }
}
