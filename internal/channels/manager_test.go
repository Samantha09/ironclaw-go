package channels

import (
	"context"
	"testing"
	"time"
)

type testChannel struct {
	name   string
	msgCh  chan IncomingMessage
	sent   []OutgoingResponse
	closed bool
}

func newTestChannel(name string) *testChannel {
	return &testChannel{
		name:  name,
		msgCh: make(chan IncomingMessage, 8),
	}
}

func (c *testChannel) Name() string { return c.name }

func (c *testChannel) Messages() <-chan IncomingMessage { return c.msgCh }

func (c *testChannel) SendMessage(_ context.Context, msg OutgoingResponse) error {
	c.sent = append(c.sent, msg)
	return nil
}

func (c *testChannel) Shutdown(_ context.Context) error {
	c.closed = true
	close(c.msgCh)
	return nil
}

func TestManagerReceive(t *testing.T) {
	mgr := NewManager()
	ch := newTestChannel("test")
	mgr.Add(ch)
	mgr.Start(context.Background())

	ch.msgCh <- IncomingMessage{ID: "1", Content: "hello"}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	msg, err := mgr.Receive(ctx)
	if err != nil {
		t.Fatalf("receive: %v", err)
	}
	if msg.Content != "hello" {
		t.Errorf("content = %q, want hello", msg.Content)
	}
}

func TestManagerBroadcast(t *testing.T) {
	mgr := NewManager()
	ch1 := newTestChannel("a")
	ch2 := newTestChannel("b")
	mgr.Add(ch1)
	mgr.Add(ch2)
	mgr.Start(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_ = mgr.Broadcast(ctx, OutgoingResponse{Content: "hi"})

	if len(ch1.sent) != 1 || ch1.sent[0].Content != "hi" {
		t.Errorf("ch1.sent = %v, want [hi]", ch1.sent)
	}
	if len(ch2.sent) != 1 || ch2.sent[0].Content != "hi" {
		t.Errorf("ch2.sent = %v, want [hi]", ch2.sent)
	}
}

func TestManagerInject(t *testing.T) {
	mgr := NewManager()
	mgr.Start(context.Background())

	mgr.Inject(IncomingMessage{ID: "inj", Content: "injected"})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	msg, err := mgr.Receive(ctx)
	if err != nil {
		t.Fatalf("receive: %v", err)
	}
	if msg.Content != "injected" {
		t.Errorf("content = %q, want injected", msg.Content)
	}
}

func TestManagerDynamicAdd(t *testing.T) {
	mgr := NewManager()
	ch1 := newTestChannel("first")
	mgr.Add(ch1)
	mgr.Start(context.Background())

	// 启动后再添加通道
	ch2 := newTestChannel("second")
	mgr.Add(ch2)

	// 等待 addCh 被处理
	time.Sleep(100 * time.Millisecond)

	ch2.msgCh <- IncomingMessage{ID: "2", Content: "dynamic"}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	msg, err := mgr.Receive(ctx)
	if err != nil {
		t.Fatalf("receive: %v", err)
	}
	if msg.Content != "dynamic" {
		t.Errorf("content = %q, want dynamic", msg.Content)
	}
}

func TestManagerRemove(t *testing.T) {
	mgr := NewManager()
	ch := newTestChannel("removable")
	mgr.Add(ch)
	mgr.Start(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := mgr.Remove(ctx, "removable"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if !ch.closed {
		t.Error("expected channel to be closed")
	}
	if _, ok := mgr.Get("removable"); ok {
		t.Error("expected channel removed")
	}
}

func TestManagerNames(t *testing.T) {
	mgr := NewManager()
	mgr.Add(newTestChannel("a"))
	mgr.Add(newTestChannel("b"))

	names := mgr.Names()
	if len(names) != 2 {
		t.Errorf("names = %d, want 2", len(names))
	}
}

func TestManagerSend(t *testing.T) {
	mgr := NewManager()
	ch := newTestChannel("target")
	mgr.Add(ch)
	mgr.Start(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := mgr.Send(ctx, "target", OutgoingResponse{Content: "direct"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if len(ch.sent) != 1 || ch.sent[0].Content != "direct" {
		t.Errorf("sent = %v, want [direct]", ch.sent)
	}
}

func TestManagerSendNotFound(t *testing.T) {
	mgr := NewManager()
	mgr.Start(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := mgr.Send(ctx, "missing", OutgoingResponse{}); err == nil {
		t.Error("expected error for missing channel")
	}
}

func TestManagerStartIdempotent(t *testing.T) {
	mgr := NewManager()
	ch := newTestChannel("idempotent")
	mgr.Add(ch)

	mgr.Start(context.Background())
	mgr.Start(context.Background()) // 第二次应该无操作

	ch.msgCh <- IncomingMessage{Content: "msg"}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	msg, err := mgr.Receive(ctx)
	if err != nil {
		t.Fatalf("receive: %v", err)
	}
	if msg.Content != "msg" {
		t.Errorf("content = %q, want msg", msg.Content)
	}
}
