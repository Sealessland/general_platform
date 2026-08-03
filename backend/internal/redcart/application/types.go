// Package application 是 redcart 的应用用例层：校验输入、执行权限与业务规则、
// 编排仓储/事件/AI 依赖，并输出面向接口层的视图对象（DTO）。
package application

import "time"

// Actor 描述已认证请求方的身份上下文：用户 ID、角色，以及商家角色下的店铺 ID。
type Actor struct {
	UserID     int64
	Role       string
	MerchantID int64
	Nickname   string
}

type RegisterInput struct {
	Nickname string `json:"nickname"`
	Phone    string `json:"phone"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

type LoginInput struct {
	Phone    string `json:"phone"`
	Password string `json:"password"`
}

// AuthSession 是登录/刷新后返回的会话：不透明访问令牌 + 刷新令牌 + 用户视图。
type AuthSession struct {
	Token        string   `json:"token"`
	RefreshToken string   `json:"refresh_token,omitempty"`
	User         UserView `json:"user"`
}

type UserView struct {
	ID         int64         `json:"id"`
	Nickname   string        `json:"nickname"`
	Phone      string        `json:"phone"`
	Role       string        `json:"role"`
	MerchantID int64         `json:"merchant_id,omitempty"`
	Merchant   *MerchantView `json:"merchant,omitempty"`
}

type MerchantView struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
}

// NoteSummary 笔记列表视图：标题/内容摘要与关联商品卡片。
type NoteSummary struct {
	ID             int64         `json:"id"`
	Title          string        `json:"title"`
	Content        string        `json:"content"`
	CoverURL       string        `json:"cover_url"`
	ViewCount      int64         `json:"view_count"`
	LikeCount      int64         `json:"like_count"`
	LinkedProducts []ProductCard `json:"linked_products"`
}

// NoteDetail 笔记详情视图，字段与 NoteSummary 完全一致（当前仅语义区分）。
type NoteDetail struct {
	ID             int64         `json:"id"`
	Title          string        `json:"title"`
	Content        string        `json:"content"`
	CoverURL       string        `json:"cover_url"`
	ViewCount      int64         `json:"view_count"`
	LikeCount      int64         `json:"like_count"`
	LinkedProducts []ProductCard `json:"linked_products"`
}

type ProductCard struct {
	ID            int64    `json:"id"`
	Title         string   `json:"title"`
	CoverURL      string   `json:"cover_url"`
	Status        string   `json:"status"`
	MinPriceCent  int64    `json:"min_price_cent"`
	Stock         int      `json:"stock"`
	SellingPoints []string `json:"selling_points"`
}

type SKUView struct {
	ID          int64             `json:"id"`
	ProductID   int64             `json:"product_id"`
	SKUName     string            `json:"sku_name"`
	SKUAttrs    map[string]string `json:"sku_attrs"`
	PriceCent   int64             `json:"price_cent"`
	Stock       int               `json:"stock"`
	LockedStock int               `json:"locked_stock"`
	Status      string            `json:"status"`
}

type ProductDetail struct {
	ID            int64     `json:"id"`
	MerchantID    int64     `json:"merchant_id"`
	Title         string    `json:"title"`
	Description   string    `json:"description"`
	CoverURL      string    `json:"cover_url"`
	CategoryID    int64     `json:"category_id"`
	Status        string    `json:"status"`
	SellingPoints []string  `json:"selling_points"`
	SKUs          []SKUView `json:"skus"`
}

type CartItemInput struct {
	SKUID    int64 `json:"sku_id"`
	Quantity int   `json:"quantity"`
}

type CartItemUpdateInput struct {
	Quantity int   `json:"quantity"`
	Selected *bool `json:"selected"`
}

type CartItemView struct {
	ID            int64    `json:"id"`
	ProductID     int64    `json:"product_id"`
	ProductTitle  string   `json:"product_title"`
	CoverURL      string   `json:"cover_url"`
	SKUID         int64    `json:"sku_id"`
	SKUName       string   `json:"sku_name"`
	PriceCent     int64    `json:"price_cent"`
	Quantity      int      `json:"quantity"`
	Selected      bool     `json:"selected"`
	Stock         int      `json:"stock"`
	Status        string   `json:"status"`
	SellingPoints []string `json:"selling_points"`
}

// CartView 购物车视图，另含勾选商品的件数、数量与金额汇总（金额单位：分）。
type CartView struct {
	Items              []CartItemView `json:"items"`
	SelectedItemCount  int            `json:"selected_item_count"`
	SelectedQuantity   int            `json:"selected_quantity"`
	SelectedAmountCent int64          `json:"selected_amount_cent"`
}

type OrderLineInput struct {
	SKUID    int64 `json:"sku_id"`
	Quantity int   `json:"quantity"`
}

type CheckoutInput struct {
	Items           []OrderLineInput `json:"items"`
	ReceiverName    string           `json:"receiver_name"`
	ReceiverPhone   string           `json:"receiver_phone"`
	ReceiverAddress string           `json:"receiver_address"`
}

type OrderItemView struct {
	ID              int64  `json:"id"`
	ProductID       int64  `json:"product_id"`
	SKUID           int64  `json:"sku_id"`
	ProductTitle    string `json:"product_title"`
	SKUName         string `json:"sku_name"`
	PriceCent       int64  `json:"price_cent"`
	Quantity        int    `json:"quantity"`
	TotalAmountCent int64  `json:"total_amount_cent"`
}

type InventoryLockView struct {
	ID          int64      `json:"id"`
	SKUID       int64      `json:"sku_id"`
	Quantity    int        `json:"quantity"`
	Status      string     `json:"status"`
	LockedAt    time.Time  `json:"locked_at"`
	ConfirmedAt *time.Time `json:"confirmed_at,omitempty"`
	ReleasedAt  *time.Time `json:"released_at,omitempty"`
}

type OrderEventView struct {
	ID           int64     `json:"id"`
	FromStatus   string    `json:"from_status,omitempty"`
	ToStatus     string    `json:"to_status"`
	EventType    string    `json:"event_type"`
	OperatorID   int64     `json:"operator_id"`
	OperatorRole string    `json:"operator_role"`
	Remark       string    `json:"remark,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// OrderView 订单详情视图：包含商品快照、状态流转事件与库存锁定记录，由 enrichOrderView 组装。
type OrderView struct {
	ID                 int64               `json:"id"`
	OrderNo            string              `json:"order_no"`
	UserID             int64               `json:"user_id"`
	MerchantID         int64               `json:"merchant_id"`
	Status             string              `json:"status"`
	TotalAmountCent    int64               `json:"total_amount_cent"`
	PayAmountCent      int64               `json:"pay_amount_cent"`
	DiscountAmountCent int64               `json:"discount_amount_cent"`
	ReceiverName       string              `json:"receiver_name"`
	ReceiverPhone      string              `json:"receiver_phone"`
	ReceiverAddress    string              `json:"receiver_address"`
	PaidAt             *time.Time          `json:"paid_at,omitempty"`
	CancelledAt        *time.Time          `json:"cancelled_at,omitempty"`
	ShippedAt          *time.Time          `json:"shipped_at,omitempty"`
	FinishedAt         *time.Time          `json:"finished_at,omitempty"`
	CreatedAt          time.Time           `json:"created_at"`
	UpdatedAt          time.Time           `json:"updated_at"`
	Items              []OrderItemView     `json:"items"`
	Events             []OrderEventView    `json:"events"`
	InventoryLocks     []InventoryLockView `json:"inventory_locks"`
}

// OrderPreview 结算预演结果：金额明细与库存是否充足（StockOK），供下单前确认。
type OrderPreview struct {
	MerchantID         int64           `json:"merchant_id"`
	Items              []OrderItemView `json:"items"`
	TotalAmountCent    int64           `json:"total_amount_cent"`
	PayAmountCent      int64           `json:"pay_amount_cent"`
	DiscountAmountCent int64           `json:"discount_amount_cent"`
	StockOK            bool            `json:"stock_ok"`
}

type RefundRequestInput struct {
	Reason string `json:"reason"`
}

type MerchantProductInput struct {
	Title         string   `json:"title"`
	Description   string   `json:"description"`
	CoverURL      string   `json:"cover_url"`
	CategoryID    int64    `json:"category_id"`
	SellingPoints []string `json:"selling_points"`
}

type MerchantSKUInput struct {
	SKUName   string            `json:"sku_name"`
	SKUAttrs  map[string]string `json:"sku_attrs"`
	PriceCent int64             `json:"price_cent"`
	Stock     int               `json:"stock"`
	Status    string            `json:"status"`
}

type MerchantOrderShipInput struct {
	LogisticsNo string `json:"logistics_no"`
	Remark      string `json:"remark"`
}

// DashboardFunnel 转化漏斗：各阶段行为事件计数（浏览→点击→加购→下单→支付→退款）。
type DashboardFunnel struct {
	NoteViews     int `json:"note_views"`
	ProductClicks int `json:"product_clicks"`
	AddToCart     int `json:"add_to_cart"`
	OrderCreate   int `json:"order_create"`
	OrderPay      int `json:"order_pay"`
	OrderRefund   int `json:"order_refund"`
}

type DashboardProductStat struct {
	ProductID      int64  `json:"product_id"`
	Title          string `json:"title"`
	Status         string `json:"status"`
	Exposure       int    `json:"exposure"`
	Clicks         int    `json:"clicks"`
	AddToCart      int    `json:"add_to_cart"`
	Orders         int    `json:"orders"`
	Paid           int    `json:"paid"`
	Refunds        int    `json:"refunds"`
	AvailableStock int    `json:"available_stock"`
}

// DashboardSummary 经营汇总：商品/订单数量、GMV（分）、退款数与库存预警 SKU 数。
type DashboardSummary struct {
	ProductCount        int   `json:"product_count"`
	OnlineProductCount  int   `json:"online_product_count"`
	OrderCount          int   `json:"order_count"`
	PaidOrderCount      int   `json:"paid_order_count"`
	GMVAmountCent       int64 `json:"gmv_amount_cent"`
	RefundOrderCount    int   `json:"refund_order_count"`
	InventoryWarningSKU int   `json:"inventory_warning_sku"`
}

// SellingPointInput 卖点文案生成任务入参：商品名称、属性、目标人群、价格与用户评价。
type SellingPointInput struct {
	ProductName string   `json:"product_name"`
	Attributes  []string `json:"attributes"`
	TargetUsers string   `json:"target_users"`
	PriceCent   int64    `json:"price_cent"`
	Reviews     []string `json:"reviews"`
}

// BusinessReviewInput 经营诊断任务入参：统计窗口天数与可选的商品维度。
type BusinessReviewInput struct {
	WindowDays int   `json:"window_days"`
	ProductID  int64 `json:"product_id"`
}

// A2UISurfaceInput A2UI 界面生成入参：surface 标识、用户意图与可选上下文 JSON。
type A2UISurfaceInput struct {
	SurfaceID   string `json:"surface_id"`
	UserIntent  string `json:"user_intent"`
	ContextJSON string `json:"context_json,omitempty"`
}

// A2UISurfaceView 生成的 A2UI 界面描述（AI 客户端可直接渲染的交互原语）。
type A2UISurfaceView struct {
	SurfaceID string `json:"surface_id"`
	A2UIJSON  string `json:"a2ui_json"`
}

// AITaskView AI 任务轮询视图：状态、入参、输出与失败原因。
type AITaskView struct {
	ID           int64          `json:"id"`
	TaskType     string         `json:"task_type"`
	Status       string         `json:"status"`
	Input        map[string]any `json:"input"`
	Output       map[string]any `json:"output,omitempty"`
	ErrorMessage string         `json:"error_message,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}
