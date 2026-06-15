package memory

import (
	"github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
	"sync"
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
