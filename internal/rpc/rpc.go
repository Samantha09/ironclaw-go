package rpc

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"

	wasmproto "github.com/nearai/ironclaw-go/proto/gen"
)

// Client 是到 Rust WASM sidecar 的 gRPC 客户端。
type Client struct {
	conn   *grpc.ClientConn
	client wasmproto.WasmRuntimeClient
	addr   string
}

// NewClient 创建新的 sidecar 客户端。
func NewClient(addr string) (*Client, error) {
	// 支持 Unix Domain Socket（以 unix:// 开头）或 TCP
	dialAddr := addr
	if len(addr) > 7 && addr[:7] == "unix://" {
		// Unix Domain Socket
		dialAddr = "unix:" + addr[7:]
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, dialAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("dial sidecar %q: %w", addr, err)
	}

	return &Client{
		conn:   conn,
		client: wasmproto.NewWasmRuntimeClient(conn),
		addr:   addr,
	}, nil
}

// Close 关闭连接。
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// Healthy 检查连接是否健康。
func (c *Client) Healthy() bool {
	if c.conn == nil {
		return false
	}
	state := c.conn.GetState()
	return state == connectivity.Ready
}

// ExecuteTool 在 sidecar 中执行 WASM 工具。
func (c *Client) ExecuteTool(ctx context.Context, toolName string, wasmModule []byte, paramsJSON, userID string, secrets map[string]string, fuelLimit, maxMemory uint64) (*wasmproto.ToolResponse, error) {
	req := &wasmproto.ToolRequest{
		ToolName:   toolName,
		WasmModule: wasmModule,
		ParamsJson: paramsJSON,
		UserId:     userID,
		Secrets:    secrets,
		FuelLimit:  fuelLimit,
		MaxMemory:  maxMemory,
	}

	resp, err := c.client.ExecuteTool(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("execute tool: %w", err)
	}
	return resp, nil
}

// LoadChannel 在 sidecar 中加载 WASM 通道。
func (c *Client) LoadChannel(ctx context.Context, channelName string, wasmModule []byte, configJSON string) (*wasmproto.ChannelHandle, error) {
	req := &wasmproto.ChannelConfig{
		ChannelName: channelName,
		WasmModule:  wasmModule,
		ConfigJson:  configJSON,
	}

	resp, err := c.client.LoadChannel(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("load channel: %w", err)
	}
	return resp, nil
}

// SendChannelEvent 向已加载的 WASM 通道发送事件。
func (c *Client) SendChannelEvent(ctx context.Context, handleID, eventJSON string) error {
	req := &wasmproto.ChannelEvent{
		HandleId:  handleID,
		EventJson: eventJSON,
	}

	_, err := c.client.SendChannelEvent(ctx, req)
	if err != nil {
		return fmt.Errorf("send channel event: %w", err)
	}
	return nil
}

// Health 检查 sidecar 健康状态。
func (c *Client) Health(ctx context.Context) (*wasmproto.HealthStatus, error) {
	resp, err := c.client.Health(ctx, &wasmproto.Empty{})
	if err != nil {
		return nil, fmt.Errorf("health check: %w", err)
	}
	return resp, nil
}
