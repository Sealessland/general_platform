package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"sync"
	"time"

	orderdomain "github.com/example/redcart-copilot/backend/internal/order/domain"
	"github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
)

type Repository struct {
	mu sync.RWMutex

	nextUserID          int64
	nextMerchantID      int64
	nextNoteID          int64
	nextProductID       int64
	nextSKUID           int64
	nextCartItemID      int64
	nextOrderID         int64
	nextOrderItemID     int64
	nextOrderEventID    int64
	nextInventoryLockID int64
	nextBehaviorEventID int64
	nextAITaskID        int64

	users              map[int64]domain.User
	usersByPhone       map[string]int64
	sessions           map[string]int64
	merchants          map[int64]domain.Merchant
	merchantsByUserID  map[int64]int64
	notes              map[int64]domain.Note
	products           map[int64]domain.Product
	skus               map[int64]domain.SKU
	cartItemsByUser    map[int64]map[int64]domain.CartItem
	orders             map[int64]domain.Order
	orderByIdempotency map[string]int64
	orderEventsByOrder map[int64][]domain.OrderEvent
	locksByOrder       map[int64][]domain.InventoryLock
	behaviorEvents     []domain.BehaviorEvent
	aiTasks            map[int64]domain.AIGenerationTask
}

var _ application.Repository = (*Repository)(nil)

func NewRepository() *Repository {
	repo := &Repository{
		users:              make(map[int64]domain.User),
		usersByPhone:       make(map[string]int64),
		sessions:           make(map[string]int64),
		merchants:          make(map[int64]domain.Merchant),
		merchantsByUserID:  make(map[int64]int64),
		notes:              make(map[int64]domain.Note),
		products:           make(map[int64]domain.Product),
		skus:               make(map[int64]domain.SKU),
		cartItemsByUser:    make(map[int64]map[int64]domain.CartItem),
		orders:             make(map[int64]domain.Order),
		orderByIdempotency: make(map[string]int64),
		orderEventsByOrder: make(map[int64][]domain.OrderEvent),
		locksByOrder:       make(map[int64][]domain.InventoryLock),
		behaviorEvents:     make([]domain.BehaviorEvent, 0),
		aiTasks:            make(map[int64]domain.AIGenerationTask),
	}
	repo.seed()
	return repo
}

func (r *Repository) seed() {
	now := time.Now().UTC()

	consumer, _ := r.CreateUser(domain.User{
		Nickname:     "Alice",
		Phone:        "13800000001",
		PasswordHash: seededPasswordHash("consumer-demo"),
		Role:         domain.RoleConsumer,
		CreatedAt:    now,
		UpdatedAt:    now,
	})

	merchantUser, _ := r.CreateUser(domain.User{
		Nickname:     "Merchant Zoe",
		Phone:        "13800000002",
		PasswordHash: seededPasswordHash("merchant-demo"),
		Role:         domain.RoleMerchant,
		CreatedAt:    now,
		UpdatedAt:    now,
	})

	merchant, _ := r.CreateMerchant(domain.Merchant{
		UserID:      merchantUser.ID,
		Name:        "RedCart Beauty Lab",
		Description: "Content-driven beauty merchant demo account",
		Status:      "active",
		CreatedAt:   now,
		UpdatedAt:   now,
	})

	productOne, _ := r.SaveProduct(domain.Product{
		MerchantID:    merchant.ID,
		Title:         "Velvet Lip Mud Set",
		Description:   "Matte lip set for commute, campus, and quick content shoots.",
		CoverURL:      "https://images.example.com/lip-mud.jpg",
		CategoryID:    101,
		Status:        domain.ProductStatusOnline,
		SellingPoints: []string{"Soft matte finish", "Pocket-size touch-up", "Daily shade bundle"},
		CreatedAt:     now,
		UpdatedAt:     now,
	})

	productTwo, _ := r.SaveProduct(domain.Product{
		MerchantID:    merchant.ID,
		Title:         "Travel Makeup Organizer",
		Description:   "Portable makeup storage for dorm, travel, and desk organization.",
		CoverURL:      "https://images.example.com/makeup-organizer.jpg",
		CategoryID:    102,
		Status:        domain.ProductStatusOnline,
		SellingPoints: []string{"Compartment layout", "Portable and lightweight", "Easy to clean"},
		CreatedAt:     now,
		UpdatedAt:     now,
	})

	skuOne, _ := r.SaveSKU(domain.SKU{
		ProductID:   productOne.ID,
		SKUName:     "Cherry Set",
		SKUAttrs:    map[string]string{"shade": "cherry", "pack": "3pcs"},
		PriceCent:   12900,
		Stock:       30,
		LockedStock: 0,
		Status:      domain.SKUStatusActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	})

	skuTwo, _ := r.SaveSKU(domain.SKU{
		ProductID:   productOne.ID,
		SKUName:     "Rose Set",
		SKUAttrs:    map[string]string{"shade": "rose", "pack": "3pcs"},
		PriceCent:   13900,
		Stock:       18,
		LockedStock: 0,
		Status:      domain.SKUStatusActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	})

	skuThree, _ := r.SaveSKU(domain.SKU{
		ProductID:   productTwo.ID,
		SKUName:     "Cream White",
		SKUAttrs:    map[string]string{"color": "cream", "size": "standard"},
		PriceCent:   8900,
		Stock:       40,
		LockedStock: 0,
		Status:      domain.SKUStatusActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	})

	_, _ = r.SaveCartItem(domain.CartItem{
		UserID:    consumer.ID,
		ProductID: productOne.ID,
		SKUID:     skuOne.ID,
		Quantity:  1,
		Selected:  true,
		CreatedAt: now,
		UpdatedAt: now,
	})

	_ = skuTwo

	noteOne := domain.Note{
		ID:         r.nextID(&r.nextNoteID),
		AuthorID:   merchantUser.ID,
		Title:      "通勤妆 5 分钟出门组合",
		Content:    "这套唇泥和整理盒是我最近拍通勤内容最常带的组合，颜色稳、补妆快、包里不乱。",
		CoverURL:   "https://images.example.com/note-commute.jpg",
		Status:     "published",
		ViewCount:  1280,
		LikeCount:  218,
		ProductIDs: []int64{productOne.ID, productTwo.ID},
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	r.notes[noteOne.ID] = cloneNote(noteOne)

	noteTwo := domain.Note{
		ID:         r.nextID(&r.nextNoteID),
		AuthorID:   merchantUser.ID,
		Title:      "宿舍桌面整理前后对比",
		Content:    "桌面一乱，化妆和出门效率都会掉。这个整理盒适合小桌面。",
		CoverURL:   "https://images.example.com/note-dorm.jpg",
		Status:     "published",
		ViewCount:  920,
		LikeCount:  141,
		ProductIDs: []int64{productTwo.ID},
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	r.notes[noteTwo.ID] = cloneNote(noteTwo)

	events := []domain.BehaviorEvent{
		{UserID: consumer.ID, EventType: domain.BehaviorNoteView, NoteID: noteOne.ID, MerchantID: merchant.ID, CreatedAt: now},
		{UserID: consumer.ID, EventType: domain.BehaviorNoteView, NoteID: noteTwo.ID, MerchantID: merchant.ID, CreatedAt: now},
		{UserID: consumer.ID, EventType: domain.BehaviorProductClick, ProductID: productOne.ID, MerchantID: merchant.ID, CreatedAt: now},
		{UserID: consumer.ID, EventType: domain.BehaviorProductClick, ProductID: productTwo.ID, MerchantID: merchant.ID, CreatedAt: now},
		{UserID: consumer.ID, EventType: domain.BehaviorAddToCart, ProductID: productOne.ID, SKUID: skuOne.ID, MerchantID: merchant.ID, CreatedAt: now},
		{UserID: consumer.ID, EventType: domain.BehaviorAddToCart, ProductID: productTwo.ID, SKUID: skuThree.ID, MerchantID: merchant.ID, CreatedAt: now},
	}
	for _, event := range events {
		_, _ = r.AppendBehaviorEvent(event)
	}

	r.seedOrderCreated(now.Add(-4*time.Hour), consumer.ID, merchant.ID, productOne, skuTwo, 1)
	r.seedOrderShipped(now.Add(-48*time.Hour), consumer.ID, merchant.ID, productTwo, skuThree, 1)
	r.seedOrderRefunding(now.Add(-24*time.Hour), consumer.ID, merchant.ID, productOne, skuOne, 1)
}

func (r *Repository) nextID(counter *int64) int64 {
	*counter = *counter + 1
	return *counter
}

func seededPasswordHash(password string) string {
	sum := sha256.Sum256([]byte(password))
	return hex.EncodeToString(sum[:])
}

func (r *Repository) seedOrderCreated(base time.Time, userID, merchantID int64, product domain.Product, sku domain.SKU, quantity int) {
	order, _ := r.SaveOrder(domain.Order{
		OrderNo:            fmt.Sprintf("RCSEEDC%06d", sku.ID),
		UserID:             userID,
		MerchantID:         merchantID,
		Status:             "CREATED",
		TotalAmountCent:    int64(quantity) * sku.PriceCent,
		PayAmountCent:      int64(quantity) * sku.PriceCent,
		DiscountAmountCent: 0,
		IdempotencyKey:     fmt.Sprintf("seed-created-%d", sku.ID),
		ReceiverName:       "Alice",
		ReceiverPhone:      "13800000001",
		ReceiverAddress:    "Shanghai Xuhui District",
		CreatedAt:          base,
		UpdatedAt:          base,
		Items: []domain.OrderItem{
			{
				ProductID:            product.ID,
				SKUID:                sku.ID,
				ProductTitleSnapshot: product.Title,
				SKUNameSnapshot:      sku.SKUName,
				PriceCentSnapshot:    sku.PriceCent,
				Quantity:             quantity,
				TotalAmountCent:      int64(quantity) * sku.PriceCent,
				CreatedAt:            base,
				UpdatedAt:            base,
			},
		},
	})
	sku.LockedStock += quantity
	_, _ = r.SaveSKU(sku)
	_, _ = r.SaveInventoryLock(domain.InventoryLock{
		OrderID:   order.ID,
		SKUID:     sku.ID,
		Quantity:  quantity,
		Status:    domain.InventoryLockStatusLocked,
		LockedAt:  base,
		CreatedAt: base,
		UpdatedAt: base,
	})
	_, _ = r.AppendOrderEvent(domain.OrderEvent{
		OrderID:      order.ID,
		ToStatus:     "CREATED",
		EventType:    "ORDER_CREATED",
		OperatorID:   userID,
		OperatorRole: domain.RoleConsumer,
		Remark:       "seeded created order",
		CreatedAt:    base,
	})
	_, _ = r.AppendBehaviorEvent(domain.BehaviorEvent{
		UserID:     userID,
		EventType:  domain.BehaviorOrderCreate,
		ProductID:  product.ID,
		SKUID:      sku.ID,
		OrderID:    order.ID,
		MerchantID: merchantID,
		CreatedAt:  base,
	})
}

func (r *Repository) seedOrderShipped(base time.Time, userID, merchantID int64, product domain.Product, sku domain.SKU, quantity int) {
	paidAt := base.Add(30 * time.Minute)
	shippedAt := base.Add(90 * time.Minute)
	order, _ := r.SaveOrder(domain.Order{
		OrderNo:            fmt.Sprintf("RCSEEDS%06d", sku.ID),
		UserID:             userID,
		MerchantID:         merchantID,
		Status:             "SHIPPED",
		TotalAmountCent:    int64(quantity) * sku.PriceCent,
		PayAmountCent:      int64(quantity) * sku.PriceCent,
		DiscountAmountCent: 0,
		IdempotencyKey:     fmt.Sprintf("seed-shipped-%d", sku.ID),
		ReceiverName:       "Alice",
		ReceiverPhone:      "13800000001",
		ReceiverAddress:    "Hangzhou Binjiang",
		PaidAt:             &paidAt,
		ShippedAt:          &shippedAt,
		CreatedAt:          base,
		UpdatedAt:          shippedAt,
		Items: []domain.OrderItem{
			{
				ProductID:            product.ID,
				SKUID:                sku.ID,
				ProductTitleSnapshot: product.Title,
				SKUNameSnapshot:      sku.SKUName,
				PriceCentSnapshot:    sku.PriceCent,
				Quantity:             quantity,
				TotalAmountCent:      int64(quantity) * sku.PriceCent,
				CreatedAt:            base,
				UpdatedAt:            shippedAt,
			},
		},
	})
	sku.Stock -= quantity
	_, _ = r.SaveSKU(sku)
	_, _ = r.SaveInventoryLock(domain.InventoryLock{
		OrderID:     order.ID,
		SKUID:       sku.ID,
		Quantity:    quantity,
		Status:      domain.InventoryLockStatusConfirmed,
		LockedAt:    base,
		ConfirmedAt: &paidAt,
		CreatedAt:   base,
		UpdatedAt:   shippedAt,
	})
	_, _ = r.AppendOrderEvent(domain.OrderEvent{OrderID: order.ID, ToStatus: "CREATED", EventType: "ORDER_CREATED", OperatorID: userID, OperatorRole: domain.RoleConsumer, Remark: "seeded created order", CreatedAt: base})
	_, _ = r.AppendOrderEvent(domain.OrderEvent{OrderID: order.ID, FromStatus: "CREATED", ToStatus: "PAID", EventType: "ORDER_PAID", OperatorID: userID, OperatorRole: domain.RoleConsumer, Remark: "seeded paid order", CreatedAt: paidAt})
	_, _ = r.AppendOrderEvent(domain.OrderEvent{OrderID: order.ID, FromStatus: "PAID", ToStatus: "SHIPPED", EventType: "ORDER_SHIPPED", OperatorID: 2, OperatorRole: domain.RoleMerchant, Remark: "seeded shipped order", CreatedAt: shippedAt})
	_, _ = r.AppendBehaviorEvent(domain.BehaviorEvent{UserID: userID, EventType: domain.BehaviorOrderCreate, ProductID: product.ID, SKUID: sku.ID, OrderID: order.ID, MerchantID: merchantID, CreatedAt: base})
	_, _ = r.AppendBehaviorEvent(domain.BehaviorEvent{UserID: userID, EventType: domain.BehaviorOrderPay, ProductID: product.ID, SKUID: sku.ID, OrderID: order.ID, MerchantID: merchantID, CreatedAt: paidAt})
}

func (r *Repository) seedOrderRefunding(base time.Time, userID, merchantID int64, product domain.Product, sku domain.SKU, quantity int) {
	paidAt := base.Add(20 * time.Minute)
	order, _ := r.SaveOrder(domain.Order{
		OrderNo:            fmt.Sprintf("RCSEEDR%06d", sku.ID),
		UserID:             userID,
		MerchantID:         merchantID,
		Status:             "REFUNDING",
		TotalAmountCent:    int64(quantity) * sku.PriceCent,
		PayAmountCent:      int64(quantity) * sku.PriceCent,
		DiscountAmountCent: 0,
		IdempotencyKey:     fmt.Sprintf("seed-refunding-%d", sku.ID),
		ReceiverName:       "Alice",
		ReceiverPhone:      "13800000001",
		ReceiverAddress:    "Suzhou Industrial Park",
		PaidAt:             &paidAt,
		CreatedAt:          base,
		UpdatedAt:          paidAt.Add(2 * time.Hour),
		Items: []domain.OrderItem{
			{
				ProductID:            product.ID,
				SKUID:                sku.ID,
				ProductTitleSnapshot: product.Title,
				SKUNameSnapshot:      sku.SKUName,
				PriceCentSnapshot:    sku.PriceCent,
				Quantity:             quantity,
				TotalAmountCent:      int64(quantity) * sku.PriceCent,
				CreatedAt:            base,
				UpdatedAt:            paidAt,
			},
		},
	})
	sku.Stock -= quantity
	_, _ = r.SaveSKU(sku)
	_, _ = r.SaveInventoryLock(domain.InventoryLock{
		OrderID:     order.ID,
		SKUID:       sku.ID,
		Quantity:    quantity,
		Status:      domain.InventoryLockStatusConfirmed,
		LockedAt:    base,
		ConfirmedAt: &paidAt,
		CreatedAt:   base,
		UpdatedAt:   paidAt,
	})
	_, _ = r.AppendOrderEvent(domain.OrderEvent{OrderID: order.ID, ToStatus: "CREATED", EventType: "ORDER_CREATED", OperatorID: userID, OperatorRole: domain.RoleConsumer, Remark: "seeded created order", CreatedAt: base})
	_, _ = r.AppendOrderEvent(domain.OrderEvent{OrderID: order.ID, FromStatus: "CREATED", ToStatus: "PAID", EventType: "ORDER_PAID", OperatorID: userID, OperatorRole: domain.RoleConsumer, Remark: "seeded paid order", CreatedAt: paidAt})
	_, _ = r.AppendOrderEvent(domain.OrderEvent{OrderID: order.ID, FromStatus: "PAID", ToStatus: "REFUNDING", EventType: "ORDER_REFUND_REQUESTED", OperatorID: userID, OperatorRole: domain.RoleConsumer, Remark: "seeded refunding order", CreatedAt: paidAt.Add(2 * time.Hour)})
	_, _ = r.AppendBehaviorEvent(domain.BehaviorEvent{UserID: userID, EventType: domain.BehaviorOrderCreate, ProductID: product.ID, SKUID: sku.ID, OrderID: order.ID, MerchantID: merchantID, CreatedAt: base})
	_, _ = r.AppendBehaviorEvent(domain.BehaviorEvent{UserID: userID, EventType: domain.BehaviorOrderPay, ProductID: product.ID, SKUID: sku.ID, OrderID: order.ID, MerchantID: merchantID, CreatedAt: paidAt})
	_, _ = r.AppendBehaviorEvent(domain.BehaviorEvent{UserID: userID, EventType: domain.BehaviorOrderRefund, ProductID: product.ID, SKUID: sku.ID, OrderID: order.ID, MerchantID: merchantID, CreatedAt: paidAt.Add(2 * time.Hour)})
}

func (r *Repository) CreateUser(user domain.User) (domain.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.usersByPhone[user.Phone]; exists {
		return domain.User{}, fmt.Errorf("user phone already exists")
	}
	user.ID = r.nextID(&r.nextUserID)
	if user.CreatedAt.IsZero() {
		user.CreatedAt = time.Now().UTC()
	}
	if user.UpdatedAt.IsZero() {
		user.UpdatedAt = user.CreatedAt
	}
	r.users[user.ID] = cloneUser(user)
	r.usersByPhone[user.Phone] = user.ID
	return cloneUser(user), nil
}

func (r *Repository) FindUserByPhone(phone string) (domain.User, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.usersByPhone[phone]
	if !ok {
		return domain.User{}, false
	}
	user, ok := r.users[id]
	return cloneUser(user), ok
}

func (r *Repository) GetUser(id int64) (domain.User, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	user, ok := r.users[id]
	return cloneUser(user), ok
}

func (r *Repository) SaveSession(token string, userID int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions[token] = userID
}

func (r *Repository) GetUserByToken(token string) (domain.User, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	userID, ok := r.sessions[token]
	if !ok {
		return domain.User{}, false
	}
	user, ok := r.users[userID]
	return cloneUser(user), ok
}

func (r *Repository) CreateMerchant(merchant domain.Merchant) (domain.Merchant, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.merchantsByUserID[merchant.UserID]; exists {
		return domain.Merchant{}, fmt.Errorf("merchant already exists for user")
	}
	merchant.ID = r.nextID(&r.nextMerchantID)
	if merchant.CreatedAt.IsZero() {
		merchant.CreatedAt = time.Now().UTC()
	}
	if merchant.UpdatedAt.IsZero() {
		merchant.UpdatedAt = merchant.CreatedAt
	}
	r.merchants[merchant.ID] = cloneMerchant(merchant)
	r.merchantsByUserID[merchant.UserID] = merchant.ID
	return cloneMerchant(merchant), nil
}

func (r *Repository) GetMerchant(id int64) (domain.Merchant, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	merchant, ok := r.merchants[id]
	return cloneMerchant(merchant), ok
}

func (r *Repository) GetMerchantByUserID(userID int64) (domain.Merchant, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.merchantsByUserID[userID]
	if !ok {
		return domain.Merchant{}, false
	}
	merchant, ok := r.merchants[id]
	return cloneMerchant(merchant), ok
}

func (r *Repository) ListNotes() []domain.Note {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]domain.Note, 0, len(r.notes))
	for _, note := range r.notes {
		out = append(out, cloneNote(note))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (r *Repository) GetNote(id int64) (domain.Note, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	note, ok := r.notes[id]
	return cloneNote(note), ok
}

func (r *Repository) UpdateNote(note domain.Note) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.notes[note.ID]; !ok {
		return fmt.Errorf("note not found")
	}
	r.notes[note.ID] = cloneNote(note)
	return nil
}

func (r *Repository) ListProducts() []domain.Product {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]domain.Product, 0, len(r.products))
	for _, product := range r.products {
		out = append(out, cloneProduct(product))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (r *Repository) GetProduct(id int64) (domain.Product, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	product, ok := r.products[id]
	return cloneProduct(product), ok
}

func (r *Repository) SaveProduct(product domain.Product) (domain.Product, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if product.ID == 0 {
		product.ID = r.nextID(&r.nextProductID)
		if product.CreatedAt.IsZero() {
			product.CreatedAt = time.Now().UTC()
		}
	}
	product.UpdatedAt = time.Now().UTC()
	r.products[product.ID] = cloneProduct(product)
	return cloneProduct(product), nil
}

func (r *Repository) ListSKUsByProduct(productID int64) []domain.SKU {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]domain.SKU, 0)
	for _, sku := range r.skus {
		if sku.ProductID == productID {
			out = append(out, cloneSKU(sku))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (r *Repository) GetSKU(id int64) (domain.SKU, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.getSKULocked(id)
}

func (r *Repository) getSKULocked(id int64) (domain.SKU, bool) {
	sku, ok := r.skus[id]
	return cloneSKU(sku), ok
}

func (r *Repository) SaveSKU(sku domain.SKU) (domain.SKU, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.saveSKULocked(sku)
}

func (r *Repository) saveSKULocked(sku domain.SKU) (domain.SKU, error) {
	if sku.ID == 0 {
		sku.ID = r.nextID(&r.nextSKUID)
		if sku.CreatedAt.IsZero() {
			sku.CreatedAt = time.Now().UTC()
		}
	}
	sku.UpdatedAt = time.Now().UTC()
	r.skus[sku.ID] = cloneSKU(sku)
	return cloneSKU(sku), nil
}

func (r *Repository) ListCartItems(userID int64) []domain.CartItem {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]domain.CartItem, 0)
	for _, item := range r.cartItemsByUser[userID] {
		out = append(out, cloneCartItem(item))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (r *Repository) GetCartItem(userID, itemID int64) (domain.CartItem, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items, ok := r.cartItemsByUser[userID]
	if !ok {
		return domain.CartItem{}, false
	}
	item, ok := items[itemID]
	return cloneCartItem(item), ok
}

func (r *Repository) SaveCartItem(item domain.CartItem) (domain.CartItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if item.ID == 0 {
		item.ID = r.nextID(&r.nextCartItemID)
		if item.CreatedAt.IsZero() {
			item.CreatedAt = time.Now().UTC()
		}
	}
	item.UpdatedAt = time.Now().UTC()
	if _, ok := r.cartItemsByUser[item.UserID]; !ok {
		r.cartItemsByUser[item.UserID] = make(map[int64]domain.CartItem)
	}
	r.cartItemsByUser[item.UserID][item.ID] = cloneCartItem(item)
	return cloneCartItem(item), nil
}

func (r *Repository) DeleteCartItem(userID, itemID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	items, ok := r.cartItemsByUser[userID]
	if !ok {
		return fmt.Errorf("cart not found")
	}
	if _, ok := items[itemID]; !ok {
		return fmt.Errorf("cart item not found")
	}
	delete(items, itemID)
	return nil
}

func (r *Repository) DeleteSelectedCartItems(userID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := r.cartItemsByUser[userID]
	for id, item := range items {
		if item.Selected {
			delete(items, id)
		}
	}
	return nil
}

func (r *Repository) FindOrderByUserAndIdempotency(userID int64, idempotencyKey string) (domain.Order, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.orderByIdempotency[fmt.Sprintf("%d:%s", userID, idempotencyKey)]
	if !ok {
		return domain.Order{}, false
	}
	order, ok := r.orders[id]
	return cloneOrder(order), ok
}

func (r *Repository) ListOrdersByUser(userID int64) []domain.Order {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]domain.Order, 0)
	for _, order := range r.orders {
		if order.UserID == userID {
			out = append(out, cloneOrder(order))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (r *Repository) ListOrdersByMerchant(merchantID int64) []domain.Order {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]domain.Order, 0)
	for _, order := range r.orders {
		if order.MerchantID == merchantID {
			out = append(out, cloneOrder(order))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (r *Repository) GetOrder(id int64) (domain.Order, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	order, ok := r.orders[id]
	return cloneOrder(order), ok
}

func (r *Repository) SaveOrder(order domain.Order) (domain.Order, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.saveOrderLocked(order), nil
}

func (r *Repository) saveOrderLocked(order domain.Order) domain.Order {
	if order.ID == 0 {
		order.ID = r.nextID(&r.nextOrderID)
		if order.CreatedAt.IsZero() {
			order.CreatedAt = time.Now().UTC()
		}
		for i := range order.Items {
			order.Items[i].ID = r.nextID(&r.nextOrderItemID)
			order.Items[i].OrderID = order.ID
			if order.Items[i].CreatedAt.IsZero() {
				order.Items[i].CreatedAt = order.CreatedAt
			}
			order.Items[i].UpdatedAt = time.Now().UTC()
		}
	}
	order.UpdatedAt = time.Now().UTC()
	r.orders[order.ID] = cloneOrder(order)
	if order.IdempotencyKey != "" {
		r.orderByIdempotency[fmt.Sprintf("%d:%s", order.UserID, order.IdempotencyKey)] = order.ID
	}
	return cloneOrder(order)
}

type memOrderTx struct {
	r *Repository
}

func (t *memOrderTx) GetSKU(id int64) (domain.SKU, bool) {
	return t.r.getSKULocked(id)
}

func (t *memOrderTx) SaveSKU(sku domain.SKU) (domain.SKU, error) {
	return t.r.saveSKULocked(sku)
}

func (t *memOrderTx) ListInventoryLocksByOrder(orderID int64) []domain.InventoryLock {
	return t.r.listInventoryLocksByOrderLocked(orderID)
}

func (t *memOrderTx) UpdateInventoryLock(lock domain.InventoryLock) error {
	return t.r.updateInventoryLockLocked(lock)
}

func (t *memOrderTx) AppendOrderEvent(event domain.OrderEvent) (domain.OrderEvent, error) {
	return t.r.appendOrderEventLocked(event)
}

func (r *Repository) UpdateOrderStatus(orderID int64, fromStatus, toStatus string, mutator func(*domain.Order) error, sideEffect func(application.OrderTx, domain.Order) error) (domain.Order, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	order, ok := r.orders[orderID]
	if !ok {
		return domain.Order{}, fmt.Errorf("order %d not found", orderID)
	}
	if order.Status != orderdomain.OrderStatus(fromStatus) {
		return domain.Order{}, fmt.Errorf("order %d status is %s, expected %s", orderID, order.Status, fromStatus)
	}
	if mutator != nil {
		if err := mutator(&order); err != nil {
			return domain.Order{}, err
		}
	}
	order.Status = orderdomain.OrderStatus(toStatus)
	order.UpdatedAt = time.Now().UTC()
	r.orders[orderID] = cloneOrder(order)
	if sideEffect != nil {
		if err := sideEffect(&memOrderTx{r: r}, order); err != nil {
			// Rollback the status change on side effect failure to keep the
			// in-memory contract consistent with the PostgreSQL transaction.
			order.Status = orderdomain.OrderStatus(fromStatus)
			r.orders[orderID] = cloneOrder(order)
			return domain.Order{}, err
		}
	}
	return cloneOrder(order), nil
}

func (r *Repository) SaveOrderWithInventoryLocks(order domain.Order, locks []domain.InventoryLock) (domain.Order, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, lock := range locks {
		sku, ok := r.skus[lock.SKUID]
		if !ok {
			return domain.Order{}, fmt.Errorf("sku not found")
		}
		if sku.Stock-sku.LockedStock < lock.Quantity {
			return domain.Order{}, application.ErrInsufficientStock
		}
	}
	saved := r.saveOrderLocked(order)
	for i := range locks {
		lock := &locks[i]
		sku := r.skus[lock.SKUID]
		sku.LockedStock += lock.Quantity
		sku.UpdatedAt = time.Now().UTC()
		r.skus[sku.ID] = cloneSKU(sku)

		lock.ID = r.nextID(&r.nextInventoryLockID)
		lock.OrderID = saved.ID
		if lock.CreatedAt.IsZero() {
			lock.CreatedAt = time.Now().UTC()
		}
		lock.UpdatedAt = time.Now().UTC()
		r.locksByOrder[saved.ID] = append(r.locksByOrder[saved.ID], cloneInventoryLock(*lock))
	}
	return cloneOrder(saved), nil
}

func (r *Repository) ListOrderEvents(orderID int64) []domain.OrderEvent {
	r.mu.RLock()
	defer r.mu.RUnlock()
	events := r.orderEventsByOrder[orderID]
	out := make([]domain.OrderEvent, len(events))
	for i, event := range events {
		out[i] = cloneOrderEvent(event)
	}
	return out
}

func (r *Repository) AppendOrderEvent(event domain.OrderEvent) (domain.OrderEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.appendOrderEventLocked(event)
}

func (r *Repository) appendOrderEventLocked(event domain.OrderEvent) (domain.OrderEvent, error) {
	event.ID = r.nextID(&r.nextOrderEventID)
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	r.orderEventsByOrder[event.OrderID] = append(r.orderEventsByOrder[event.OrderID], cloneOrderEvent(event))
	return cloneOrderEvent(event), nil
}

func (r *Repository) ListInventoryLocksByOrder(orderID int64) []domain.InventoryLock {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.listInventoryLocksByOrderLocked(orderID)
}

func (r *Repository) listInventoryLocksByOrderLocked(orderID int64) []domain.InventoryLock {
	locks := r.locksByOrder[orderID]
	out := make([]domain.InventoryLock, len(locks))
	for i, lock := range locks {
		out[i] = cloneInventoryLock(lock)
	}
	return out
}

func (r *Repository) SaveInventoryLock(lock domain.InventoryLock) (domain.InventoryLock, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.saveInventoryLockLocked(lock)
}

func (r *Repository) saveInventoryLockLocked(lock domain.InventoryLock) (domain.InventoryLock, error) {
	lock.ID = r.nextID(&r.nextInventoryLockID)
	if lock.CreatedAt.IsZero() {
		lock.CreatedAt = time.Now().UTC()
	}
	lock.UpdatedAt = time.Now().UTC()
	r.locksByOrder[lock.OrderID] = append(r.locksByOrder[lock.OrderID], cloneInventoryLock(lock))
	return cloneInventoryLock(lock), nil
}

func (r *Repository) UpdateInventoryLock(lock domain.InventoryLock) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.updateInventoryLockLocked(lock)
}

func (r *Repository) updateInventoryLockLocked(lock domain.InventoryLock) error {
	locks := r.locksByOrder[lock.OrderID]
	for i := range locks {
		if locks[i].ID == lock.ID {
			lock.UpdatedAt = time.Now().UTC()
			locks[i] = cloneInventoryLock(lock)
			r.locksByOrder[lock.OrderID] = locks
			return nil
		}
	}
	return fmt.Errorf("inventory lock not found")
}

func (r *Repository) AppendBehaviorEvent(event domain.BehaviorEvent) (domain.BehaviorEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	event.ID = r.nextID(&r.nextBehaviorEventID)
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	r.behaviorEvents = append(r.behaviorEvents, event)
	return event, nil
}

func (r *Repository) ListBehaviorEvents() []domain.BehaviorEvent {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]domain.BehaviorEvent, len(r.behaviorEvents))
	copy(out, r.behaviorEvents)
	return out
}

func (r *Repository) CreateAITask(task domain.AIGenerationTask) (domain.AIGenerationTask, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	task.ID = r.nextID(&r.nextAITaskID)
	if task.CreatedAt.IsZero() {
		task.CreatedAt = time.Now().UTC()
	}
	task.UpdatedAt = task.CreatedAt
	r.aiTasks[task.ID] = cloneAITask(task)
	return cloneAITask(task), nil
}

func (r *Repository) UpdateAITask(task domain.AIGenerationTask) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.aiTasks[task.ID]; !ok {
		return fmt.Errorf("ai task not found")
	}
	task.UpdatedAt = time.Now().UTC()
	r.aiTasks[task.ID] = cloneAITask(task)
	return nil
}

func (r *Repository) GetAITask(id int64) (domain.AIGenerationTask, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	task, ok := r.aiTasks[id]
	return cloneAITask(task), ok
}

func cloneUser(user domain.User) domain.User {
	return user
}

func cloneMerchant(merchant domain.Merchant) domain.Merchant {
	return merchant
}

func cloneNote(note domain.Note) domain.Note {
	note.ProductIDs = domain.CloneInt64Slice(note.ProductIDs)
	return note
}

func cloneProduct(product domain.Product) domain.Product {
	product.SellingPoints = domain.CloneStringSlice(product.SellingPoints)
	return product
}

func cloneSKU(sku domain.SKU) domain.SKU {
	sku.SKUAttrs = domain.CloneMap(sku.SKUAttrs)
	return sku
}

func cloneCartItem(item domain.CartItem) domain.CartItem {
	return item
}

func cloneOrder(order domain.Order) domain.Order {
	if len(order.Items) == 0 {
		return order
	}
	items := make([]domain.OrderItem, len(order.Items))
	copy(items, order.Items)
	order.Items = items
	return order
}

func cloneOrderEvent(event domain.OrderEvent) domain.OrderEvent {
	return event
}

func cloneInventoryLock(lock domain.InventoryLock) domain.InventoryLock {
	return lock
}

func cloneAITask(task domain.AIGenerationTask) domain.AIGenerationTask {
	if task.Input != nil {
		input := make(map[string]any, len(task.Input))
		for key, value := range task.Input {
			input[key] = value
		}
		task.Input = input
	}
	if task.Output != nil {
		output := make(map[string]any, len(task.Output))
		for key, value := range task.Output {
			output[key] = value
		}
		task.Output = output
	}
	return task
}
