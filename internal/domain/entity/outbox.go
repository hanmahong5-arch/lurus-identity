package entity

import (
	"encoding/json"
	"time"
)

// OutboxEvent represents a pending or published event in the transactional outbox.
// Events are inserted within the same DB transaction as the business state change,
// ensuring at-least-once delivery to NATS via the relay goroutine.
type OutboxEvent struct {
	ID          int64           `gorm:"primaryKey;autoIncrement"`
	EventID     string          `gorm:"type:varchar(36);not null"`
	Subject     string          `gorm:"type:varchar(128);not null"`
	Payload     json.RawMessage `gorm:"type:jsonb;not null"`
	CreatedAt   time.Time       `gorm:"autoCreateTime"`
	PublishedAt *time.Time
	Attempts    int    `gorm:"default:0"`
	LastError   string `gorm:"type:text"`
}

// TableName returns the fully qualified table name for GORM.
func (OutboxEvent) TableName() string { return "identity.outbox_events" }
