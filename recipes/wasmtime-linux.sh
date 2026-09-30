#!/bin/sh
# Pinned Linux build; source is read-only and credentials are not mounted.
set -eu
action=${1:-build}
case "$action" in build|test|test-code-lifetime|build-code-lifetime|build-process-snapshot) ;; *) printf 'usage: %s [build|test|test-code-lifetime|build-code-lifetime|build-process-snapshot]\n' "$0" >&2; exit 2;; esac
if [ "$action" = test-code-lifetime ]; then
  set -- test --release --locked --features native-code-lifetime --bin adapter-wasmtime code_lifetime
elif [ "$action" = build-code-lifetime ]; then
  set -- build --release --locked --features native-code-lifetime --bin adapter-wasmtime --target-dir /target/code-lifetime
elif [ "$action" = build-process-snapshot ]; then
  set -- build --release --locked --features process-snapshot --bin qualify-process-snapshot --target-dir /target/process-snapshot
else
  set -- "$action" --release --locked --bin adapter-wasmtime --bin wasm-analyze
fi
recipe_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
mkdir -p "$recipe_root/.wasmbench/wasmtime-linux-target" "$recipe_root/.wasmbench/cargo-registry"
docker run --rm \
  --mount "type=bind,src=$recipe_root/adapters/wasmtime,dst=/src,readonly" \
  --mount "type=bind,src=$recipe_root/adapters/wazero/testdata,dst=/wazero/testdata,readonly" \
  --mount "type=bind,src=$recipe_root/corpus/testdata,dst=/corpus/testdata,readonly" \
  --mount "type=bind,src=$recipe_root/.wasmbench/wasmtime-linux-target,dst=/target" \
  --mount "type=bind,src=$recipe_root/.wasmbench/cargo-registry,dst=/usr/local/cargo/registry" \
  --workdir /src -e CARGO_TARGET_DIR=/target \
  rust:1.98.1-bookworm@sha256:93ce27a88655056a51dbdd8f5f2d7ddc071c7b0070fb288a37b5a285fc83971e \
  cargo "$@"
if [ "$action" = build-process-snapshot ]; then
  printf 'Linux process snapshot qualifier: %s/.wasmbench/wasmtime-linux-target/process-snapshot/release/qualify-process-snapshot\n' "$recipe_root"
elif [ "$action" = build-code-lifetime ]; then
  printf 'Linux code lifetime adapter: %s/.wasmbench/wasmtime-linux-target/code-lifetime/release/adapter-wasmtime\n' "$recipe_root"
elif [ "$action" = build ]; then
  printf 'Linux adapter: %s/.wasmbench/wasmtime-linux-target/release/adapter-wasmtime\n' "$recipe_root"
  printf 'Linux analyzer: %s/.wasmbench/wasmtime-linux-target/release/wasm-analyze\n' "$recipe_root"
elif [ "$action" = test-code-lifetime ]; then
  printf 'Linux native code publication/lifetime qualification passed.\n'
else
  printf 'Linux adapter and analyzer tests passed.\n'
fi
