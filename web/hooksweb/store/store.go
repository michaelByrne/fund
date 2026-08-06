// Package store records which provider webhooks have already been accepted.
package store

import (
	"context"

	"boardfund/db"

	"github.com/jackc/pgx/v5/pgxpool"
)

type DeliveryStore struct {
	queries *db.Queries
}

func NewDeliveryStore(conn *pgxpool.Pool) DeliveryStore {
	return DeliveryStore{queries: db.New(conn)}
}

// RecordDelivery reports whether this transmission is one we have not seen.
//
// False means a replay, or PayPal redelivering something we already accepted --
// which is the same thing from here, and in both cases the event has already been
// published to the stream and does not need publishing again.
func (s DeliveryStore) RecordDelivery(ctx context.Context, transmissionID, eventType string) (bool, error) {
	rows, err := s.queries.RecordWebhookDelivery(ctx, db.RecordWebhookDeliveryParams{
		TransmissionID: transmissionID,
		EventType:      eventType,
	})
	if err != nil {
		return false, err
	}

	return len(rows) > 0, nil
}
