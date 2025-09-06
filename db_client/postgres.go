package db_client

import (
	"context"

	"github.com/Strum355/log"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	DB  *pgxpool.Pool
	Ctx = context.Background()
)

func init() {
	var err error
	dsn := "postgres://postgres:postgres@postgres:5432/postgres"

	DB, err = pgxpool.New(Ctx, dsn)
	if err != nil {
		log.WithError(err).Error("Unable to connect to database")
	}

	if err := DB.Ping(Ctx); err != nil {
		log.WithError(err).Error("Cannot ping database")
	}
}
