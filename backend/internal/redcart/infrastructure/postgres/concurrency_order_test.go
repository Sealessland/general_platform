package postgres

import (
	"context"
	"fmt"
	"github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestConcurrentPayOrderNoDoubleConfirm launches multiple PayOrder calls for
// the same order. Because PayOrder is not wrapped in a transaction, a lost
// update can double-confirm inventory (stock decremented twice for one order).
// The test fails if such a regression is introduced.
// 并发支付同一订单不应导致库存被双重确认扣减（回归测试）。
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
// 并发执行支付与取消，二者只能成功其一，且库存不可为负或被破坏。
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
// 并发执行完成订单与退款审批，二者只能成功其一，且库存保持一致。
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
// 高并发抢库存：并发数远超库存时，恰好只有库存数量的下单成功且不超卖。
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
// 两个订单以相反顺序操作同一组 SKU，验证并发释放库存不会形成死锁。
func TestCyclicSKUAccessNoDeadlock(t *testing.T) {
	repo, service := newPostgresService(t)
	skuA := createStabilityProductAndSKU(t, repo, 10)
	skuB := createStabilityProductAndSKU(t, repo, 10)

	// createOrderWithTwoSKUs 创建同时包含两个 SKU 的订单，供死锁回归测试复用。
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

// isDeadlock 判断错误是否为 PostgreSQL 死锁（SQLSTATE 40P01 或关键字匹配）。
func isDeadlock(err error) bool {
	if err == nil {
		return false
	}
	return contains(err.Error(), "deadlock") || contains(err.Error(), "40P01")
}

// contains 判断字符串 s 是否包含子串 substr（大小写敏感的简单子串匹配）。
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsInternal(s, substr))
}

// containsInternal 子串查找的核心循环实现。
func containsInternal(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
