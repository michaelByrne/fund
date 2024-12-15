package stats

import (
	"context"
	"github.com/google/uuid"
	"log/slog"
)

type statsStore interface {
	GetFundStats(ctx context.Context, id uuid.UUID) (*FundStats, error)
}

type StatsService struct {
	statsStore statsStore

	logger *slog.Logger
}

func NewStatsService(statsStore statsStore, logger *slog.Logger) *StatsService {
	return &StatsService{
		statsStore: statsStore,
		logger:     logger,
	}
}

func (s StatsService) GetFundStats(ctx context.Context, id uuid.UUID) (*FundStats, error) {
	fundStats, err := s.statsStore.GetFundStats(ctx, id)
	if err != nil {
		s.logger.Error("failed to get fund stats", slog.String("error", err.Error()))

		return nil, err
	}

	return fundStats, nil
}
