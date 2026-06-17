package postgres

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	orderdomain "github.com/example/redcart-copilot/backend/internal/order/domain"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
)

type scanFunc func(dest ...any) error

func (f scanFunc) Scan(dest ...any) error {
	return f(dest...)
}

func TestScanHelpersDecodeDatabaseRows(t *testing.T) {
	now := time.Date(2026, 6, 6, 10, 0, 0, 0, time.UTC)
	later := now.Add(time.Hour)

	product, err := scanProduct(scanFunc(func(dest ...any) error {
		*(dest[0].(*int64)) = 1
		*(dest[1].(*int64)) = 2
		*(dest[2].(*string)) = "Travel Organizer"
		*(dest[3].(*string)) = "Portable storage"
		*(dest[4].(*string)) = "https://images.example.com/organizer.jpg"
		*(dest[5].(*int64)) = 102
		*(dest[6].(*string)) = domain.ProductStatusOnline
		*(dest[7].(*[]byte)) = []byte(`["portable","washable"]`)
		*(dest[8].(*time.Time)) = now
		*(dest[9].(*time.Time)) = later
		return nil
	}))
	if err != nil {
		t.Fatalf("scan product: %v", err)
	}
	if product.ID != 1 || product.MerchantID != 2 || len(product.SellingPoints) != 2 {
		t.Fatalf("unexpected product scan result: %+v", product)
	}

	sku, err := scanSKU(scanFunc(func(dest ...any) error {
		*(dest[0].(*int64)) = 11
		*(dest[1].(*int64)) = product.ID
		*(dest[2].(*string)) = "Cream White"
		*(dest[3].(*[]byte)) = []byte(`{"color":"cream","size":"standard"}`)
		*(dest[4].(*int64)) = 8900
		*(dest[5].(*int)) = 40
		*(dest[6].(*int)) = 3
		*(dest[7].(*string)) = domain.SKUStatusActive
		*(dest[8].(*time.Time)) = now
		*(dest[9].(*time.Time)) = later
		return nil
	}))
	if err != nil {
		t.Fatalf("scan sku: %v", err)
	}
	if sku.ID != 11 || sku.SKUAttrs["color"] != "cream" || sku.LockedStock != 3 {
		t.Fatalf("unexpected sku scan result: %+v", sku)
	}

	order, err := scanOrder(scanFunc(func(dest ...any) error {
		*(dest[0].(*int64)) = 21
		*(dest[1].(*string)) = "RC202606060001"
		*(dest[2].(*int64)) = 1
		*(dest[3].(*int64)) = 2
		*(dest[4].(*string)) = string(orderdomain.StatusShipped)
		*(dest[5].(*int64)) = 8900
		*(dest[6].(*int64)) = 8900
		*(dest[7].(*int64)) = 0
		*(dest[8].(*string)) = "idem-001"
		*(dest[9].(*string)) = "Alice"
		*(dest[10].(*string)) = "13800000001"
		*(dest[11].(*string)) = "Shanghai"
		*(dest[12].(*sql.NullTime)) = sql.NullTime{Time: now, Valid: true}
		*(dest[13].(*sql.NullTime)) = sql.NullTime{}
		*(dest[14].(*sql.NullTime)) = sql.NullTime{Time: later, Valid: true}
		*(dest[15].(*sql.NullTime)) = sql.NullTime{}
		*(dest[16].(*time.Time)) = now
		*(dest[17].(*time.Time)) = later
		return nil
	}))
	if err != nil {
		t.Fatalf("scan order: %v", err)
	}
	if order.ID != 21 || order.Status != orderdomain.StatusShipped || order.PaidAt == nil || order.CancelledAt != nil || order.ShippedAt == nil {
		t.Fatalf("unexpected order scan result: %+v", order)
	}

	event, err := scanOrderEvent(scanFunc(func(dest ...any) error {
		*(dest[0].(*int64)) = 31
		*(dest[1].(*int64)) = order.ID
		*(dest[2].(*sql.NullString)) = sql.NullString{String: string(orderdomain.StatusPaid), Valid: true}
		*(dest[3].(*string)) = string(orderdomain.StatusShipped)
		*(dest[4].(*string)) = "ORDER_SHIPPED"
		*(dest[5].(*int64)) = 2
		*(dest[6].(*string)) = domain.RoleMerchant
		*(dest[7].(*string)) = "shipped"
		*(dest[8].(*time.Time)) = later
		return nil
	}))
	if err != nil {
		t.Fatalf("scan order event: %v", err)
	}
	if event.ID != 31 || event.FromStatus != string(orderdomain.StatusPaid) || event.ToStatus != string(orderdomain.StatusShipped) {
		t.Fatalf("unexpected order event scan result: %+v", event)
	}

	lock, err := scanInventoryLock(scanFunc(func(dest ...any) error {
		*(dest[0].(*int64)) = 41
		*(dest[1].(*int64)) = order.ID
		*(dest[2].(*int64)) = sku.ID
		*(dest[3].(*int)) = 1
		*(dest[4].(*string)) = domain.InventoryLockStatusConfirmed
		*(dest[5].(*time.Time)) = now
		*(dest[6].(*sql.NullTime)) = sql.NullTime{Time: later, Valid: true}
		*(dest[7].(*sql.NullTime)) = sql.NullTime{}
		*(dest[8].(*time.Time)) = now
		*(dest[9].(*time.Time)) = later
		return nil
	}))
	if err != nil {
		t.Fatalf("scan inventory lock: %v", err)
	}
	if lock.ID != 41 || lock.ConfirmedAt == nil || lock.ReleasedAt != nil {
		t.Fatalf("unexpected inventory lock scan result: %+v", lock)
	}

	behavior, err := scanBehaviorEvent(scanFunc(func(dest ...any) error {
		*(dest[0].(*int64)) = 51
		*(dest[1].(*sql.NullInt64)) = sql.NullInt64{Int64: 1, Valid: true}
		*(dest[2].(*string)) = domain.BehaviorOrderPay
		*(dest[3].(*sql.NullInt64)) = sql.NullInt64{}
		*(dest[4].(*sql.NullInt64)) = sql.NullInt64{Int64: product.ID, Valid: true}
		*(dest[5].(*sql.NullInt64)) = sql.NullInt64{Int64: sku.ID, Valid: true}
		*(dest[6].(*sql.NullInt64)) = sql.NullInt64{Int64: order.ID, Valid: true}
		*(dest[7].(*sql.NullInt64)) = sql.NullInt64{Int64: product.MerchantID, Valid: true}
		*(dest[8].(*time.Time)) = later
		return nil
	}))
	if err != nil {
		t.Fatalf("scan behavior event: %v", err)
	}
	if behavior.ID != 51 || behavior.EventType != domain.BehaviorOrderPay || behavior.OrderID != order.ID {
		t.Fatalf("unexpected behavior event scan result: %+v", behavior)
	}

	task, err := scanAITask(scanFunc(func(dest ...any) error {
		*(dest[0].(*int64)) = 61
		*(dest[1].(*sql.NullInt64)) = sql.NullInt64{Int64: 2, Valid: true}
		*(dest[2].(*sql.NullInt64)) = sql.NullInt64{Int64: product.MerchantID, Valid: true}
		*(dest[3].(*string)) = domain.TaskTypeBusinessReview
		*(dest[4].(*[]byte)) = []byte(`{"window_days":7}`)
		*(dest[5].(*[]byte)) = []byte(`{"diagnosis":"ok"}`)
		*(dest[6].(*string)) = domain.AITaskStatusCompleted
		*(dest[7].(*string)) = ""
		*(dest[8].(*time.Time)) = now
		*(dest[9].(*time.Time)) = later
		return nil
	}))
	if err != nil {
		t.Fatalf("scan ai task: %v", err)
	}
	if task.ID != 61 || task.MerchantID != product.MerchantID || task.Input["window_days"].(float64) != 7 || task.Output["diagnosis"] != "ok" {
		t.Fatalf("unexpected ai task scan result: %+v", task)
	}
}

func TestScanHelpersReturnScannerErrors(t *testing.T) {
	expected := errors.New("scan failed")
	scanner := scanFunc(func(dest ...any) error { return expected })

	if _, err := scanProduct(scanner); !errors.Is(err, expected) {
		t.Fatalf("expected product scan error, got %v", err)
	}
	if _, err := scanSKU(scanner); !errors.Is(err, expected) {
		t.Fatalf("expected sku scan error, got %v", err)
	}
	if _, err := scanOrder(scanner); !errors.Is(err, expected) {
		t.Fatalf("expected order scan error, got %v", err)
	}
	if _, err := scanOrderEvent(scanner); !errors.Is(err, expected) {
		t.Fatalf("expected order event scan error, got %v", err)
	}
	if _, err := scanInventoryLock(scanner); !errors.Is(err, expected) {
		t.Fatalf("expected inventory lock scan error, got %v", err)
	}
	if _, err := scanBehaviorEvent(scanner); !errors.Is(err, expected) {
		t.Fatalf("expected behavior event scan error, got %v", err)
	}
	if _, err := scanAITask(scanner); !errors.Is(err, expected) {
		t.Fatalf("expected ai task scan error, got %v", err)
	}
}

func TestPostgresNullableHelpersAndMigrationResolution(t *testing.T) {
	now := time.Date(2026, 6, 6, 10, 0, 0, 0, time.UTC)

	if nullTimePtr(sql.NullTime{}) != nil {
		t.Fatal("expected invalid sql null time to become nil")
	}
	if got := nullTimePtr(sql.NullTime{Time: now, Valid: true}); got == nil || !got.Equal(now) {
		t.Fatalf("expected valid sql null time pointer, got %v", got)
	}
	if nullTime(time.Time{}) != nil {
		t.Fatal("expected zero time to become nil")
	}
	if nullTime(now) != now {
		t.Fatal("expected non-zero time to pass through")
	}
	if nullableString("") != nil || nullableString("x") != "x" {
		t.Fatal("unexpected nullable string result")
	}
	if nullInt64(0) != nil || nullInt64(42) != int64(42) {
		t.Fatal("unexpected nullable int64 result")
	}
	if nullableJSON(nil) != nil || nullableJSON([]byte("null")) != nil || nullableJSON([]byte(`{"ok":true}`)) != `{"ok":true}` {
		t.Fatal("unexpected nullable json result")
	}

	result := gormResult{rowsAffected: 3}
	if rows, err := result.RowsAffected(); err != nil || rows != 3 {
		t.Fatalf("expected rows affected, got rows=%d err=%v", rows, err)
	}
	if _, err := result.LastInsertId(); err == nil {
		t.Fatal("expected unsupported last insert id error")
	}

	if seededPasswordHash("consumer-demo") == seededPasswordHash("merchant-demo") {
		t.Fatal("expected seeded password hash to depend on password")
	}
	t.Setenv("REDCART_POSTGRES_TEST_ENV", "configured")
	if envOrDefault("REDCART_POSTGRES_TEST_ENV", "fallback") != "configured" {
		t.Fatal("expected configured env value")
	}
	t.Setenv("REDCART_POSTGRES_TEST_ENV", "")
	if envOrDefault("REDCART_POSTGRES_TEST_ENV", "fallback") != "fallback" {
		t.Fatal("expected fallback env value")
	}

	dir := t.TempDir()
	migration := filepath.Join(dir, "0001_init_schema.sql")
	if err := os.WriteFile(migration, []byte("-- test migration"), 0o600); err != nil {
		t.Fatalf("write migration fixture: %v", err)
	}
	t.Setenv("MIGRATIONS_DIR", dir)
	resolvedDir, err := resolveMigrationsDir()
	if err != nil {
		t.Fatalf("resolve migrations dir: %v", err)
	}
	if resolvedDir != dir {
		t.Fatalf("expected %s, got %s", dir, resolvedDir)
	}
	files, err := listMigrationFiles(resolvedDir)
	if err != nil {
		t.Fatalf("list migration files: %v", err)
	}
	if len(files) != 1 || files[0] != migration {
		t.Fatalf("expected [%s], got %v", migration, files)
	}
}
