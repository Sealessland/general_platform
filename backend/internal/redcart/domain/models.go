package domain

import (
	"time"

	orderdomain "github.com/example/redcart-copilot/backend/internal/order/domain"
)

const (
	RoleConsumer = "consumer"
	RoleMerchant = "merchant"
)

const (
	ProductStatusDraft   = "draft"
	ProductStatusOnline  = "online"
	ProductStatusOffline = "offline"
)

const (
	SKUStatusActive   = "active"
	SKUStatusInactive = "inactive"
)

const (
	InventoryLockStatusLocked    = "locked"
	InventoryLockStatusConfirmed = "confirmed"
	InventoryLockStatusReleased  = "released"
)

const (
	AITaskStatusPending   = "pending"
	AITaskStatusCompleted = "completed"
	AITaskStatusFailed    = "failed"
)

const (
	TaskTypeSellingPoints  = "product_selling_points"
	TaskTypeBusinessReview = "business_review"
)

const (
	BehaviorNoteView     = "NOTE_VIEW"
	BehaviorProductClick = "PRODUCT_CLICK"
	BehaviorAddToCart    = "ADD_TO_CART"
	BehaviorOrderCreate  = "ORDER_CREATE"
	BehaviorOrderPay     = "ORDER_PAY"
	BehaviorOrderCancel  = "ORDER_CANCEL"
	BehaviorOrderRefund  = "ORDER_REFUND"
)

type User struct {
	ID           int64
	Nickname     string
	Phone        string
	PasswordHash string
	Role         string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Merchant struct {
	ID          int64
	UserID      int64
	Name        string
	Description string
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Note struct {
	ID         int64
	AuthorID   int64
	Title      string
	Content    string
	CoverURL   string
	Status     string
	ViewCount  int64
	LikeCount  int64
	ProductIDs []int64
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type Product struct {
	ID            int64
	MerchantID    int64
	Title         string
	Description   string
	CoverURL      string
	CategoryID    int64
	Status        string
	SellingPoints []string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type SKU struct {
	ID          int64
	ProductID   int64
	SKUName     string
	SKUAttrs    map[string]string
	PriceCent   int64
	Stock       int
	LockedStock int
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type CartItem struct {
	ID        int64
	UserID    int64
	ProductID int64
	SKUID     int64
	Quantity  int
	Selected  bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Order struct {
	ID                 int64
	OrderNo            string
	UserID             int64
	MerchantID         int64
	Status             orderdomain.OrderStatus
	TotalAmountCent    int64
	PayAmountCent      int64
	DiscountAmountCent int64
	IdempotencyKey     string
	ReceiverName       string
	ReceiverPhone      string
	ReceiverAddress    string
	PaidAt             *time.Time
	CancelledAt        *time.Time
	ShippedAt          *time.Time
	FinishedAt         *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
	Items              []OrderItem
}

type OrderItem struct {
	ID                   int64
	OrderID              int64
	ProductID            int64
	SKUID                int64
	ProductTitleSnapshot string
	SKUNameSnapshot      string
	PriceCentSnapshot    int64
	Quantity             int
	TotalAmountCent      int64
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type OrderEvent struct {
	ID           int64
	OrderID      int64
	FromStatus   string
	ToStatus     string
	EventType    string
	OperatorID   int64
	OperatorRole string
	Remark       string
	CreatedAt    time.Time
}

type InventoryLock struct {
	ID          int64
	OrderID     int64
	SKUID       int64
	Quantity    int
	Status      string
	LockedAt    time.Time
	ConfirmedAt *time.Time
	ReleasedAt  *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type BehaviorEvent struct {
	ID         int64
	UserID     int64
	EventType  string
	NoteID     int64
	ProductID  int64
	SKUID      int64
	OrderID    int64
	MerchantID int64
	CreatedAt  time.Time
}

type AIGenerationTask struct {
	ID           int64
	UserID       int64
	MerchantID   int64
	TaskType     string
	Input        map[string]any
	Output       map[string]any
	Status       string
	ErrorMessage string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func CloneStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}

func CloneInt64Slice(values []int64) []int64 {
	if len(values) == 0 {
		return nil
	}
	out := make([]int64, len(values))
	copy(out, values)
	return out
}

func CloneMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}
