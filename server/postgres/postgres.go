package postgres

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/poodbooq/bitburst_server/logger"
	"github.com/poodbooq/bitburst_server/models"
	"sync"
)

type Postgres interface {
	UpsertObject(ctx context.Context, obj models.Object) error
	DeleteObjectByID(ctx context.Context, id int) error
}

type Config struct {
	Host               string
	Port               string
	User               string
	Password           string
	PoolMaxConnections int
	Database           string
	SSLMode            string
}

type postgres struct {
	pg  *pgxpool.Pool
	log logger.Logger
}

var (
	singleton *postgres
	once      = new(sync.Once)
)

func Load(ctx context.Context, cfg Config, log logger.Logger) (*postgres, error) {
	var err error
	once.Do(func() {
		singleton = new(postgres)
		var (
			poolConfig *pgxpool.Config
			pool       *pgxpool.Pool
		)
		poolConfig, err = pgxpool.ParseConfig(getPgUrl(cfg))
		if err != nil {
			log.Error(err)
			return
		}
		pool, err = pgxpool.ConnectConfig(ctx, poolConfig)
		if err != nil {
			log.Error(err)
			return
		}

		singleton.pg = pool
		singleton.log = log
	})
	return singleton, err
}

func getPgUrl(cfg Config) string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s&pool_max_conns=%v",
		cfg.User,
		cfg.Password,
		cfg.Host,
		cfg.Port,
		cfg.Database,
		cfg.SSLMode,
		cfg.PoolMaxConnections,
	)
}

func (p *postgres) Close() error {
	p.pg.Close()
	return nil
}

func (p *postgres) UpsertObject(ctx context.Context, obj models.Object) error {
	_, err := p.pg.Exec(ctx, "INSERT INTO objects (id, last_seen_at) VALUES ($1, $2) ON CONFLICT (id) DO UPDATE SET last_seen_at = $2", obj.ID, obj.LastSeenAt)
	return err
}

func (p *postgres) DeleteObjectByID(ctx context.Context, id int) error {
	_, err := p.pg.Exec(ctx, `DELETE FROM objects WHERE id = $1`, id)
	return err
}
