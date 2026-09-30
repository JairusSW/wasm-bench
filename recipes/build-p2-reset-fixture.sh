#!/bin/sh
# Pinned standalone fixture build; only source and the generated cache are mounted.
set -eu
recipe_root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
out="$recipe_root/.wasmbench/p2-reset-fixture"
mkdir -p "$out"
docker run --rm \
  --mount "type=bind,src=$recipe_root/corpus/testdata/p2-filesystem-reset.rs,dst=/src/main.rs,readonly" \
  --mount "type=bind,src=$out,dst=/out" \
  --entrypoint sh \
  rust:1.98.1-bookworm@sha256:93ce27a88655056a51dbdd8f5f2d7ddc071c7b0070fb288a37b5a285fc83971e \
  -eu -c 'rustup target add wasm32-wasip2; rustc --edition 2024 --target wasm32-wasip2 -O /src/main.rs -o /out/p2-filesystem-reset.component.wasm; rustc -vV > /out/compiler.txt; sha256sum /src/main.rs /out/p2-filesystem-reset.component.wasm > /out/checksums.sha256'
printf 'Preview 2 filesystem reset fixture: %s/p2-filesystem-reset.component.wasm\n' "$out"
