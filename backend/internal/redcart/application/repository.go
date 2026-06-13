package application

import (
	"errors"

	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
)

var ErrInsufficientStock = errors.New("stock is insufficient")

// OrderTx exposes repository operations that can be performed inside the
// transaction scoped to an order status transition. It is passed to the
// sideEffect callback of UpdateOrderStatus so that inventory changes and
// event writes are committed atomically with the status change.
type OrderTx interface {
	GetSKU(id int64) (domain.SKU, bool)
	SaveSKU(sku domain.SKU) (domain.SKU, error)
	ListInventoryLocksByOrder(orderID int64) []domain.InventoryLock
	UpdateInventoryLock(lock domain.InventoryLock) error
	AppendOrderEvent(event domain.OrderEvent) (domain.OrderEvent, error)
}

type Repository interface {
	CreateUser(user domain.User) (domain.User, error)
	FindUserByPhone(phone string) (domain.User, bool)
	GetUser(id int64) (domain.User, bool)
	SaveSession(token string, userID int64)
	GetUserByToken(token string) (domain.User, bool)
	CreateMerchant(merchant domain.Merchant) (domain.Merchant, error)
	GetMerchant(id int64) (domain.Merchant, bool)
	GetMerchantByUserID(userID int64) (domain.Merchant, bool)

	ListNotes() []domain.Note
	GetNote(id int64) (domain.Note, bool)
	UpdateNote(note domain.Note) error

	ListProducts() []domain.Product
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
	ListOrdersByUser(userID int64) []domain.Order
	ListOrdersByMerchant(merchantID int64) []domain.Order
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
