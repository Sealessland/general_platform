package application

import (
	"errors"

	"github.com/example/redcart-copilot/backend/internal/event"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
)

// ErrInsufficientStock 是仓储层在库存不足/并发扣减失败时返回的哨兵错误，由调用方映射为冲突响应。
var ErrInsufficientStock = errors.New("stock is insufficient")

// TokenType 区分访问令牌与刷新令牌，应用层据此拒绝把刷新令牌当作访问令牌使用（反之亦然）。
type TokenType string

const (
	TokenTypeAccess  TokenType = "access"
	TokenTypeRefresh TokenType = "refresh"
)

// OrderTx 暴露可在订单状态流转事务内执行的仓储操作；它被传入 UpdateOrderStatus 的
// sideEffect 回调，使库存变更与事件写入随状态变更原子提交。
type OrderTx interface {
	event.Outbox
	GetSKU(id int64) (domain.SKU, bool)
	SaveSKU(sku domain.SKU) (domain.SKU, error)
	ListInventoryLocksByOrder(orderID int64) []domain.InventoryLock
	UpdateInventoryLock(lock domain.InventoryLock) error
	AppendOrderEvent(event domain.OrderEvent) (domain.OrderEvent, error)
}

// Repository 定义应用层所需的全部持久化操作，由基础设施层实现
// （postgres 仓储同时实现 event.Outbox，以支持订单状态流转的事务内事件写出）。
type Repository interface {
	CreateUser(user domain.User) (domain.User, error)
	FindUserByPhone(phone string) (domain.User, bool)
	GetUser(id int64) (domain.User, bool)
	SaveSession(accessToken, refreshToken string, userID int64) error
	GetUserByToken(token string) (domain.User, TokenType, bool)
	DeleteSession(token string)
	CreateMerchant(merchant domain.Merchant) (domain.Merchant, error)
	GetMerchant(id int64) (domain.Merchant, bool)
	GetMerchantByUserID(userID int64) (domain.Merchant, bool)

	ListNotes(limit, offset int) []domain.Note
	GetNote(id int64) (domain.Note, bool)
	UpdateNote(note domain.Note) error

	ListProducts(limit, offset int) []domain.Product
	GetProduct(id int64) (domain.Product, bool)
	SaveProduct(product domain.Product) (domain.Product, error)

	ListSKUsByProduct(productID int64) []domain.SKU
	GetSKU(id int64) (domain.SKU, bool)
	SaveSKU(sku domain.SKU) (domain.SKU, error)

	ListCartItems(userID int64) []domain.CartItem
	GetCartItem(userID, itemID int64) (domain.CartItem, bool)
	SaveCartItem(item domain.CartItem) (domain.CartItem, error)
	DeleteCartItem(userID, itemID int64) error
	DeleteSelectedCartItems(userID int64) error

	FindOrderByUserAndIdempotency(userID int64, idempotencyKey string) (domain.Order, bool)
	ListOrdersByUser(userID int64, limit, offset int) []domain.Order
	ListOrdersByMerchant(merchantID int64, limit, offset int) []domain.Order
	GetOrder(id int64) (domain.Order, bool)
	SaveOrder(order domain.Order) (domain.Order, error)
	SaveOrderWithInventoryLocks(order domain.Order, locks []domain.InventoryLock) (domain.Order, error)
	UpdateOrderStatus(orderID int64, fromStatus, toStatus string, mutator func(*domain.Order) error, sideEffect func(OrderTx, domain.Order) error) (domain.Order, error)

	ListOrderEvents(orderID int64) []domain.OrderEvent
	AppendOrderEvent(event domain.OrderEvent) (domain.OrderEvent, error)

	ListInventoryLocksByOrder(orderID int64) []domain.InventoryLock
	SaveInventoryLock(lock domain.InventoryLock) (domain.InventoryLock, error)
	UpdateInventoryLock(lock domain.InventoryLock) error

	AppendBehaviorEvent(event domain.BehaviorEvent) (domain.BehaviorEvent, error)
	ListBehaviorEvents() []domain.BehaviorEvent

	CreateAITask(task domain.AIGenerationTask) (domain.AIGenerationTask, error)
	UpdateAITask(task domain.AIGenerationTask) error
	GetAITask(id int64) (domain.AIGenerationTask, bool)
}
