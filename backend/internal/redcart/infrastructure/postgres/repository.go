package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Repository struct {
	db     *gormSQL
	gormDB *gorm.DB
	sqlDB  *sql.DB
	Outbox *outboxStore
}

type gormSQL struct {
	db *sql.DB
}

type gormTx struct {
	tx *sql.Tx
}

type gormResult struct {
	rowsAffected int64
}

type dbQuerier interface {
	QueryRow(query string, args ...any) *sql.Row
	Query(query string, args ...any) (*sql.Rows, error)
	Exec(query string, args ...any) (sql.Result, error)
}

var _ dbQuerier = (*gormSQL)(nil)
var _ dbQuerier = (*gormTx)(nil)

func (g *gormSQL) QueryRow(query string, args ...any) *sql.Row {
	return g.db.QueryRow(query, args...)
}

func (g *gormSQL) Query(query string, args ...any) (*sql.Rows, error) {
	return g.db.Query(query, args...)
}

func (g *gormSQL) Exec(query string, args ...any) (sql.Result, error) {
	return g.db.Exec(query, args...)
}

func (g *gormSQL) Begin() (*gormTx, error) {
	tx, err := g.db.Begin()
	if err != nil {
		return nil, err
	}
	return &gormTx{tx: tx}, nil
}

func (tx *gormTx) QueryRow(query string, args ...any) *sql.Row {
	return tx.tx.QueryRow(query, args...)
}

func (tx *gormTx) Query(query string, args ...any) (*sql.Rows, error) {
	return tx.tx.Query(query, args...)
}

func (tx *gormTx) Exec(query string, args ...any) (sql.Result, error) {
	return tx.tx.Exec(query, args...)
}

func (tx *gormTx) Commit() error {
	return tx.tx.Commit()
}

func (tx *gormTx) Rollback() error {
	return tx.tx.Rollback()
}

func (r gormResult) LastInsertId() (int64, error) {
	return 0, fmt.Errorf("last insert id is not supported")
}

func (r gormResult) RowsAffected() (int64, error) {
	return r.rowsAffected, nil
}

var _ application.Repository = (*Repository)(nil)

func NewRepository(dsn string) (*Repository, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open postgres with gorm: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get postgres sql db from gorm: %w", err)
	}
	sqlDB.SetMaxOpenConns(poolMaxOpenConns())
	sqlDB.SetMaxIdleConns(poolMaxIdleConns())
	sqlDB.SetConnMaxLifetime(poolConnMaxLifetime())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	repo := &Repository{
		db:     &gormSQL{db: sqlDB},
		gormDB: db,
		sqlDB:  sqlDB,
		Outbox: newOutboxStore(&gormSQL{db: sqlDB}),
	}
	if err := repo.migrate(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	if err := repo.seed(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return repo, nil
}

func (r *Repository) Close() error {
	return r.sqlDB.Close()
}
func (r *Repository) queryUser(query string, arg any) (domain.User, error) {
	row := r.db.QueryRow(query, arg)
	var user domain.User
	err := row.Scan(&user.ID, &user.Nickname, &user.Phone, &user.PasswordHash, &user.Role, &user.CreatedAt, &user.UpdatedAt)
	return user, err
}

func (r *Repository) queryMerchant(query string, arg any) (domain.Merchant, error) {
	row := r.db.QueryRow(query, arg)
	var merchant domain.Merchant
	err := row.Scan(&merchant.ID, &merchant.UserID, &merchant.Name, &merchant.Description, &merchant.Status, &merchant.CreatedAt, &merchant.UpdatedAt)
	return merchant, err
}

func nullTimePtr(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}

func nullTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullInt64(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}

func nullableJSON(payload []byte) any {
	if len(payload) == 0 || string(payload) == "null" {
		return nil
	}
	return string(payload)
}

const (
	defaultMaxOpenConns    = 10
	defaultMaxIdleConns    = 10
	defaultConnMaxLifetime = 30 * time.Minute
)

func poolMaxOpenConns() int {
	return envInt("DB_MAX_OPEN_CONNS", defaultMaxOpenConns)
}

func poolMaxIdleConns() int {
	return envInt("DB_MAX_IDLE_CONNS", defaultMaxIdleConns)
}

func poolConnMaxLifetime() time.Duration {
	return envDuration("DB_CONN_MAX_LIFETIME", defaultConnMaxLifetime)
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return fallback
}
