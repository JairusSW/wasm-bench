use std::{env, path::PathBuf};

fn main() {
    let selected: Vec<_> = [
        "wasmi",
        "wavm",
        "wamr",
        "wasm3",
        "wasmedge",
        "wasmer_llvm",
        "wasmer_singlepass",
    ]
    .into_iter()
    .filter(|x| env::var_os(format!("CARGO_FEATURE_{}", x.to_uppercase())).is_some())
    .collect();
    assert_eq!(
        selected.len(),
        1,
        "select exactly one native runtime feature"
    );
    let runtime = selected[0];
    println!("cargo:rustc-env=WB_RUNTIME={}", runtime.replace('_', "-"));
    let wasmer = runtime.starts_with("wasmer_");
    let sdk_runtime = if wasmer { "wasmer" } else { runtime };
    if runtime == "wasmi" {
        return;
    }
    let key = format!("WASMBENCH_{}_SDK", sdk_runtime.to_uppercase());
    println!("cargo:rerun-if-env-changed={key}");
    let sdk = PathBuf::from(env::var(&key).unwrap_or_else(|_| {
        panic!("set {key} to an installed runtime SDK prefix (include/ and lib/)")
    }))
    .canonicalize()
    .expect("SDK prefix must exist");
    let mut build = cc::Build::new();
    build
        .cpp(true)
        .std("c++17")
        .include(sdk.join("include"))
        .file(format!("src/{sdk_runtime}.cpp"));
    if wasmer {
        build.define(
            "WB_WASMER_LLVM",
            Some(if runtime == "wasmer_llvm" { "1" } else { "0" }),
        );
    }
    let version_key = format!("WASMBENCH_{}_VERSION", runtime.to_uppercase());
    println!("cargo:rerun-if-env-changed={version_key}");
    if runtime != "wasmedge" && !wasmer {
        let version = env::var(&version_key).unwrap_or_else(|_| {
            panic!("set {version_key} to the selected SDK version or source commit")
        });
        assert!(
            !version.is_empty()
                && version
                    .chars()
                    .all(|x| x.is_ascii_alphanumeric() || ".-_+".contains(x)),
            "invalid SDK version"
        );
        build.define("WB_VERSION", Some(format!("\"{version}\"").as_str()));
    }
    build.compile("wb_embedding");
    println!("cargo:rerun-if-changed=src/{sdk_runtime}.cpp");
    println!("cargo:rerun-if-changed=src/embedding.h");
    println!(
        "cargo:rustc-link-search=native={}",
        sdk.join("lib").display()
    );
    println!(
        "cargo:rustc-link-lib={}",
        match sdk_runtime {
            "wasmer" => "wasmer",
            "wavm" => "WAVM",
            "wamr" => "iwasm",
            "wasm3" => "m3",
            _ => "wasmedge",
        }
    );
    if runtime == "wasm3" {
        println!("cargo:rustc-link-lib=m");
    }
    if env::var("CARGO_CFG_TARGET_FAMILY").as_deref() == Ok("unix") {
        println!(
            "cargo:rustc-link-arg=-Wl,-rpath,{}",
            sdk.join("lib").display()
        );
    }
}
