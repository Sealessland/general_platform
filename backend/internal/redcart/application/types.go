package application

import "time"

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

type AuthSession struct {
	Token string   `json:"token"`
	User  UserView `json:"user"`
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

type NoteSummary struct {
	ID             int64         `json:"id"`
	Title          string        `json:"title"`
	Content        string        `json:"content"`
	CoverURL       string        `json:"cover_url"`
	ViewCount      int64         `json:"view_count"`
	LikeCount      int64         `json:"like_count"`
	LinkedProducts []ProductCard `json:"linked_products"`
}

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

type DashboardSummary struct {
	ProductCount        int   `json:"product_count"`
	OnlineProductCount  int   `json:"online_product_count"`
	OrderCount          int   `json:"order_count"`
	PaidOrderCount      int   `json:"paid_order_count"`
	GMVAmountCent       int64 `json:"gmv_amount_cent"`
	RefundOrderCount    int   `json:"refund_order_count"`
	InventoryWarningSKU int   `json:"inventory_warning_sku"`
}

type SellingPointInput struct {
	ProductName string   `json:"product_name"`
	Attributes  []string `json:"attributes"`
	TargetUsers string   `json:"target_users"`
	PriceCent   int64    `json:"price_cent"`
	Reviews     []string `json:"reviews"`
}

type BusinessReviewInput struct {
	WindowDays int   `json:"window_days"`
	ProductID  int64 `json:"product_id"`
}

type A2UISurfaceInput struct {
	SurfaceID   string `json:"surface_id"`
	UserIntent  string `json:"user_intent"`
	ContextJSON string `json:"context_json,omitempty"`
}

type A2UISurfaceView struct {
	SurfaceID string `json:"surface_id"`
	A2UIJSON  string `json:"a2ui_json"`
}

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
