// Package postgres 是 redcart 的 PostgreSQL 基础设施适配层：
// 以原生 SQL（database/sql）为主、GORM 辅助迁移/种子，向 application 层
// 提供仓储（Repository）与事务性发件箱（outbox）的实现。
package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Repository 实现 application.Repository 接口，持有连接池、outbox 存储
// 与进程内的会话（token）表；redcart 各业务聚合的数据读写最终都落到这里。
type Repository struct {
	db     *gormSQL
	gormDB *gorm.DB
	sqlDB  *sql.DB
	Outbox *outboxStore

	sessionMu sync.RWMutex
	sessions  map[string]sessionEntry
}

// gormSQL 是 *sql.DB 的薄封装，仅为让仓储与事务共享同一套查询辅助函数
// （dbQuerier）。名字中的 gorm 是历史遗留：这里实际操作的是标准库 database/sql。
type gormSQL struct {
	db *sql.DB
}

// gormTx 与 gormSQL 类似，是 *sql.Tx 的封装，让查询辅助函数在事务内也能复用。
type gormTx struct {
	tx *sql.Tx
}

// gormResult 模拟 sql.Result 供无法获得真实 Exec 结果的场景（如单元测试）使用；
// PostgreSQL 不支持自增主键回读，LastInsertId 一律返回错误。
type gormResult struct {
	rowsAffected int64
}

// sessionEntry 记录内存会话的归属用户与其 token 类型（访问/刷新）。
type sessionEntry struct {
	userID    int64
	tokenType application.TokenType
}

// dbQuerier 抽象出仓储与事务共有的三类查询方法，
// 使 getSKU、appendOrderEvent 等辅助函数既能作用于普通连接也能作用于事务连接。
type dbQuerier interface {
	QueryRow(query string, args ...any) *sql.Row
	Query(query string, args ...any) (*sql.Rows, error)
	Exec(query string, args ...any) (sql.Result, error)
}

var _ dbQuerier = (*gormSQL)(nil)
var _ dbQuerier = (*gormTx)(nil)

// QueryRow 执行单行查询。
func (g *gormSQL) QueryRow(query string, args ...any) *sql.Row {
	return g.db.QueryRow(query, args...)
}

// Query 执行多行查询。
func (g *gormSQL) Query(query string, args ...any) (*sql.Rows, error) {
	return g.db.Query(query, args...)
}

// Exec 执行写操作。
func (g *gormSQL) Exec(query string, args ...any) (sql.Result, error) {
	return g.db.Exec(query, args...)
}

// Begin 开启数据库事务并包装为 gormTx。
func (g *gormSQL) Begin() (*gormTx, error) {
	tx, err := g.db.Begin()
	if err != nil {
		return nil, err
	}
	return &gormTx{tx: tx}, nil
}

// QueryRow 在事务内执行单行查询。
func (tx *gormTx) QueryRow(query string, args ...any) *sql.Row {
	return tx.tx.QueryRow(query, args...)
}

// Query 在事务内执行多行查询。
func (tx *gormTx) Query(query string, args ...any) (*sql.Rows, error) {
	return tx.tx.Query(query, args...)
}

// Exec 在事务内执行写操作。
func (tx *gormTx) Exec(query string, args ...any) (sql.Result, error) {
	return tx.tx.Exec(query, args...)
}

// Commit 提交事务。
func (tx *gormTx) Commit() error {
	return tx.tx.Commit()
}

// Rollback 回滚事务。
func (tx *gormTx) Rollback() error {
	return tx.tx.Rollback()
}

// LastInsertId 不支持自增主键回读，一律返回错误。
func (r gormResult) LastInsertId() (int64, error) {
	return 0, fmt.Errorf("last insert id is not supported")
}

// RowsAffected 返回影响行数。
func (r gormResult) RowsAffected() (int64, error) {
	return r.rowsAffected, nil
}

var _ application.Repository = (*Repository)(nil)

// NewRepository 打开 PostgreSQL 连接池并完成连通性检查，随后自动执行
// schema 迁移与种子数据写入，保证开箱即用；任一步失败都会关闭连接返回错误。
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
		db:       &gormSQL{db: sqlDB},
		gormDB:   db,
		sqlDB:    sqlDB,
		Outbox:   newOutboxStore(&gormSQL{db: sqlDB}),
		sessions: make(map[string]sessionEntry),
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

// Close 关闭底层数据库连接池。
func (r *Repository) Close() error {
	return r.sqlDB.Close()
}

// queryUser 执行用户单行查询并解码为 domain.User。
func (r *Repository) queryUser(query string, arg any) (domain.User, error) {
	row := r.db.QueryRow(query, arg)
	var user domain.User
	err := row.Scan(&user.ID, &user.Nickname, &user.Phone, &user.PasswordHash, &user.Role, &user.CreatedAt, &user.UpdatedAt)
	return user, err
}

// queryMerchant 执行商家单行查询并解码为 domain.Merchant。
func (r *Repository) queryMerchant(query string, arg any) (domain.Merchant, error) {
	row := r.db.QueryRow(query, arg)
	var merchant domain.Merchant
	err := row.Scan(&merchant.ID, &merchant.UserID, &merchant.Name, &merchant.Description, &merchant.Status, &merchant.CreatedAt, &merchant.UpdatedAt)
	return merchant, err
}

// 以下一组辅助函数用于 Go 零值与 SQL NULL 之间的双向转换，
// 避免在业务代码里到处判断空值：零值（0/空串/零时间）一律落 NULL。
func nullTimePtr(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}

// nullTime 将零值时间转为 nil（NULL），非零时间原样返回。
func nullTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}

// nullableString 将空串转为 nil（NULL），非空串原样返回。
func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

// nullInt64 将 0 转为 nil（NULL），非零值原样返回。
func nullInt64(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}

// nullableJSON 将空或字面量 "null" 的 JSON 转为 nil（NULL），其余原样返回。
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

// poolMaxOpenConns 从环境变量读取连接池最大打开连接数。
func poolMaxOpenConns() int {
	return envInt("DB_MAX_OPEN_CONNS", defaultMaxOpenConns)
}

// poolMaxIdleConns 从环境变量读取连接池最大空闲连接数。
func poolMaxIdleConns() int {
	return envInt("DB_MAX_IDLE_CONNS", defaultMaxIdleConns)
}

// poolConnMaxLifetime 从环境变量读取连接最长复用时长。
func poolConnMaxLifetime() time.Duration {
	return envDuration("DB_CONN_MAX_LIFETIME", defaultConnMaxLifetime)
}

// envInt 读取环境变量为整数，非法或非正数时回退默认值。
func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}

// envDuration 读取环境变量为时长，非法或非正数时回退默认值。
func envDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return fallback
}
