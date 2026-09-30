FROM rust:1.98.1-bookworm@sha256:93ce27a88655056a51dbdd8f5f2d7ddc071c7b0070fb288a37b5a285fc83971e AS analyzer
WORKDIR /src
COPY adapters/wasmtime/ ./
RUN cargo build --release --locked --features native-code-lifetime --bin wasm-analyze --bin adapter-wasmtime

FROM golang:1.26.5-bookworm@sha256:53eeac89074db483fdf0ab3be1df32bf6e47562263d2d0d6baa7f26acb4957dd AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go test ./... && CGO_ENABLED=0 go build -trimpath -o /out/wasmbench ./cmd/wasmbench && CGO_ENABLED=0 go build -trimpath -o /out/adapter-wazero ./adapters/wazero

FROM node:26.4.0-bookworm-slim@sha256:ec82d089a8ae2cf02628da7b34ea57dc357b24db724d557fe2d240e6beb659c1
LABEL org.opencontainers.image.source="https://github.com/JairusSW/wasm-bench" \
      org.opencontainers.image.licenses="Apache-2.0"
WORKDIR /opt/wasmbench
COPY LICENSE NOTICE ./
COPY corpus/LICENSE ./corpus/LICENSE
COPY --from=build /out/ ./bin/
COPY --from=analyzer /src/target/release/wasm-analyze ./adapters/wasmtime/target/release/wasm-analyze
COPY --from=analyzer /src/target/release/adapter-wasmtime ./adapters/wasmtime/target/code-lifetime/release/adapter-wasmtime
COPY adapters/v8/adapter.mjs ./adapters/v8/adapter.mjs
COPY adapters/v8/harness.mjs ./adapters/v8/harness.mjs
COPY adapters/v8/floats.mjs ./adapters/v8/floats.mjs
COPY adapters/v8/profiling.mjs ./adapters/v8/profiling.mjs
COPY adapters/v8/compiler-mode.mjs ./adapters/v8/compiler-mode.mjs
COPY adapters/v8/tier-adapter.mjs ./adapters/v8/tier-adapter.mjs
COPY adapters/v8/tracing.mjs ./adapters/v8/tracing.mjs
ENTRYPOINT ["./bin/wasmbench"]
CMD ["doctor"]
