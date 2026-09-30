.PHONY: build test demo clean
build:
	cargo build --release --locked --manifest-path adapters/wasmtime/Cargo.toml --bin wasm-analyze
	go build -trimpath -o bin/wasmbench ./cmd/wasmbench
	go build -trimpath -o bin/adapter-wazero ./adapters/wazero
test:
	go test ./...
	node --test publish/report-ui.test.mjs recipes/*.test.mjs
demo: build
	./bin/wasmbench run --suite core --runtimes wazero,v8 --launches 3 --samples 5 --operations 10
