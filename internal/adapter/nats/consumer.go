package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	natsgo "github.com/nats-io/nats.go"
	"github.com/hanmahong5-arch/lurus-identity/internal/app"
	"github.com/hanmahong5-arch/lurus-identity/internal/pkg/event"
)

// Consumer subscribes to the LLM_EVENTS stream (published by lurus-api) and
// processes messages relevant to lurus-identity (VIP accumulation, etc.).
type Consumer struct {
	js  natsgo.JetStreamContext
	vip *app.VIPService
}

// NewConsumer creates a NATS JetStream consumer.
func NewConsumer(nc *natsgo.Conn, vip *app.VIPService) (*Consumer, error) {
	js, err := nc.JetStream()
	if err != nil {
		return nil, fmt.Errorf("jetstream context: %w", err)
	}
	return &Consumer{js: js, vip: vip}, nil
}

// Run starts consuming messages until ctx is cancelled.
func (c *Consumer) Run(ctx context.Context) error {
	sub, err := c.js.QueueSubscribe(
		event.SubjectLLMUsageReported,
		"lurus-identity-llm-usage",
		func(msg *natsgo.Msg) {
			if err := c.handleLLMUsage(ctx, msg); err != nil {
				slog.Error("handle llm usage", "err", err)
				_ = msg.Nak()
				return
			}
			_ = msg.Ack()
		},
		natsgo.Durable("lurus-identity-llm-usage"),
		natsgo.AckExplicit(),
		natsgo.MaxDeliver(5),
	)
	if err != nil {
		return fmt.Errorf("subscribe %s: %w", event.SubjectLLMUsageReported, err)
	}
	defer sub.Unsubscribe()

	<-ctx.Done()
	return ctx.Err()
}

func (c *Consumer) handleLLMUsage(ctx context.Context, msg *natsgo.Msg) error {
	var payload event.LLMUsageReportedPayload
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		return fmt.Errorf("unmarshal: %w", err)
	}
	if payload.AccountID <= 0 {
		return nil // ignore invalid messages
	}
	return c.vip.RecalculateFromWallet(ctx, payload.AccountID)
}
