//! IronClaw WASM Runtime Sidecar
//!
//! A standalone process that hosts wasmtime and executes WASM tools
//! and channel adapters on behalf of the Go orchestrator.

use std::net::SocketAddr;
use tonic::{transport::Server, Request, Response, Status};

pub mod wasm_proto {
    tonic::include_proto!("ironclaw.wasm");
}

use wasm_proto::{
    wasm_runtime_server::{WasmRuntime, WasmRuntimeServer},
    ChannelConfig, ChannelEvent, ChannelHandle, Empty, HealthStatus, ToolRequest, ToolResponse,
};

#[derive(Default)]
pub struct WasmRuntimeService {
    // TODO: wasmtime engine, module cache, channel handles
}

#[tonic::async_trait]
impl WasmRuntime for WasmRuntimeService {
    async fn execute_tool(
        &self,
        request: Request<ToolRequest>,
    ) -> Result<Response<ToolResponse>, Status> {
        let req = request.into_inner();
        tracing::info!(tool = %req.tool_name, user_id = %req.user_id, "execute_tool");

        // TODO: instantiate wasmtime, inject secrets, execute with fuel limit
        let response = ToolResponse {
            success: true,
            output_json: r#"{"result":"ok"}"#.to_string(),
            error_message: String::new(),
            fuel_consumed: 0,
        };
        Ok(Response::new(response))
    }

    async fn load_channel(
        &self,
        request: Request<ChannelConfig>,
    ) -> Result<Response<ChannelHandle>, Status> {
        let req = request.into_inner();
        tracing::info!(channel = %req.channel_name, "load_channel");

        let handle = ChannelHandle {
            handle_id: uuid::Uuid::new_v4().to_string(),
            success: true,
            error_message: String::new(),
        };
        Ok(Response::new(handle))
    }

    async fn send_channel_event(
        &self,
        request: Request<ChannelEvent>,
    ) -> Result<Response<Empty>, Status> {
        let req = request.into_inner();
        tracing::info!(handle = %req.handle_id, "send_channel_event");
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

    Server::builder()
        .add_service(WasmRuntimeServer::new(WasmRuntimeService::default()))
        .serve(addr)
        .await?;

    Ok(())
}
