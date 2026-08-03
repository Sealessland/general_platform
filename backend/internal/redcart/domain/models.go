package domain

import (
	"time"

	orderdomain "github.com/example/redcart-copilot/backend/internal/order/domain"
)

// 用户角色，用于鉴权中间件判断访问权限。
const (
	RoleConsumer = "consumer"
	RoleMerchant = "merchant"
)

// 商品生命周期状态。
const (
	ProductStatusDraft   = "draft"
	ProductStatusOnline  = "online"
	ProductStatusOffline = "offline"
)

// SKU 上下架状态。
const (
	SKUStatusActive   = "active"
	SKUStatusInactive = "inactive"
)

// 库存锁生命周期：下单锁定，支付确认，取消/退款释放。
const (
	InventoryLockStatusLocked    = "locked"
	InventoryLockStatusConfirmed = "confirmed"
	InventoryLockStatusReleased  = "released"
)

// AI 生成任务执行状态。
const (
	AITaskStatusPending   = "pending"
	AITaskStatusCompleted = "completed"
	AITaskStatusFailed    = "failed"
)

// AI 任务类型，区分卖点提炼与经营复盘。
const (
	TaskTypeSellingPoints  = "product_selling_points"
	TaskTypeBusinessReview = "business_review"
)

// 用户行为埋点事件类型，供商家经营看板统计使用。
const (
	BehaviorNoteView     = "NOTE_VIEW"
	BehaviorProductClick = "PRODUCT_CLICK"
	BehaviorAddToCart    = "ADD_TO_CART"
	BehaviorOrderCreate  = "ORDER_CREATE"
	BehaviorOrderPay     = "ORDER_PAY"
	BehaviorOrderCancel  = "ORDER_CANCEL"
	BehaviorOrderRefund  = "ORDER_REFUND"
)

// User 用户账号，包含登录凭据与角色。
type User struct {
	ID           int64
	Nickname     string
	Phone        string
	PasswordHash string
	Role         string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Merchant 商家资料，与用户账号一对一关联。
type Merchant struct {
	ID          int64
	UserID      int64
	Name        string
	Description string
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Note 种草笔记，可关联多个商品。
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

// Product 商品，归属于商家，可包含多个 SKU。
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

// SKU 商品规格（具体可售单元），价格以分为单位记录。
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

// CartItem 购物车条目。
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

// Order 订单。Status 复用 order 模块的状态机类型，金额均以分存储。
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

// OrderItem 订单条目，快照商品/SKU 名称与价格，避免后续改价影响历史订单。
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

// OrderEvent 订单状态流转事件记录。
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

// InventoryLock 库存锁，用于下单到支付期间的库存预占，以及取消/退款时的补偿释放。
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

// BehaviorEvent 用户行为事件，用于经营分析。
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

// AIGenerationTask AI 生成任务，Input/Output 为各任务类型通用的结构化字段。
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

// CloneStringSlice 深拷贝字符串切片，空输入返回 nil，避免调用方意外共享底层数组。
func CloneStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}

// CloneInt64Slice 深拷贝 int64 切片，空输入返回 nil。
func CloneInt64Slice(values []int64) []int64 {
	if len(values) == 0 {
		return nil
	}
	out := make([]int64, len(values))
	copy(out, values)
	return out
}

// CloneMap 深拷贝字符串键值对 map，空输入返回 nil。
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
