package frontier_test

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wanderingtilldeath/news-aggregator/internal/db"
)

func migrateAgain(pool *pgxpool.Pool) error {
	return db.Migrate(context.Background(), pool)
}
