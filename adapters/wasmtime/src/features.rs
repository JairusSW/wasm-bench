//! Explicit subset of the pinned engine's default validator policy. This is
//! not a promise that every instruction has native-code support on every ISA.
use serde_json::{Value, json};
use wasmtime::{Config, WasmFeatures};

fn policy(winch: bool) -> Vec<(&'static str, WasmFeatures, bool)> {
    let mut flags = vec![
        ("TAIL_CALL", WasmFeatures::TAIL_CALL, !winch),
        ("RELAXED_SIMD", WasmFeatures::RELAXED_SIMD, !winch),
        ("GC", WasmFeatures::GC, false),
        (
            "FUNCTION_REFERENCES",
            WasmFeatures::FUNCTION_REFERENCES,
            false,
        ),
        ("EXCEPTIONS", WasmFeatures::EXCEPTIONS, false),
        ("LEGACY_EXCEPTIONS", WasmFeatures::LEGACY_EXCEPTIONS, false),
    ];
    if winch {
        flags.push(("GC_TYPES", WasmFeatures::GC_TYPES, false));
        flags.push(("STACK_SWITCHING", WasmFeatures::STACK_SWITCHING, false));
        if cfg!(target_arch = "aarch64") {
            flags.push(("THREADS", WasmFeatures::THREADS, false));
        }
    }
    flags
}

pub fn configure(config: &mut Config, winch: bool) {
    for (_, flag, enabled) in policy(winch) {
        config.wasm_features(flag, enabled);
    }
}

pub fn describe(winch: bool) -> Value {
    let supported: serde_json::Map<String, Value> = policy(winch)
        .into_iter()
        .map(|(name, _, enabled)| (name.to_owned(), json!(enabled)))
        .collect();
    json!({
        "namespace": "wasmparser/0.251.0",
        "evidence": "Wasmtime 46.0.1 config.rs default features and compiler_panicking_wasm_features; explicitly applied Config::wasm_features subset; enabled validation is not a guarantee of complete backend instruction support",
        "supported": supported
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::Adapter;
    use wasmtime::{Instance, Module, Store};

    #[test]
    fn advertised_policy_matches_configured_engines() {
        let tail = include_bytes!("../../../corpus/testdata/feature-tail-call.wasm");
        let simd = include_bytes!("../../../corpus/testdata/analyzer-features.wasm");
        for winch in [false, true] {
            let adapter = Adapter {
                prep: None,
                bytes: vec![],
                winch,
            };
            let engine = adapter.engine().unwrap();
            assert_eq!(describe(winch)["supported"]["TAIL_CALL"], json!(!winch));
            assert_eq!(Module::validate(&engine, tail).is_ok(), !winch);
            assert!(Module::validate(&engine, simd).is_ok());
            for bytes in [tail.as_slice(), simd.as_slice()] {
                if winch && bytes == tail {
                    continue;
                }
                let module = Module::new(&engine, bytes).unwrap();
                let mut store = Store::new(&engine, ());
                let instance = Instance::new(&mut store, &module, &[]).unwrap();
                let result = instance
                    .get_typed_func::<(), i32>(&mut store, "benchmark")
                    .unwrap()
                    .call(&mut store, ())
                    .unwrap();
                assert_eq!(result, 7);
            }
        }
    }
}
