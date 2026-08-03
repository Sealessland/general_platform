package postgres

import (
	"context"
	"database/sql"
	"fmt"
	backendai "github.com/example/redcart-copilot/backend/internal/ai"
	"github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
	"os"
	"testing"
	"time"
)

// skipIfNoPostgres 未配置 PostgreSQL 集成环境时跳过测试，并返回 DSN。
func skipIfNoPostgres(t *testing.T) (string, bool) {
	t.Helper()
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" || os.Getenv("RUN_POSTGRES_INTEGRATION") != "1" {
		t.Skip("POSTGRES_DSN not set or RUN_POSTGRES_INTEGRATION != 1")
	}
	return dsn, true
}

// newPostgresRepo 创建连接真实 PostgreSQL 的仓储，测试结束时自动关闭。
func newPostgresRepo(t *testing.T) *Repository {
	t.Helper()
	dsn, _ := skipIfNoPostgres(t)
	repo, err := NewRepository(dsn)
	if err != nil {
		t.Fatalf("new postgres repository: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	return repo
}

// newPostgresService 创建仓储与应用服务（Mock AI），供集成测试复用。
func newPostgresService(t *testing.T) (*Repository, *application.Service) {
	t.Helper()
	repo := newPostgresRepo(t)
	return repo, application.NewService(repo, backendai.MockProvider{})
}

// openRawConn 打开直连 PostgreSQL 的裸连接，用于模拟外部并发事务。
func openRawConn(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open raw postgres conn: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Ping(); err != nil {
		t.Fatalf("ping raw postgres conn: %v", err)
	}
	return db
}

// createStabilityProductAndSKU 创建用于稳定性测试的商品与指定库存的 SKU。
func createStabilityProductAndSKU(t *testing.T, repo *Repository, stock int) domain.SKU {
	t.Helper()
	now := time.Now().UTC()
	product, err := repo.SaveProduct(domain.Product{
		MerchantID:    1,
		Title:         fmt.Sprintf("Stability Product %d", now.UnixNano()),
		Description:   "created for stability test",
		CategoryID:    999,
		Status:        domain.ProductStatusOnline,
		SellingPoints: []string{"stability"},
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if err != nil {
		t.Fatalf("create product: %v", err)
	}
	sku, err := repo.SaveSKU(domain.SKU{
		ProductID:   product.ID,
		SKUName:     fmt.Sprintf("Stability SKU %d", now.UnixNano()),
		SKUAttrs:    map[string]string{"batch": fmt.Sprintf("%d", now.UnixNano())},
		PriceCent:   100,
		Stock:       stock,
		LockedStock: 0,
		Status:      domain.SKUStatusActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("create sku: %v", err)
	}
	return sku
}

// createStabilityOrder 通过应用服务创建一条指定数量 SKU 的稳定性测试订单。
func createStabilityOrder(t *testing.T, service *application.Service, sku domain.SKU, quantity int) *application.OrderView {
	t.Helper()
	view, err := service.CreateOrder(context.Background(), application.Actor{UserID: 1, Role: domain.RoleConsumer}, fmt.Sprintf("stability-%d-%d", sku.ID, time.Now().UnixNano()), application.CheckoutInput{
		Items:           []application.OrderLineInput{{SKUID: sku.ID, Quantity: quantity}},
		ReceiverName:    "Alice",
		ReceiverPhone:   "13800000001",
		ReceiverAddress: "Shanghai",
	})
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	return view
}
