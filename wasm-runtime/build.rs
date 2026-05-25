fn main() {
    tonic_build::compile_protos("../proto/wasm_runtime.proto")
        .expect("Failed to compile protos");
}
