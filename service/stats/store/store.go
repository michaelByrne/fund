package store

import (
	"boardfund/db"
	"boardfund/pg"
	"boardfund/service/stats"
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type StatsStore struct {
	queries *db.Queries
	conn    *pgxpool.Pool
}

func NewStatsStore(conn *pgxpool.Pool) StatsStore {
	return StatsStore{
		queries: db.New(conn),
		conn:    conn,
	}
}

func (s StatsStore) GetFundStats(ctx context.Context, id uuid.UUID) (*stats.FundStats, error) {
	query := s.queries.GetFundStats

	return pg.FetchOne(ctx, id, query, fromDBFundStatsRow)
}

func (s StatsStore) GetMonthlyTotalsByFund(ctx context.Context, id uuid.UUID) ([]stats.MonthTotal, error) {
	query := s.queries.GetMonthlyTotalsByFund

	return pg.FetchMany(ctx, id, query, fromDBMonthlyTotalsRow)
}
