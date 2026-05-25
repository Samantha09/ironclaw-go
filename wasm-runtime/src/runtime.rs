//! IronClaw WASM Runtime Core
//!
//! Manages wasmtime Engine, module cache, and execution of WASM tools
//! and channel adapters with resource limits and secret injection.

use std::collections::HashMap;
use std::sync::Mutex;
use wasmtime::{Config, Engine, Linker, Module, Store, StoreLimits, StoreLimitsBuilder};
use wasmtime_wasi::preview1::{self, WasiP1Ctx};
use wasmtime_wasi::{pipe::MemoryOutputPipe, WasiCtxBuilder};

/// Per-store state that holds both WASI context and resource limits.
struct StoreState {
    wasi: WasiP1Ctx,
    limits: StoreLimits,
}

/// Channel instance metadata held by the runtime.
struct ChannelInstance {
    module_name: String,
    config_json: String,
}

/// Result of executing a WASM tool.
#[derive(Debug)]
pub struct ToolResult {
    pub success: bool,
    pub output_json: String,
    pub error_message: String,
    pub fuel_consumed: u64,
}

/// The WASM runtime that hosts wasmtime and manages modules / channels.
pub struct Runtime {
    engine: Engine,
    modules: Mutex<HashMap<String, Module>>,
    channels: Mutex<HashMap<String, ChannelInstance>>,
}

impl Runtime {
    /// Create a new Runtime with fuel consumption enabled.
    pub fn new() -> anyhow::Result<Self> {
        let mut config = Config::new();
        config.consume_fuel(true);
        let engine = Engine::new(&config)?;
        Ok(Self {
            engine,
            modules: Mutex::new(HashMap::new()),
            channels: Mutex::new(HashMap::new()),
        })
    }

    /// Execute a WASM tool with the given parameters and resource limits.
    pub fn execute_tool(
        &self,
        tool_name: &str,
        wasm_bytes: &[u8],
        params_json: &str,
        secrets: &HashMap<String, String>,
        fuel_limit: u64,
        max_memory: u64,
    ) -> anyhow::Result<ToolResult> {
        let module = self.get_or_compile_module(tool_name, wasm_bytes)?;

        let mut linker: Linker<StoreState> = Linker::new(&self.engine);
        preview1::add_to_linker_sync(&mut linker, |state| &mut state.wasi)?;

        let stdout_pipe = MemoryOutputPipe::new(1024 * 1024);
        let stderr_pipe = MemoryOutputPipe::new(1024 * 1024);

        let mut builder = WasiCtxBuilder::new();
        builder.stdin(wasmtime_wasi::pipe::MemoryInputPipe::new(&[][..]));
        builder.stdout(stdout_pipe.clone());
        builder.stderr(stderr_pipe.clone());
        builder.env("PARAMS_JSON", params_json);
        for (k, v) in secrets {
            builder.env(format!("SECRET_{}", k), v);
        }
        let wasi_ctx = builder.build_p1();

        let limits = StoreLimitsBuilder::new()
            .memory_size(max_memory as usize)
            .build();

        let mut store = Store::new(
            &self.engine,
            StoreState {
                wasi: wasi_ctx,
                limits,
            },
        );
        store.limiter(|state| &mut state.limits);
        store.set_fuel(fuel_limit)?;

        let instance = linker.instantiate(&mut store, &module)?;
        let start = instance.get_typed_func::<(), ()>(&mut store, "_start")?;

        let exec_result = start.call(&mut store, ());

        let fuel_consumed = fuel_limit.saturating_sub(store.get_fuel().unwrap_or(0));
        let stdout = String::from_utf8_lossy(&stdout_pipe.contents()).to_string();
        let stderr = String::from_utf8_lossy(&stderr_pipe.contents()).to_string();

        match exec_result {
            Ok(()) => Ok(ToolResult {
                success: true,
                output_json: stdout,
                error_message: String::new(),
                fuel_consumed,
            }),
            Err(e) => Ok(ToolResult {
                success: false,
                output_json: stdout,
                error_message: format!("{}\n{}", e, stderr),
                fuel_consumed,
            }),
        }
    }

    /// Load a WASM channel adapter and return a handle ID.
    pub fn load_channel(
        &self,
        channel_name: &str,
        wasm_bytes: &[u8],
        config_json: &str,
    ) -> anyhow::Result<String> {
        self.get_or_compile_module(channel_name, wasm_bytes)?;
        let handle_id = uuid::Uuid::new_v4().to_string();
        let mut channels = self.channels.lock().unwrap();
        channels.insert(
            handle_id.clone(),
            ChannelInstance {
                module_name: channel_name.to_string(),
                config_json: config_json.to_string(),
            },
        );
        Ok(handle_id)
    }

    /// Send an event to a loaded channel.
    pub fn send_channel_event(&self, handle_id: &str, event_json: &str) -> anyhow::Result<()> {
        let channels = self.channels.lock().unwrap();
        let _channel = channels
            .get(handle_id)
            .ok_or_else(|| anyhow::anyhow!("channel handle {} not found", handle_id))?;
        tracing::info!(
            handle = %handle_id,
            event = %event_json,
            "channel event received (MVP stub)"
        );
        Ok(())
    }

    fn get_or_compile_module(&self, name: &str, bytes: &[u8]) -> anyhow::Result<Module> {
        let mut modules = self.modules.lock().unwrap();
        if let Some(module) = modules.get(name) {
            return Ok(module.clone());
        }
        let module = Module::new(&self.engine, bytes)?;
        modules.insert(name.to_string(), module.clone());
        Ok(module)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    /// A minimal valid WASM module in WAT text format.
    /// wasmtime::Module::new accepts WAT when the `wat` feature is enabled (default).
    const EMPTY_WAT: &str = r#"
(module
  (func $start)
  (export "_start" (func $start))
)
"#;

    /// A WASM module that writes {"result":"hello"} to stdout using WASI fd_write.
    /// This is a pre-compiled wat snippet for testing output capture.
    /// For simplicity in unit tests we rely on the empty module and verify fuel.
    #[test]
    fn test_execute_empty_tool() {
        let rt = Runtime::new().unwrap();
        let result = rt
            .execute_tool(
                "empty",
                EMPTY_WAT.as_bytes(),
                r#"{"msg":"hi"}"#,
                &HashMap::new(),
                10_000,
                16 * 1024 * 1024,
            )
            .unwrap();
        assert!(result.success, "empty tool should succeed: {}", result.error_message);
        assert!(result.fuel_consumed > 0, "empty func should consume some fuel");
    }

    #[test]
    fn test_fuel_limit_traps() {
        let rt = Runtime::new().unwrap();
        // Give only 1 fuel — not enough to even instantiate and call.
        let result = rt
            .execute_tool(
                "empty",
                EMPTY_WAT.as_bytes(),
                "{}",
                &HashMap::new(),
                1,
                16 * 1024 * 1024,
            )
            .unwrap();
        assert!(!result.success, "should trap due to fuel exhaustion");
        // wasmtime fuel trap message varies by version; just ensure it failed.
        assert!(
            !result.error_message.is_empty(),
            "error message should not be empty: {:?}",
            result
        );
    }

    #[test]
    fn test_module_caching() {
        let rt = Runtime::new().unwrap();
        // First call compiles the module.
        let _ = rt
            .execute_tool("cached", EMPTY_WAT.as_bytes(), "{}", &HashMap::new(), 10_000, 16 * 1024 * 1024)
            .unwrap();
        // Second call should use cached module.
        let _ = rt
            .execute_tool("cached", EMPTY_WAT.as_bytes(), "{}", &HashMap::new(), 10_000, 16 * 1024 * 1024)
            .unwrap();
        let modules = rt.modules.lock().unwrap();
        assert_eq!(modules.len(), 1);
    }

    #[test]
    fn test_load_and_send_channel() {
        let rt = Runtime::new().unwrap();
        let handle = rt
            .load_channel("test_ch", EMPTY_WAT.as_bytes(), r#"{"url":"http://example.com"}"#)
            .unwrap();
        assert!(!handle.is_empty());

        rt.send_channel_event(&handle, r#"{"type":"message"}"#)
            .unwrap();

        assert!(rt.send_channel_event("bad-handle", "{}").is_err());
    }
}
