package postgres

import (
	"testing"
)

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
