//! IronClaw WASM Runtime Sidecar
//!
//! A standalone process that hosts wasmtime and executes WASM tools
//! and channel adapters on behalf of the Go orchestrator.

use std::net::SocketAddr;
use std::sync::Arc;
use tonic::{transport::Server, Request, Response, Status};

mod runtime;
use runtime::Runtime;

pub mod wasm_proto {
    tonic::include_proto!("ironclaw.wasm");
}

use wasm_proto::{
    wasm_runtime_server::{WasmRuntime, WasmRuntimeServer},
    ChannelConfig, ChannelEvent, ChannelHandle, Empty, HealthStatus, ToolRequest, ToolResponse,
};

pub struct WasmRuntimeService {
    runtime: Arc<Runtime>,
}

impl WasmRuntimeService {
    pub fn new() -> anyhow::Result<Self> {
        Ok(Self {
            runtime: Arc::new(Runtime::new()?),
        })
    }
}

#[tonic::async_trait]
impl WasmRuntime for WasmRuntimeService {
    async fn execute_tool(
        &self,
        request: Request<ToolRequest>,
    ) -> Result<Response<ToolResponse>, Status> {
        let req = request.into_inner();
        tracing::info!(tool = %req.tool_name, user_id = %req.user_id, "execute_tool");

        let secrets: std::collections::HashMap<String, String> = req.secrets.into_iter().collect();

        let result = tokio::task::spawn_blocking({
            let rt = Arc::clone(&self.runtime);
            let tool_name = req.tool_name.clone();
            let wasm_module = req.wasm_module.clone();
            let params_json = req.params_json.clone();
            let secrets = secrets.clone();
            let fuel_limit = req.fuel_limit;
            let max_memory = req.max_memory;
            move || {
                rt.execute_tool(
                    &tool_name,
                    &wasm_module,
                    &params_json,
                    &secrets,
                    fuel_limit,
                    max_memory,
                )
            }
        })
        .await
        .map_err(|e| Status::internal(format!("blocking task failed: {}", e)))?
        .map_err(|e| Status::internal(format!("wasm execution failed: {}", e)))?;

        let response = ToolResponse {
            success: result.success,
            output_json: result.output_json,
            error_message: result.error_message,
            fuel_consumed: result.fuel_consumed,
        };
        Ok(Response::new(response))
    }

    async fn load_channel(
        &self,
        request: Request<ChannelConfig>,
    ) -> Result<Response<ChannelHandle>, Status> {
        let req = request.into_inner();
        tracing::info!(channel = %req.channel_name, "load_channel");

        let handle_id = self
            .runtime
            .load_channel(&req.channel_name, &req.wasm_module, &req.config_json)
            .map_err(|e| Status::internal(format!("load channel failed: {}", e)))?;

        Ok(Response::new(ChannelHandle {
            handle_id,
            success: true,
            error_message: String::new(),
        }))
    }

    async fn send_channel_event(
        &self,
        request: Request<ChannelEvent>,
    ) -> Result<Response<Empty>, Status> {
        let req = request.into_inner();
        tracing::info!(handle = %req.handle_id, "send_channel_event");

        self.runtime
            .send_channel_event(&req.handle_id, &req.event_json)
            .map_err(|e| Status::internal(format!("send event failed: {}", e)))?;

        Ok(Response::new(Empty {}))
    }

    async fn health(&self, _request: Request<Empty>) -> Result<Response<HealthStatus>, Status> {
        Ok(Response::new(HealthStatus {
            healthy: true,
            version: env!("CARGO_PKG_VERSION").to_string(),
        }))
    }
}

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    tracing_subscriber::fmt()
        .with_env_filter(tracing_subscriber::EnvFilter::from_default_env())
        .init();

    let addr: SocketAddr = std::env::var("WASM_RUNTIME_ADDR")
        .unwrap_or_else(|_| "127.0.0.1:50051".to_string())
        .parse()?;

    tracing::info!(%addr, "IronClaw WASM Runtime starting");

    let service = WasmRuntimeService::new()?;

    Server::builder()
        .add_service(WasmRuntimeServer::new(service))
        .serve(addr)
        .await?;

    Ok(())
}
