.PHONY: all build build-go build-rust run run-rust test fmt proto clean

all: build

# ─── Protocol Buffers ───────────────────────────────────────────
proto:
	@echo "Generating Go gRPC code from proto..."
	@mkdir -p proto/gen
	@protoc \
		--go_out=proto/gen --go_opt=paths=source_relative \
		--go-grpc_out=proto/gen --go-grpc_opt=paths=source_relative \
		-I proto proto/wasm_runtime.proto

# ─── Go (Orchestrator) ──────────────────────────────────────────
build-go: proto
	go build -o bin/ironclaw ./cmd/ironclaw

run-go: build-go
	./bin/ironclaw

test-go:
	go test -v ./...

fmt-go:
	go fmt ./...

# ─── Rust (WASM Runtime) ────────────────────────────────────────
build-rust:
	cd wasm-runtime && cargo build --release

run-rust:
	cd wasm-runtime && cargo run

test-rust:
	cd wasm-runtime && cargo test

fmt-rust:
	cd wasm-runtime && cargo fmt

# ─── Combined ───────────────────────────────────────────────────
build: build-go build-rust

run: build
	@echo "Starting WASM runtime sidecar..."
	@cd wasm-runtime && cargo run &
	@sleep 2
	@echo "Starting Go orchestrator..."
	@./bin/ironclaw

test: test-go test-rust

fmt: fmt-go fmt-rust

clean:
	rm -rf bin/ proto/gen/
	cd wasm-runtime && cargo clean
