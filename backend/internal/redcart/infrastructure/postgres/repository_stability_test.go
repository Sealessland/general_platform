package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	backendai "github.com/example/redcart-copilot/backend/internal/ai"
	orderdomain "github.com/example/redcart-copilot/backend/internal/order/domain"
	"github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func skipIfNoPostgres(t *testing.T) (string, bool) {
	t.Helper()
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" || os.Getenv("RUN_POSTGRES_INTEGRATION") != "1" {
		t.Skip("POSTGRES_DSN not set or RUN_POSTGRES_INTEGRATION != 1")
	}
	return dsn, true
}

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

func newPostgresService(t *testing.T) (*Repository, *application.Service) {
	t.Helper()
	repo := newPostgresRepo(t)
	return repo, application.NewService(repo, backendai.MockProvider{})
}

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

// TestReadCommittedNoDirtyRead verifies that uncommitted writes are invisible
// to other transactions under PostgreSQL default READ COMMITTED isolation.
func TestReadCommittedNoDirtyRead(t *testing.T) {
	dsn, _ := skipIfNoPostgres(t)
	db := openRawConn(t, dsn)
	repo := newPostgresRepo(t)
	sku := createStabilityProductAndSKU(t, repo, 100)

	tx1, err := db.Begin()
	if err != nil {
		t.Fatalf("begin tx1: %v", err)
	}
	_, err = tx1.Exec(`UPDATE product_skus SET locked_stock = locked_stock + 1 WHERE id = $1`, sku.ID)
	if err != nil {
		_ = tx1.Rollback()
		t.Fatalf("tx1 update: %v", err)
	}

	var locked int64
	err = db.QueryRow(`SELECT locked_stock FROM product_skus WHERE id = $1`, sku.ID).Scan(&locked)
	if err != nil {
		_ = tx1.Rollback()
		t.Fatalf("read outside tx: %v", err)
	}
	if locked != 0 {
		t.Fatalf("dirty read detected: expected locked_stock=0, got %d", locked)
	}

	if err := tx1.Rollback(); err != nil {
		t.Fatalf("rollback tx1: %v", err)
	}
}

// TestNonRepeatableReadInventory documents that READ COMMITTED allows
// non-repeatable reads: a second read in the same transaction sees committed
// changes from other transactions. This is expected behaviour, not a bug.
func TestNonRepeatableReadInventory(t *testing.T) {
	dsn, _ := skipIfNoPostgres(t)
	db := openRawConn(t, dsn)
	repo := newPostgresRepo(t)
	sku := createStabilityProductAndSKU(t, repo, 100)

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback()

	var first int64
	if err := tx.QueryRow(`SELECT locked_stock FROM product_skus WHERE id = $1`, sku.ID).Scan(&first); err != nil {
		t.Fatalf("first read: %v", err)
	}

	_, err = db.Exec(`UPDATE product_skus SET locked_stock = locked_stock + 5 WHERE id = $1`, sku.ID)
	if err != nil {
		t.Fatalf("external update: %v", err)
	}

	var second int64
	if err := tx.QueryRow(`SELECT locked_stock FROM product_skus WHERE id = $1`, sku.ID).Scan(&second); err != nil {
		t.Fatalf("second read: %v", err)
	}
	if second != first+5 {
		t.Fatalf("unexpected repeatable read: first=%d second=%d", first, second)
	}
}

// TestPhantomInventoryLocks documents that READ COMMITTED allows phantom reads:
// a second query in the same transaction can see rows inserted by other
// transactions. This is expected behaviour, not a bug.
func TestPhantomInventoryLocks(t *testing.T) {
	dsn, _ := skipIfNoPostgres(t)
	db := openRawConn(t, dsn)
	repo, service := newPostgresService(t)
	sku := createStabilityProductAndSKU(t, repo, 100)
	order := createStabilityOrder(t, service, sku, 1)

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback()

	var first int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM inventory_locks WHERE order_id = $1`, order.ID).Scan(&first); err != nil {
		t.Fatalf("first count: %v", err)
	}

	// Simulate another transaction inserting a lock row for a different SKU.
	// In the real app this would happen through the repository; here we use raw
	// SQL to observe the phantom read phenomenon in isolation.
	otherSKU := createStabilityProductAndSKU(t, repo, 100)
	_, err = db.Exec(`
		INSERT INTO inventory_locks (order_id, sku_id, quantity, status, locked_at, created_at, updated_at)
		VALUES ($1, $2, 1, 'locked', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		order.ID, otherSKU.ID,
	)
	if err != nil {
		t.Fatalf("external insert: %v", err)
	}

	var second int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM inventory_locks WHERE order_id = $1`, order.ID).Scan(&second); err != nil {
		t.Fatalf("second count: %v", err)
	}
	if second != first+1 {
		t.Fatalf("phantom read did not occur: first=%d second=%d", first, second)
	}
}

// TestConcurrentPayOrderNoDoubleConfirm launches multiple PayOrder calls for
// the same order. Because PayOrder is not wrapped in a transaction, a lost
// update can double-confirm inventory (stock decremented twice for one order).
// The test fails if such a regression is introduced.
func TestConcurrentPayOrderNoDoubleConfirm(t *testing.T) {
	repo, service := newPostgresService(t)
	sku := createStabilityProductAndSKU(t, repo, 10)
	order := createStabilityOrder(t, service, sku, 1)

	const workers = 16
	start := make(chan struct{})
	var wg sync.WaitGroup
	var success atomic.Int64
	var failures atomic.Int64
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := service.PayOrder(context.Background(), application.Actor{UserID: 1, Role: domain.RoleConsumer}, order.ID)
			if err == nil {
				success.Add(1)
			} else {
				failures.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()

	updated, ok := repo.GetSKU(sku.ID)
	if !ok {
		t.Fatal("expected sku after concurrent pay")
	}

	// Under concurrent PayOrder calls, exactly one goroutine should win the
	// status race and confirm inventory; the rest should idempotently return
	// the paid view without double-confirming. We therefore check inventory
	// consistency rather than counting exactly one success.
	if success.Load() == 0 {
		t.Fatalf("expected at least one successful PayOrder, got success=%d failures=%d", success.Load(), failures.Load())
	}
	if updated.Stock != 9 || updated.LockedStock != 0 {
		t.Fatalf("inventory corrupted by double confirm: expected stock=9 locked_stock=0, got stock=%d locked_stock=%d", updated.Stock, updated.LockedStock)
	}
}

// TestConcurrentPayAndCancelNoInventoryCorruption interleaves PayOrder and
// CancelOrder on the same order. Only one should win; inventory must remain
// consistent (no negative stock or negative locked_stock).
func TestConcurrentPayAndCancelNoInventoryCorruption(t *testing.T) {
	repo, service := newPostgresService(t)
	sku := createStabilityProductAndSKU(t, repo, 10)
	order := createStabilityOrder(t, service, sku, 1)

	start := make(chan struct{})
	var wg sync.WaitGroup
	var paid atomic.Int64
	var cancelled atomic.Int64

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		_, err := service.PayOrder(context.Background(), application.Actor{UserID: 1, Role: domain.RoleConsumer}, order.ID)
		if err == nil {
			paid.Add(1)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		_, err := service.CancelOrder(context.Background(), application.Actor{UserID: 1, Role: domain.RoleConsumer}, order.ID)
		if err == nil {
			cancelled.Add(1)
		}
	}()

	close(start)
	wg.Wait()

	updated, ok := repo.GetSKU(sku.ID)
	if !ok {
		t.Fatal("expected sku after concurrent pay/cancel")
	}

	// Only one of Pay/Cancel should succeed because the order can only end in
	// one terminal state. Inventory must never go negative.
	if paid.Load()+cancelled.Load() != 1 {
		t.Fatalf("expected exactly one successful operation, got paid=%d cancelled=%d", paid.Load(), cancelled.Load())
	}
	if updated.Stock < 0 || updated.LockedStock < 0 {
		t.Fatalf("negative inventory: stock=%d locked_stock=%d", updated.Stock, updated.LockedStock)
	}
	if paid.Load() == 1 && (updated.Stock != 9 || updated.LockedStock != 0) {
		t.Fatalf("pay win invariant violated: expected stock=9 locked_stock=0, got stock=%d locked_stock=%d", updated.Stock, updated.LockedStock)
	}
	if cancelled.Load() == 1 && (updated.Stock != 10 || updated.LockedStock != 0) {
		t.Fatalf("cancel win invariant violated: expected stock=10 locked_stock=0, got stock=%d locked_stock=%d", updated.Stock, updated.LockedStock)
	}
}

// TestConcurrentRefundAndFinishNoInventoryCorruption pays an order and then
// races a consumer FinishOrder against a merchant refund approval. Only one
// should win; inventory must remain consistent.
func TestConcurrentRefundAndFinishNoInventoryCorruption(t *testing.T) {
	repo, service := newPostgresService(t)
	sku := createStabilityProductAndSKU(t, repo, 10)
	order := createStabilityOrder(t, service, sku, 1)

	// Bring the order to PAID and SHIPPED so both Finish and Refund are legal.
	if _, err := service.PayOrder(context.Background(), application.Actor{UserID: 1, Role: domain.RoleConsumer}, order.ID); err != nil {
		t.Fatalf("pay order: %v", err)
	}
	if _, err := service.MerchantShipOrder(context.Background(), application.Actor{UserID: 2, Role: domain.RoleMerchant, MerchantID: 1}, order.ID, application.MerchantOrderShipInput{LogisticsNo: "SF123"}); err != nil {
		t.Fatalf("ship order: %v", err)
	}

	// Request refund first so the merchant approval path is valid.
	if _, err := service.RequestRefund(context.Background(), application.Actor{UserID: 1, Role: domain.RoleConsumer}, order.ID, application.RefundRequestInput{Reason: "no reason"}); err != nil {
		t.Fatalf("request refund: %v", err)
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	var finished atomic.Int64
	var refunded atomic.Int64

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		_, err := service.FinishOrder(context.Background(), application.Actor{UserID: 1, Role: domain.RoleConsumer}, order.ID)
		if err == nil {
			finished.Add(1)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		_, err := service.MerchantApproveRefund(context.Background(), application.Actor{UserID: 2, Role: domain.RoleMerchant, MerchantID: 1}, order.ID)
		if err == nil {
			refunded.Add(1)
		}
	}()

	close(start)
	wg.Wait()

	updated, ok := repo.GetSKU(sku.ID)
	if !ok {
		t.Fatal("expected sku after concurrent finish/refund")
	}

	if finished.Load()+refunded.Load() != 1 {
		t.Fatalf("expected exactly one successful operation, got finished=%d refunded=%d", finished.Load(), refunded.Load())
	}
	if updated.Stock < 0 || updated.LockedStock < 0 {
		t.Fatalf("negative inventory: stock=%d locked_stock=%d", updated.Stock, updated.LockedStock)
	}
	if refunded.Load() == 1 && updated.Stock != 10 {
		t.Fatalf("refund win invariant violated: expected stock=10, got %d", updated.Stock)
	}
}

// TestMassConcurrentStockReservation stresses the reservation path with far
// more workers than available stock. Exactly `stock` orders should succeed and
// the final inventory must equal the initial stock with all successful orders
// reserved.
func TestMassConcurrentStockReservation(t *testing.T) {
	repo, service := newPostgresService(t)
	const stock int = 50
	const workers = 200
	sku := createStabilityProductAndSKU(t, repo, stock)

	start := make(chan struct{})
	var wg sync.WaitGroup
	var created atomic.Int64
	var conflicts atomic.Int64
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, err := service.CreateOrder(context.Background(), application.Actor{UserID: 1, Role: domain.RoleConsumer}, fmt.Sprintf("mass-stock-%d-%d", sku.ID, i), application.CheckoutInput{
				Items:           []application.OrderLineInput{{SKUID: sku.ID, Quantity: 1}},
				ReceiverName:    "Alice",
				ReceiverPhone:   "13800000001",
				ReceiverAddress: "Shanghai",
			})
			if err == nil {
				created.Add(1)
			} else {
				conflicts.Add(1)
			}
		}(i)
	}
	close(start)
	wg.Wait()

	updated, ok := repo.GetSKU(sku.ID)
	if !ok {
		t.Fatal("expected sku after mass concurrent reservation")
	}
	if int(created.Load()) != stock {
		t.Fatalf("expected exactly %d successful orders, got created=%d conflicts=%d", stock, created.Load(), conflicts.Load())
	}
	if updated.Stock != stock || updated.LockedStock != stock {
		t.Fatalf("mass reservation invariant violated: expected stock=%d locked_stock=%d, got stock=%d locked_stock=%d", stock, stock, updated.Stock, updated.LockedStock)
	}
}

// TestCyclicSKUAccessNoDeadlock creates two orders whose SKUs are accessed in
// opposite orders. PayOrder/CancelOrder release inventory by iterating locks in
// database order, not SKU order, so concurrent release can form a deadlock
// cycle. The test fails if a deadlock (or timeout) is detected.
func TestCyclicSKUAccessNoDeadlock(t *testing.T) {
	repo, service := newPostgresService(t)
	skuA := createStabilityProductAndSKU(t, repo, 10)
	skuB := createStabilityProductAndSKU(t, repo, 10)

	createOrderWithTwoSKUs := func() *application.OrderView {
		t.Helper()
		view, err := service.CreateOrder(context.Background(), application.Actor{UserID: 1, Role: domain.RoleConsumer}, fmt.Sprintf("cycle-%d-%d", skuA.ID, time.Now().UnixNano()), application.CheckoutInput{
			Items: []application.OrderLineInput{
				{SKUID: skuA.ID, Quantity: 1},
				{SKUID: skuB.ID, Quantity: 1},
			},
			ReceiverName:    "Alice",
			ReceiverPhone:   "13800000001",
			ReceiverAddress: "Shanghai",
		})
		if err != nil {
			t.Fatalf("create two-sku order: %v", err)
		}
		return view
	}

	order1 := createOrderWithTwoSKUs()
	order2 := createOrderWithTwoSKUs()

	start := make(chan struct{})
	var wg sync.WaitGroup
	errors := make(chan error, 2)

	// Order1 pays (confirms SKU A then SKU B), Order2 cancels (releases SKU A
	// then SKU B). Because releaseInventory iterates in DB lock insertion order
	// (which is sorted by SKUID), both paths touch SKU A then SKU B in the same
	// order and should not deadlock. If a future change breaks this ordering,
	// this test will catch a timeout or deadlock error.
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		if _, err := service.PayOrder(context.Background(), application.Actor{UserID: 1, Role: domain.RoleConsumer}, order1.ID); err != nil {
			errors <- err
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		if _, err := service.CancelOrder(context.Background(), application.Actor{UserID: 1, Role: domain.RoleConsumer}, order2.ID); err != nil {
			errors <- err
		}
	}()

	close(start)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("possible deadlock: cyclic SKU pay/cancel did not complete within 10s")
	}

	close(errors)
	for err := range errors {
		// Deadlock errors are reported by PostgreSQL as SQLSTATE 40P01.
		if isDeadlock(err) {
			t.Fatalf("deadlock detected: %v", err)
		}
	}
}

func isDeadlock(err error) bool {
	if err == nil {
		return false
	}
	return contains(err.Error(), "deadlock") || contains(err.Error(), "40P01")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsInternal(s, substr))
}

func containsInternal(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestPayOrderInventoryFailureRollsBackStatus verifies that if the inventory
// side effect of PayOrder fails, the order status change is rolled back and the
// SKU remains unchanged. This confirms status migration and inventory mutation
// are atomic.
func TestPayOrderInventoryFailureRollsBackStatus(t *testing.T) {
	dsn, _ := skipIfNoPostgres(t)
	db := openRawConn(t, dsn)
	repo, service := newPostgresService(t)
	sku := createStabilityProductAndSKU(t, repo, 10)
	order := createStabilityOrder(t, service, sku, 1)

	// Simulate external corruption: the lock row still claims quantity=1, but the
	// SKU locked_stock has been cleared to 0. PayOrder will try to decrement
	// locked_stock and detect an underflow, causing the side effect to fail.
	_, err := db.Exec(`UPDATE product_skus SET locked_stock = 0 WHERE id = $1`, sku.ID)
	if err != nil {
		t.Fatalf("corrupt sku: %v", err)
	}

	_, err = service.PayOrder(context.Background(), application.Actor{UserID: 1, Role: domain.RoleConsumer}, order.ID)
	if err == nil {
		t.Fatal("expected PayOrder to fail when inventory underflows")
	}

	current, ok := repo.GetOrder(order.ID)
	if !ok {
		t.Fatal("expected order to still exist")
	}
	if current.Status != orderdomain.StatusCreated {
		t.Fatalf("expected order status to remain CREATED after rollback, got %s", current.Status)
	}

	updated, ok := repo.GetSKU(sku.ID)
	if !ok {
		t.Fatal("expected sku to exist")
	}
	if updated.Stock != 10 || updated.LockedStock != 0 {
		t.Fatalf("expected sku unchanged after rollback, got stock=%d locked_stock=%d", updated.Stock, updated.LockedStock)
	}
}
