package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	backendai "github.com/example/redcart-copilot/backend/internal/ai"
	orderdomain "github.com/example/redcart-copilot/backend/internal/order/domain"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
)

type Service struct {
	repo       Repository
	aiProvider backendai.AIProvider
	now        func() time.Time
}

func NewService(repo Repository, aiProvider backendai.AIProvider) *Service {
	return &Service{
		repo:       repo,
		aiProvider: aiProvider,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

func (s *Service) Register(ctx context.Context, input RegisterInput) (*AuthSession, error) {
	_ = ctx
	if strings.TrimSpace(input.Nickname) == "" || strings.TrimSpace(input.Phone) == "" || input.Password == "" {
		return nil, newError(ErrorInvalidArgument, "nickname, phone, and password are required")
	}
	if input.Role != domain.RoleConsumer && input.Role != domain.RoleMerchant {
		return nil, newError(ErrorInvalidArgument, "role must be consumer or merchant")
	}
	now := s.now()
	user, err := s.repo.CreateUser(domain.User{
		Nickname:     strings.TrimSpace(input.Nickname),
		Phone:        strings.TrimSpace(input.Phone),
		PasswordHash: hashPassword(input.Password),
		Role:         input.Role,
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	if err != nil {
		return nil, newError(ErrorConflict, err.Error())
	}
	if user.Role == domain.RoleMerchant {
		_, err = s.repo.CreateMerchant(domain.Merchant{
			UserID:      user.ID,
			Name:        fmt.Sprintf("%s 的店铺", user.Nickname),
			Description: "merchant workspace created from registration",
			Status:      "active",
			CreatedAt:   now,
			UpdatedAt:   now,
		})
		if err != nil {
			return nil, err
		}
	}
	return s.issueSession(user)
}

func (s *Service) Login(ctx context.Context, input LoginInput) (*AuthSession, error) {
	_ = ctx
	user, ok := s.repo.FindUserByPhone(strings.TrimSpace(input.Phone))
	if !ok {
		return nil, newError(ErrorUnauthorized, "invalid phone or password")
	}
	if user.PasswordHash != hashPassword(input.Password) {
		return nil, newError(ErrorUnauthorized, "invalid phone or password")
	}
	return s.issueSession(user)
}

func (s *Service) Me(ctx context.Context, token string) (*UserView, error) {
	_ = ctx
	user, ok := s.repo.GetUserByToken(token)
	if !ok {
		return nil, newError(ErrorUnauthorized, "invalid token")
	}
	view := s.toUserView(user)
	return &view, nil
}

func (s *Service) Authenticate(token string) (*Actor, error) {
	user, ok := s.repo.GetUserByToken(token)
	if !ok {
		return nil, newError(ErrorUnauthorized, "missing or invalid token")
	}
	actor := &Actor{
		UserID:   user.ID,
		Role:     user.Role,
		Nickname: user.Nickname,
	}
	if merchant, ok := s.repo.GetMerchantByUserID(user.ID); ok {
		actor.MerchantID = merchant.ID
	}
	return actor, nil
}

func (s *Service) ListNotes(ctx context.Context) ([]NoteSummary, error) {
	_ = ctx
	notes := s.repo.ListNotes()
	out := make([]NoteSummary, 0, len(notes))
	for _, note := range notes {
		out = append(out, s.toNoteSummary(note))
	}
	return out, nil
}

func (s *Service) GetNote(ctx context.Context, noteID int64, actor *Actor) (*NoteDetail, error) {
	_ = ctx
	note, ok := s.repo.GetNote(noteID)
	if !ok {
		return nil, newError(ErrorNotFound, "note not found")
	}
	note.ViewCount++
	note.UpdatedAt = s.now()
	if err := s.repo.UpdateNote(note); err != nil {
		return nil, err
	}
	if actor != nil {
		productID := int64(0)
		if len(note.ProductIDs) > 0 {
			productID = note.ProductIDs[0]
		}
		_, _ = s.repo.AppendBehaviorEvent(domain.BehaviorEvent{
			UserID:     actor.UserID,
			EventType:  domain.BehaviorNoteView,
			NoteID:     note.ID,
			ProductID:  productID,
			MerchantID: s.primaryMerchantID(note.ProductIDs),
			CreatedAt:  s.now(),
		})
	}
	view := s.toNoteDetail(note)
	return &view, nil
}

func (s *Service) ListProducts(ctx context.Context) ([]ProductCard, error) {
	_ = ctx
	products := s.repo.ListProducts()
	out := make([]ProductCard, 0, len(products))
	for _, product := range products {
		if product.Status != domain.ProductStatusOnline {
			continue
		}
		out = append(out, s.toProductCard(product))
	}
	return out, nil
}

func (s *Service) GetProduct(ctx context.Context, productID int64, actor *Actor) (*ProductDetail, error) {
	_ = ctx
	product, ok := s.repo.GetProduct(productID)
	if !ok {
		return nil, newError(ErrorNotFound, "product not found")
	}
	if actor != nil {
		_, _ = s.repo.AppendBehaviorEvent(domain.BehaviorEvent{
			UserID:     actor.UserID,
			EventType:  domain.BehaviorProductClick,
			ProductID:  product.ID,
			MerchantID: product.MerchantID,
			CreatedAt:  s.now(),
		})
	}
	view := s.toProductDetail(product)
	return &view, nil
}

func (s *Service) ListProductSKUs(ctx context.Context, productID int64) ([]SKUView, error) {
	_ = ctx
	if _, ok := s.repo.GetProduct(productID); !ok {
		return nil, newError(ErrorNotFound, "product not found")
	}
	skus := s.repo.ListSKUsByProduct(productID)
	out := make([]SKUView, 0, len(skus))
	for _, sku := range skus {
		out = append(out, s.toSKUView(sku))
	}
	return out, nil
}

func (s *Service) GetCart(ctx context.Context, actor Actor) (*CartView, error) {
	_ = ctx
	items := s.repo.ListCartItems(actor.UserID)
	view := s.buildCartView(items)
	return &view, nil
}

func (s *Service) AddCartItem(ctx context.Context, actor Actor, input CartItemInput) (*CartItemView, error) {
	_ = ctx
	if input.Quantity <= 0 {
		return nil, newError(ErrorInvalidArgument, "quantity must be positive")
	}
	sku, ok := s.repo.GetSKU(input.SKUID)
	if !ok {
		return nil, newError(ErrorNotFound, "sku not found")
	}
	product, ok := s.repo.GetProduct(sku.ProductID)
	if !ok {
		return nil, newError(ErrorNotFound, "product not found")
	}
	if product.Status != domain.ProductStatusOnline {
		return nil, newError(ErrorConflict, "product is not online")
	}
	for _, item := range s.repo.ListCartItems(actor.UserID) {
		if item.SKUID == input.SKUID {
			item.Quantity += input.Quantity
			item.UpdatedAt = s.now()
			saved, err := s.repo.SaveCartItem(item)
			if err != nil {
				return nil, err
			}
			_, _ = s.repo.AppendBehaviorEvent(domain.BehaviorEvent{
				UserID:     actor.UserID,
				EventType:  domain.BehaviorAddToCart,
				ProductID:  product.ID,
				SKUID:      sku.ID,
				MerchantID: product.MerchantID,
				CreatedAt:  s.now(),
			})
			view := s.toCartItemView(saved)
			return &view, nil
		}
	}
	item := domain.CartItem{
		UserID:    actor.UserID,
		ProductID: product.ID,
		SKUID:     sku.ID,
		Quantity:  input.Quantity,
		Selected:  true,
		CreatedAt: s.now(),
		UpdatedAt: s.now(),
	}
	saved, err := s.repo.SaveCartItem(item)
	if err != nil {
		return nil, err
	}
	_, _ = s.repo.AppendBehaviorEvent(domain.BehaviorEvent{
		UserID:     actor.UserID,
		EventType:  domain.BehaviorAddToCart,
		ProductID:  product.ID,
		SKUID:      sku.ID,
		MerchantID: product.MerchantID,
		CreatedAt:  s.now(),
	})
	view := s.toCartItemView(saved)
	return &view, nil
}

func (s *Service) UpdateCartItem(ctx context.Context, actor Actor, itemID int64, input CartItemUpdateInput) (*CartItemView, error) {
	_ = ctx
	item, ok := s.repo.GetCartItem(actor.UserID, itemID)
	if !ok {
		return nil, newError(ErrorNotFound, "cart item not found")
	}
	if input.Quantity > 0 {
		item.Quantity = input.Quantity
	}
	if input.Selected != nil {
		item.Selected = *input.Selected
	}
	item.UpdatedAt = s.now()
	saved, err := s.repo.SaveCartItem(item)
	if err != nil {
		return nil, err
	}
	view := s.toCartItemView(saved)
	return &view, nil
}

func (s *Service) DeleteCartItem(ctx context.Context, actor Actor, itemID int64) error {
	_ = ctx
	if err := s.repo.DeleteCartItem(actor.UserID, itemID); err != nil {
		return newError(ErrorNotFound, "cart item not found")
	}
	return nil
}

func (s *Service) PreviewOrder(ctx context.Context, actor Actor, input CheckoutInput) (*OrderPreview, error) {
	_ = ctx
	lines, err := s.normalizeCheckoutLines(actor, input.Items)
	if err != nil {
		return nil, err
	}
	preview, err := s.buildOrderPreview(lines)
	if err != nil {
		return nil, err
	}
	return preview, nil
}

func (s *Service) CreateOrder(ctx context.Context, actor Actor, idempotencyKey string, input CheckoutInput) (*OrderView, error) {
	_ = ctx
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" {
		return nil, newError(ErrorInvalidArgument, "idempotency key is required")
	}
	if existing, ok := s.repo.FindOrderByUserAndIdempotency(actor.UserID, idempotencyKey); ok {
		view, err := s.enrichOrderView(existing)
		if err != nil {
			return nil, err
		}
		return &view, nil
	}
	lines, err := s.normalizeCheckoutLines(actor, input.Items)
	if err != nil {
		return nil, err
	}
	preview, err := s.buildOrderPreview(lines)
	if err != nil {
		return nil, err
	}
	if !preview.StockOK {
		return nil, newError(ErrorConflict, "stock is insufficient")
	}
	now := s.now()
	order := domain.Order{
		OrderNo:            fmt.Sprintf("RC%014d", now.UnixNano()%1e14),
		UserID:             actor.UserID,
		MerchantID:         preview.MerchantID,
		Status:             orderdomain.StatusCreated,
		TotalAmountCent:    preview.TotalAmountCent,
		PayAmountCent:      preview.PayAmountCent,
		DiscountAmountCent: preview.DiscountAmountCent,
		IdempotencyKey:     idempotencyKey,
		ReceiverName:       strings.TrimSpace(input.ReceiverName),
		ReceiverPhone:      strings.TrimSpace(input.ReceiverPhone),
		ReceiverAddress:    strings.TrimSpace(input.ReceiverAddress),
		CreatedAt:          now,
		UpdatedAt:          now,
		Items:              make([]domain.OrderItem, 0, len(preview.Items)),
	}
	for _, item := range preview.Items {
		order.Items = append(order.Items, domain.OrderItem{
			ProductID:            item.ProductID,
			SKUID:                item.SKUID,
			ProductTitleSnapshot: item.ProductTitle,
			SKUNameSnapshot:      item.SKUName,
			PriceCentSnapshot:    item.PriceCent,
			Quantity:             item.Quantity,
			TotalAmountCent:      item.TotalAmountCent,
			CreatedAt:            now,
			UpdatedAt:            now,
		})
	}
	locks := make([]domain.InventoryLock, 0, len(order.Items))
	for _, item := range order.Items {
		locks = append(locks, domain.InventoryLock{
			SKUID:     item.SKUID,
			Quantity:  item.Quantity,
			Status:    domain.InventoryLockStatusLocked,
			LockedAt:  now,
			CreatedAt: now,
			UpdatedAt: now,
		})
	}
	saved, err := s.repo.SaveOrderWithInventoryLocks(order, locks)
	if err != nil {
		if errors.Is(err, ErrInsufficientStock) {
			return nil, newError(ErrorConflict, "stock is insufficient")
		}
		return nil, err
	}
	_, _ = s.repo.AppendOrderEvent(domain.OrderEvent{
		OrderID:      saved.ID,
		FromStatus:   "",
		ToStatus:     string(orderdomain.StatusCreated),
		EventType:    "ORDER_CREATED",
		OperatorID:   actor.UserID,
		OperatorRole: actor.Role,
		Remark:       "order created",
		CreatedAt:    now,
	})
	_, _ = s.repo.AppendBehaviorEvent(domain.BehaviorEvent{
		UserID:     actor.UserID,
		EventType:  domain.BehaviorOrderCreate,
		OrderID:    saved.ID,
		MerchantID: saved.MerchantID,
		CreatedAt:  now,
	})
	_ = s.repo.DeleteSelectedCartItems(actor.UserID)
	view, err := s.enrichOrderView(saved)
	if err != nil {
		return nil, err
	}
	return &view, nil
}

func (s *Service) ListOrders(ctx context.Context, actor Actor) ([]OrderView, error) {
	_ = ctx
	orders := s.repo.ListOrdersByUser(actor.UserID)
	out := make([]OrderView, 0, len(orders))
	for _, order := range orders {
		view, err := s.enrichOrderView(order)
		if err != nil {
			return nil, err
		}
		out = append(out, view)
	}
	return out, nil
}

func (s *Service) GetOrder(ctx context.Context, actor Actor, orderID int64) (*OrderView, error) {
	_ = ctx
	order, ok := s.repo.GetOrder(orderID)
	if !ok || (order.UserID != actor.UserID && order.MerchantID != actor.MerchantID) {
		return nil, newError(ErrorNotFound, "order not found")
	}
	view, err := s.enrichOrderView(order)
	if err != nil {
		return nil, err
	}
	return &view, nil
}

func (s *Service) PayOrder(ctx context.Context, actor Actor, orderID int64) (*OrderView, error) {
	_ = ctx
	order, ok := s.repo.GetOrder(orderID)
	if !ok || order.UserID != actor.UserID {
		return nil, newError(ErrorNotFound, "order not found")
	}
	if err := orderdomain.Transition(order.Status, orderdomain.StatusPaid); err != nil {
		return nil, newError(ErrorConflict, err.Error())
	}
	now := s.now()
	order.Status = orderdomain.StatusPaid
	order.PaidAt = &now
	order.UpdatedAt = now
	saved, err := s.repo.SaveOrder(order)
	if err != nil {
		return nil, err
	}
	for _, lock := range s.repo.ListInventoryLocksByOrder(saved.ID) {
		sku, ok := s.repo.GetSKU(lock.SKUID)
		if !ok {
			return nil, newError(ErrorNotFound, "sku not found for inventory lock")
		}
		sku.Stock -= lock.Quantity
		sku.LockedStock -= lock.Quantity
		if sku.Stock < 0 || sku.LockedStock < 0 {
			return nil, newError(ErrorConflict, "inventory underflow detected")
		}
		if _, err := s.repo.SaveSKU(sku); err != nil {
			return nil, err
		}
		lock.Status = domain.InventoryLockStatusConfirmed
		lock.ConfirmedAt = &now
		if err := s.repo.UpdateInventoryLock(lock); err != nil {
			return nil, err
		}
	}
	_, _ = s.repo.AppendOrderEvent(domain.OrderEvent{
		OrderID:      saved.ID,
		FromStatus:   string(orderdomain.StatusCreated),
		ToStatus:     string(orderdomain.StatusPaid),
		EventType:    "ORDER_PAID",
		OperatorID:   actor.UserID,
		OperatorRole: actor.Role,
		Remark:       "payment simulated",
		CreatedAt:    now,
	})
	_, _ = s.repo.AppendBehaviorEvent(domain.BehaviorEvent{
		UserID:     actor.UserID,
		EventType:  domain.BehaviorOrderPay,
		OrderID:    saved.ID,
		MerchantID: saved.MerchantID,
		CreatedAt:  now,
	})
	view, err := s.enrichOrderView(saved)
	if err != nil {
		return nil, err
	}
	return &view, nil
}

func (s *Service) CancelOrder(ctx context.Context, actor Actor, orderID int64) (*OrderView, error) {
	_ = ctx
	order, ok := s.repo.GetOrder(orderID)
	if !ok || order.UserID != actor.UserID {
		return nil, newError(ErrorNotFound, "order not found")
	}
	if err := orderdomain.Transition(order.Status, orderdomain.StatusCancelled); err != nil {
		return nil, newError(ErrorConflict, err.Error())
	}
	now := s.now()
	order.Status = orderdomain.StatusCancelled
	order.CancelledAt = &now
	order.UpdatedAt = now
	saved, err := s.repo.SaveOrder(order)
	if err != nil {
		return nil, err
	}
	if err := s.releaseInventory(saved.ID, true); err != nil {
		return nil, err
	}
	_, _ = s.repo.AppendOrderEvent(domain.OrderEvent{
		OrderID:      saved.ID,
		FromStatus:   string(orderdomain.StatusCreated),
		ToStatus:     string(orderdomain.StatusCancelled),
		EventType:    "ORDER_CANCELLED",
		OperatorID:   actor.UserID,
		OperatorRole: actor.Role,
		Remark:       "consumer cancelled before payment",
		CreatedAt:    now,
	})
	_, _ = s.repo.AppendBehaviorEvent(domain.BehaviorEvent{
		UserID:     actor.UserID,
		EventType:  domain.BehaviorOrderCancel,
		OrderID:    saved.ID,
		MerchantID: saved.MerchantID,
		CreatedAt:  now,
	})
	view, err := s.enrichOrderView(saved)
	if err != nil {
		return nil, err
	}
	return &view, nil
}

func (s *Service) FinishOrder(ctx context.Context, actor Actor, orderID int64) (*OrderView, error) {
	_ = ctx
	order, ok := s.repo.GetOrder(orderID)
	if !ok || order.UserID != actor.UserID {
		return nil, newError(ErrorNotFound, "order not found")
	}
	if err := orderdomain.Transition(order.Status, orderdomain.StatusFinished); err != nil {
		return nil, newError(ErrorConflict, err.Error())
	}
	now := s.now()
	order.Status = orderdomain.StatusFinished
	order.FinishedAt = &now
	order.UpdatedAt = now
	saved, err := s.repo.SaveOrder(order)
	if err != nil {
		return nil, err
	}
	_, _ = s.repo.AppendOrderEvent(domain.OrderEvent{
		OrderID:      saved.ID,
		FromStatus:   string(orderdomain.StatusShipped),
		ToStatus:     string(orderdomain.StatusFinished),
		EventType:    "ORDER_FINISHED",
		OperatorID:   actor.UserID,
		OperatorRole: actor.Role,
		Remark:       "consumer confirmed receipt",
		CreatedAt:    now,
	})
	view, err := s.enrichOrderView(saved)
	if err != nil {
		return nil, err
	}
	return &view, nil
}

func (s *Service) RequestRefund(ctx context.Context, actor Actor, orderID int64, input RefundRequestInput) (*OrderView, error) {
	_ = ctx
	order, ok := s.repo.GetOrder(orderID)
	if !ok || order.UserID != actor.UserID {
		return nil, newError(ErrorNotFound, "order not found")
	}
	if err := orderdomain.Transition(order.Status, orderdomain.StatusRefunding); err != nil {
		return nil, newError(ErrorConflict, err.Error())
	}
	now := s.now()
	prevStatus := order.Status
	order.Status = orderdomain.StatusRefunding
	order.UpdatedAt = now
	saved, err := s.repo.SaveOrder(order)
	if err != nil {
		return nil, err
	}
	_, _ = s.repo.AppendOrderEvent(domain.OrderEvent{
		OrderID:      saved.ID,
		FromStatus:   string(prevStatus),
		ToStatus:     string(orderdomain.StatusRefunding),
		EventType:    "ORDER_REFUND_REQUESTED",
		OperatorID:   actor.UserID,
		OperatorRole: actor.Role,
		Remark:       strings.TrimSpace(input.Reason),
		CreatedAt:    now,
	})
	view, err := s.enrichOrderView(saved)
	if err != nil {
		return nil, err
	}
	return &view, nil
}

func (s *Service) MerchantListProducts(ctx context.Context, actor Actor) ([]ProductDetail, error) {
	_ = ctx
	if actor.Role != domain.RoleMerchant {
		return nil, newError(ErrorForbidden, "merchant access required")
	}
	products := s.repo.ListProducts()
	out := make([]ProductDetail, 0)
	for _, product := range products {
		if product.MerchantID != actor.MerchantID {
			continue
		}
		out = append(out, s.toProductDetail(product))
	}
	return out, nil
}

func (s *Service) MerchantCreateProduct(ctx context.Context, actor Actor, input MerchantProductInput) (*ProductDetail, error) {
	_ = ctx
	if actor.Role != domain.RoleMerchant {
		return nil, newError(ErrorForbidden, "merchant access required")
	}
	if strings.TrimSpace(input.Title) == "" {
		return nil, newError(ErrorInvalidArgument, "title is required")
	}
	product, err := s.repo.SaveProduct(domain.Product{
		MerchantID:    actor.MerchantID,
		Title:         strings.TrimSpace(input.Title),
		Description:   strings.TrimSpace(input.Description),
		CoverURL:      strings.TrimSpace(input.CoverURL),
		CategoryID:    input.CategoryID,
		Status:        domain.ProductStatusDraft,
		SellingPoints: domain.CloneStringSlice(input.SellingPoints),
		CreatedAt:     s.now(),
		UpdatedAt:     s.now(),
	})
	if err != nil {
		return nil, err
	}
	view := s.toProductDetail(product)
	return &view, nil
}

func (s *Service) MerchantUpdateProduct(ctx context.Context, actor Actor, productID int64, input MerchantProductInput) (*ProductDetail, error) {
	_ = ctx
	product, ok := s.repo.GetProduct(productID)
	if !ok || product.MerchantID != actor.MerchantID {
		return nil, newError(ErrorNotFound, "product not found")
	}
	product.Title = strings.TrimSpace(input.Title)
	product.Description = strings.TrimSpace(input.Description)
	product.CoverURL = strings.TrimSpace(input.CoverURL)
	product.CategoryID = input.CategoryID
	product.SellingPoints = domain.CloneStringSlice(input.SellingPoints)
	product.UpdatedAt = s.now()
	saved, err := s.repo.SaveProduct(product)
	if err != nil {
		return nil, err
	}
	view := s.toProductDetail(saved)
	return &view, nil
}

func (s *Service) MerchantCreateSKU(ctx context.Context, actor Actor, productID int64, input MerchantSKUInput) (*SKUView, error) {
	_ = ctx
	product, ok := s.repo.GetProduct(productID)
	if !ok || product.MerchantID != actor.MerchantID {
		return nil, newError(ErrorNotFound, "product not found")
	}
	if input.PriceCent <= 0 || input.Stock < 0 {
		return nil, newError(ErrorInvalidArgument, "price and stock must be valid")
	}
	status := input.Status
	if status == "" {
		status = domain.SKUStatusActive
	}
	sku, err := s.repo.SaveSKU(domain.SKU{
		ProductID:   product.ID,
		SKUName:     strings.TrimSpace(input.SKUName),
		SKUAttrs:    domain.CloneMap(input.SKUAttrs),
		PriceCent:   input.PriceCent,
		Stock:       input.Stock,
		LockedStock: 0,
		Status:      status,
		CreatedAt:   s.now(),
		UpdatedAt:   s.now(),
	})
	if err != nil {
		return nil, err
	}
	view := s.toSKUView(sku)
	return &view, nil
}

func (s *Service) MerchantUpdateSKU(ctx context.Context, actor Actor, skuID int64, input MerchantSKUInput) (*SKUView, error) {
	_ = ctx
	sku, ok := s.repo.GetSKU(skuID)
	if !ok {
		return nil, newError(ErrorNotFound, "sku not found")
	}
	product, ok := s.repo.GetProduct(sku.ProductID)
	if !ok || product.MerchantID != actor.MerchantID {
		return nil, newError(ErrorNotFound, "product not found")
	}
	if input.SKUName != "" {
		sku.SKUName = strings.TrimSpace(input.SKUName)
	}
	if input.SKUAttrs != nil {
		sku.SKUAttrs = domain.CloneMap(input.SKUAttrs)
	}
	if input.PriceCent > 0 {
		sku.PriceCent = input.PriceCent
	}
	if input.Stock >= 0 {
		sku.Stock = input.Stock
	}
	if input.Status != "" {
		sku.Status = input.Status
	}
	sku.UpdatedAt = s.now()
	saved, err := s.repo.SaveSKU(sku)
	if err != nil {
		return nil, err
	}
	view := s.toSKUView(saved)
	return &view, nil
}

func (s *Service) MerchantSetProductStatus(ctx context.Context, actor Actor, productID int64, status string) (*ProductDetail, error) {
	_ = ctx
	product, ok := s.repo.GetProduct(productID)
	if !ok || product.MerchantID != actor.MerchantID {
		return nil, newError(ErrorNotFound, "product not found")
	}
	product.Status = status
	product.UpdatedAt = s.now()
	saved, err := s.repo.SaveProduct(product)
	if err != nil {
		return nil, err
	}
	view := s.toProductDetail(saved)
	return &view, nil
}

func (s *Service) MerchantListOrders(ctx context.Context, actor Actor) ([]OrderView, error) {
	_ = ctx
	if actor.Role != domain.RoleMerchant {
		return nil, newError(ErrorForbidden, "merchant access required")
	}
	orders := s.repo.ListOrdersByMerchant(actor.MerchantID)
	out := make([]OrderView, 0, len(orders))
	for _, order := range orders {
		view, err := s.enrichOrderView(order)
		if err != nil {
			return nil, err
		}
		out = append(out, view)
	}
	return out, nil
}

func (s *Service) MerchantShipOrder(ctx context.Context, actor Actor, orderID int64, input MerchantOrderShipInput) (*OrderView, error) {
	_ = ctx
	order, ok := s.repo.GetOrder(orderID)
	if !ok || order.MerchantID != actor.MerchantID {
		return nil, newError(ErrorNotFound, "order not found")
	}
	if err := orderdomain.Transition(order.Status, orderdomain.StatusShipped); err != nil {
		return nil, newError(ErrorConflict, err.Error())
	}
	now := s.now()
	order.Status = orderdomain.StatusShipped
	order.ShippedAt = &now
	order.UpdatedAt = now
	saved, err := s.repo.SaveOrder(order)
	if err != nil {
		return nil, err
	}
	_, _ = s.repo.AppendOrderEvent(domain.OrderEvent{
		OrderID:      saved.ID,
		FromStatus:   string(orderdomain.StatusPaid),
		ToStatus:     string(orderdomain.StatusShipped),
		EventType:    "ORDER_SHIPPED",
		OperatorID:   actor.UserID,
		OperatorRole: actor.Role,
		Remark:       strings.TrimSpace(input.LogisticsNo),
		CreatedAt:    now,
	})
	view, err := s.enrichOrderView(saved)
	if err != nil {
		return nil, err
	}
	return &view, nil
}

func (s *Service) MerchantApproveRefund(ctx context.Context, actor Actor, orderID int64) (*OrderView, error) {
	_ = ctx
	order, ok := s.repo.GetOrder(orderID)
	if !ok || order.MerchantID != actor.MerchantID {
		return nil, newError(ErrorNotFound, "order not found")
	}
	if err := orderdomain.Transition(order.Status, orderdomain.StatusRefunded); err != nil {
		return nil, newError(ErrorConflict, err.Error())
	}
	now := s.now()
	order.Status = orderdomain.StatusRefunded
	order.UpdatedAt = now
	saved, err := s.repo.SaveOrder(order)
	if err != nil {
		return nil, err
	}
	if err := s.releaseInventory(saved.ID, false); err != nil {
		return nil, err
	}
	_, _ = s.repo.AppendOrderEvent(domain.OrderEvent{
		OrderID:      saved.ID,
		FromStatus:   string(orderdomain.StatusRefunding),
		ToStatus:     string(orderdomain.StatusRefunded),
		EventType:    "ORDER_REFUNDED",
		OperatorID:   actor.UserID,
		OperatorRole: actor.Role,
		Remark:       "merchant approved refund",
		CreatedAt:    now,
	})
	_, _ = s.repo.AppendBehaviorEvent(domain.BehaviorEvent{
		EventType:  domain.BehaviorOrderRefund,
		OrderID:    saved.ID,
		MerchantID: saved.MerchantID,
		CreatedAt:  now,
	})
	view, err := s.enrichOrderView(saved)
	if err != nil {
		return nil, err
	}
	return &view, nil
}

func (s *Service) DashboardFunnel(ctx context.Context, actor Actor) (*DashboardFunnel, error) {
	_ = ctx
	if actor.Role != domain.RoleMerchant {
		return nil, newError(ErrorForbidden, "merchant access required")
	}
	result := &DashboardFunnel{}
	for _, event := range s.repo.ListBehaviorEvents() {
		if event.MerchantID != actor.MerchantID {
			continue
		}
		switch event.EventType {
		case domain.BehaviorNoteView:
			result.NoteViews++
		case domain.BehaviorProductClick:
			result.ProductClicks++
		case domain.BehaviorAddToCart:
			result.AddToCart++
		case domain.BehaviorOrderCreate:
			result.OrderCreate++
		case domain.BehaviorOrderPay:
			result.OrderPay++
		case domain.BehaviorOrderRefund:
			result.OrderRefund++
		}
	}
	return result, nil
}

func (s *Service) DashboardProducts(ctx context.Context, actor Actor) ([]DashboardProductStat, error) {
	_ = ctx
	if actor.Role != domain.RoleMerchant {
		return nil, newError(ErrorForbidden, "merchant access required")
	}
	stats := make(map[int64]*DashboardProductStat)
	for _, product := range s.repo.ListProducts() {
		if product.MerchantID != actor.MerchantID {
			continue
		}
		available := 0
		for _, sku := range s.repo.ListSKUsByProduct(product.ID) {
			available += sku.Stock
		}
		stats[product.ID] = &DashboardProductStat{
			ProductID:      product.ID,
			Title:          product.Title,
			Status:         product.Status,
			AvailableStock: available,
		}
	}
	for _, event := range s.repo.ListBehaviorEvents() {
		stat, ok := stats[event.ProductID]
		if !ok {
			continue
		}
		switch event.EventType {
		case domain.BehaviorNoteView:
			stat.Exposure++
		case domain.BehaviorProductClick:
			stat.Clicks++
		case domain.BehaviorAddToCart:
			stat.AddToCart++
		case domain.BehaviorOrderCreate:
			stat.Orders++
		case domain.BehaviorOrderPay:
			stat.Paid++
		case domain.BehaviorOrderRefund:
			stat.Refunds++
		}
	}
	out := make([]DashboardProductStat, 0, len(stats))
	for _, stat := range stats {
		out = append(out, *stat)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ProductID < out[j].ProductID })
	return out, nil
}

func (s *Service) DashboardSummary(ctx context.Context, actor Actor) (*DashboardSummary, error) {
	_ = ctx
	if actor.Role != domain.RoleMerchant {
		return nil, newError(ErrorForbidden, "merchant access required")
	}
	summary := &DashboardSummary{}
	for _, product := range s.repo.ListProducts() {
		if product.MerchantID != actor.MerchantID {
			continue
		}
		summary.ProductCount++
		if product.Status == domain.ProductStatusOnline {
			summary.OnlineProductCount++
		}
		for _, sku := range s.repo.ListSKUsByProduct(product.ID) {
			if sku.Stock <= 5 {
				summary.InventoryWarningSKU++
			}
		}
	}
	for _, order := range s.repo.ListOrdersByMerchant(actor.MerchantID) {
		summary.OrderCount++
		if order.Status == orderdomain.StatusPaid || order.Status == orderdomain.StatusShipped || order.Status == orderdomain.StatusFinished || order.Status == orderdomain.StatusRefunding || order.Status == orderdomain.StatusRefunded {
			summary.PaidOrderCount++
			summary.GMVAmountCent += order.PayAmountCent
		}
		if order.Status == orderdomain.StatusRefunded || order.Status == orderdomain.StatusRefunding {
			summary.RefundOrderCount++
		}
	}
	return summary, nil
}

func (s *Service) GenerateSellingPoints(ctx context.Context, actor Actor, input SellingPointInput) (*AITaskView, error) {
	if actor.Role != domain.RoleMerchant {
		return nil, newError(ErrorForbidden, "merchant access required")
	}
	task, err := s.repo.CreateAITask(domain.AIGenerationTask{
		UserID:     actor.UserID,
		MerchantID: actor.MerchantID,
		TaskType:   domain.TaskTypeSellingPoints,
		Input: map[string]any{
			"product_name": input.ProductName,
			"attributes":   input.Attributes,
			"target_users": input.TargetUsers,
			"price_cent":   input.PriceCent,
			"reviews":      input.Reviews,
		},
		Status:    domain.AITaskStatusPending,
		CreatedAt: s.now(),
		UpdatedAt: s.now(),
	})
	if err != nil {
		return nil, err
	}
	result, err := s.aiProvider.GenerateSellingPoints(ctx, backendai.SellingPointRequest{
		ProductName: input.ProductName,
		Audience:    input.TargetUsers,
		Attributes:  input.Attributes,
		Reviews:     input.Reviews,
	})
	if err != nil {
		task.Status = domain.AITaskStatusFailed
		task.ErrorMessage = err.Error()
		_ = s.repo.UpdateAITask(task)
		return nil, err
	}
	task.Status = domain.AITaskStatusCompleted
	task.Output = map[string]any{
		"core_points":          result.Points,
		"scenarios":            []string{"通勤补妆", "宿舍整理", "出差旅行"},
		"pain_points":          []string{"出门前找不到单品", "内容拍摄补妆慢"},
		"detail_title_suggest": fmt.Sprintf("%s｜内容电商详情页标题建议", input.ProductName),
		"note_copy_suggest":    fmt.Sprintf("围绕 %s 生成适合小红书的种草文案", input.ProductName),
	}
	_ = s.repo.UpdateAITask(task)
	view := s.toAITaskView(task)
	return &view, nil
}

func (s *Service) GenerateBusinessReview(ctx context.Context, actor Actor, input BusinessReviewInput) (*AITaskView, error) {
	funnel, err := s.DashboardFunnel(ctx, actor)
	if err != nil {
		return nil, err
	}
	summary, err := s.DashboardSummary(ctx, actor)
	if err != nil {
		return nil, err
	}
	task, err := s.repo.CreateAITask(domain.AIGenerationTask{
		UserID:     actor.UserID,
		MerchantID: actor.MerchantID,
		TaskType:   domain.TaskTypeBusinessReview,
		Input: map[string]any{
			"window_days": input.WindowDays,
			"product_id":  input.ProductID,
			"funnel":      funnel,
			"summary":     summary,
		},
		Status:    domain.AITaskStatusPending,
		CreatedAt: s.now(),
		UpdatedAt: s.now(),
	})
	if err != nil {
		return nil, err
	}
	result, err := s.aiProvider.GenerateBusinessReview(ctx, backendai.BusinessReviewRequest{
		WindowDays: input.WindowDays,
		GMV:        summary.GMVAmountCent,
		RefundRate: refundRate(summary),
	})
	if err != nil {
		task.Status = domain.AITaskStatusFailed
		task.ErrorMessage = err.Error()
		_ = s.repo.UpdateAITask(task)
		return nil, err
	}
	task.Status = domain.AITaskStatusCompleted
	task.Output = map[string]any{
		"diagnosis":        result.Diagnosis,
		"possible_reasons": []string{"商品卡点击后卖点不够集中", "加购到支付阶段信息损耗"},
		"optimization":     result.NextSteps,
		"next_experiments": []string{"调整主图与首屏卖点顺序", "按价格带拆分 SKU 对照实验"},
		"window_days":      input.WindowDays,
	}
	_ = s.repo.UpdateAITask(task)
	view := s.toAITaskView(task)
	return &view, nil
}

func (s *Service) GetAITask(ctx context.Context, actor Actor, taskID int64) (*AITaskView, error) {
	_ = ctx
	task, ok := s.repo.GetAITask(taskID)
	if !ok || !canReadAITask(actor, task) {
		return nil, newError(ErrorNotFound, "ai task not found")
	}
	view := s.toAITaskView(task)
	return &view, nil
}

func canReadAITask(actor Actor, task domain.AIGenerationTask) bool {
	if actor.Role == domain.RoleMerchant {
		return actor.MerchantID != 0 && task.MerchantID == actor.MerchantID
	}
	return task.UserID == actor.UserID
}

type checkoutLine struct {
	product  domain.Product
	sku      domain.SKU
	quantity int
}

func (s *Service) normalizeCheckoutLines(actor Actor, items []OrderLineInput) ([]checkoutLine, error) {
	lines := items
	if len(lines) == 0 {
		for _, item := range s.repo.ListCartItems(actor.UserID) {
			if item.Selected {
				lines = append(lines, OrderLineInput{SKUID: item.SKUID, Quantity: item.Quantity})
			}
		}
	}
	if len(lines) == 0 {
		return nil, newError(ErrorInvalidArgument, "checkout items are required")
	}
	out := make([]checkoutLine, 0, len(lines))
	var merchantID int64
	for _, line := range lines {
		if line.Quantity <= 0 {
			return nil, newError(ErrorInvalidArgument, "quantity must be positive")
		}
		sku, ok := s.repo.GetSKU(line.SKUID)
		if !ok {
			return nil, newError(ErrorNotFound, "sku not found")
		}
		product, ok := s.repo.GetProduct(sku.ProductID)
		if !ok {
			return nil, newError(ErrorNotFound, "product not found")
		}
		if product.Status != domain.ProductStatusOnline {
			return nil, newError(ErrorConflict, "product is offline")
		}
		if sku.Status != domain.SKUStatusActive {
			return nil, newError(ErrorConflict, "sku is inactive")
		}
		if sku.Stock-sku.LockedStock < line.Quantity {
			return nil, newError(ErrorConflict, "stock is insufficient")
		}
		if merchantID == 0 {
			merchantID = product.MerchantID
		}
		if merchantID != product.MerchantID {
			return nil, newError(ErrorInvalidArgument, "cross-merchant checkout is not supported")
		}
		out = append(out, checkoutLine{product: product, sku: sku, quantity: line.Quantity})
	}
	return out, nil
}

func (s *Service) buildOrderPreview(lines []checkoutLine) (*OrderPreview, error) {
	if len(lines) == 0 {
		return nil, newError(ErrorInvalidArgument, "checkout items are required")
	}
	preview := &OrderPreview{
		MerchantID:         lines[0].product.MerchantID,
		Items:              make([]OrderItemView, 0, len(lines)),
		DiscountAmountCent: 0,
		StockOK:            true,
	}
	for _, line := range lines {
		total := int64(line.quantity) * line.sku.PriceCent
		preview.Items = append(preview.Items, OrderItemView{
			ProductID:       line.product.ID,
			SKUID:           line.sku.ID,
			ProductTitle:    line.product.Title,
			SKUName:         line.sku.SKUName,
			PriceCent:       line.sku.PriceCent,
			Quantity:        line.quantity,
			TotalAmountCent: total,
		})
		preview.TotalAmountCent += total
	}
	preview.PayAmountCent = preview.TotalAmountCent - preview.DiscountAmountCent
	return preview, nil
}

func (s *Service) buildCartView(items []domain.CartItem) CartView {
	out := CartView{Items: make([]CartItemView, 0, len(items))}
	for _, item := range items {
		view := s.toCartItemView(item)
		out.Items = append(out.Items, view)
		if view.Selected {
			out.SelectedItemCount++
			out.SelectedQuantity += view.Quantity
			out.SelectedAmountCent += int64(view.Quantity) * view.PriceCent
		}
	}
	return out
}

func (s *Service) releaseInventory(orderID int64, fromLocked bool) error {
	now := s.now()
	for _, lock := range s.repo.ListInventoryLocksByOrder(orderID) {
		sku, ok := s.repo.GetSKU(lock.SKUID)
		if !ok {
			return newError(ErrorNotFound, "sku not found for inventory release")
		}
		switch {
		case fromLocked && lock.Status == domain.InventoryLockStatusLocked:
			sku.LockedStock -= lock.Quantity
		case !fromLocked && lock.Status == domain.InventoryLockStatusConfirmed:
			sku.Stock += lock.Quantity
		}
		if sku.LockedStock < 0 || sku.Stock < 0 {
			return newError(ErrorConflict, "inventory underflow detected")
		}
		if _, err := s.repo.SaveSKU(sku); err != nil {
			return err
		}
		lock.Status = domain.InventoryLockStatusReleased
		lock.ReleasedAt = &now
		if err := s.repo.UpdateInventoryLock(lock); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) enrichOrderView(order domain.Order) (OrderView, error) {
	view := OrderView{
		ID:                 order.ID,
		OrderNo:            order.OrderNo,
		UserID:             order.UserID,
		MerchantID:         order.MerchantID,
		Status:             string(order.Status),
		TotalAmountCent:    order.TotalAmountCent,
		PayAmountCent:      order.PayAmountCent,
		DiscountAmountCent: order.DiscountAmountCent,
		ReceiverName:       order.ReceiverName,
		ReceiverPhone:      order.ReceiverPhone,
		ReceiverAddress:    order.ReceiverAddress,
		PaidAt:             order.PaidAt,
		CancelledAt:        order.CancelledAt,
		ShippedAt:          order.ShippedAt,
		FinishedAt:         order.FinishedAt,
		CreatedAt:          order.CreatedAt,
		UpdatedAt:          order.UpdatedAt,
		Items:              make([]OrderItemView, 0, len(order.Items)),
		Events:             make([]OrderEventView, 0),
		InventoryLocks:     make([]InventoryLockView, 0),
	}
	for _, item := range order.Items {
		view.Items = append(view.Items, OrderItemView{
			ID:              item.ID,
			ProductID:       item.ProductID,
			SKUID:           item.SKUID,
			ProductTitle:    item.ProductTitleSnapshot,
			SKUName:         item.SKUNameSnapshot,
			PriceCent:       item.PriceCentSnapshot,
			Quantity:        item.Quantity,
			TotalAmountCent: item.TotalAmountCent,
		})
	}
	for _, event := range s.repo.ListOrderEvents(order.ID) {
		view.Events = append(view.Events, OrderEventView{
			ID:           event.ID,
			FromStatus:   event.FromStatus,
			ToStatus:     event.ToStatus,
			EventType:    event.EventType,
			OperatorID:   event.OperatorID,
			OperatorRole: event.OperatorRole,
			Remark:       event.Remark,
			CreatedAt:    event.CreatedAt,
		})
	}
	for _, lock := range s.repo.ListInventoryLocksByOrder(order.ID) {
		view.InventoryLocks = append(view.InventoryLocks, InventoryLockView{
			ID:          lock.ID,
			SKUID:       lock.SKUID,
			Quantity:    lock.Quantity,
			Status:      lock.Status,
			LockedAt:    lock.LockedAt,
			ConfirmedAt: lock.ConfirmedAt,
			ReleasedAt:  lock.ReleasedAt,
		})
	}
	return view, nil
}

func (s *Service) toUserView(user domain.User) UserView {
	view := UserView{
		ID:       user.ID,
		Nickname: user.Nickname,
		Phone:    user.Phone,
		Role:     user.Role,
	}
	if merchant, ok := s.repo.GetMerchantByUserID(user.ID); ok {
		view.MerchantID = merchant.ID
		view.Merchant = &MerchantView{
			ID:          merchant.ID,
			Name:        merchant.Name,
			Description: merchant.Description,
			Status:      merchant.Status,
		}
	}
	return view
}

func (s *Service) toNoteSummary(note domain.Note) NoteSummary {
	return NoteSummary{
		ID:             note.ID,
		Title:          note.Title,
		Content:        note.Content,
		CoverURL:       note.CoverURL,
		ViewCount:      note.ViewCount,
		LikeCount:      note.LikeCount,
		LinkedProducts: s.productCards(note.ProductIDs),
	}
}

func (s *Service) toNoteDetail(note domain.Note) NoteDetail {
	return NoteDetail{
		ID:             note.ID,
		Title:          note.Title,
		Content:        note.Content,
		CoverURL:       note.CoverURL,
		ViewCount:      note.ViewCount,
		LikeCount:      note.LikeCount,
		LinkedProducts: s.productCards(note.ProductIDs),
	}
}

func (s *Service) productCards(ids []int64) []ProductCard {
	out := make([]ProductCard, 0, len(ids))
	for _, id := range ids {
		product, ok := s.repo.GetProduct(id)
		if !ok {
			continue
		}
		out = append(out, s.toProductCard(product))
	}
	return out
}

func (s *Service) toProductCard(product domain.Product) ProductCard {
	minPrice := int64(0)
	stock := 0
	for _, sku := range s.repo.ListSKUsByProduct(product.ID) {
		if minPrice == 0 || sku.PriceCent < minPrice {
			minPrice = sku.PriceCent
		}
		stock += sku.Stock - sku.LockedStock
	}
	return ProductCard{
		ID:            product.ID,
		Title:         product.Title,
		CoverURL:      product.CoverURL,
		Status:        product.Status,
		MinPriceCent:  minPrice,
		Stock:         stock,
		SellingPoints: domain.CloneStringSlice(product.SellingPoints),
	}
}

func (s *Service) toProductDetail(product domain.Product) ProductDetail {
	detail := ProductDetail{
		ID:            product.ID,
		MerchantID:    product.MerchantID,
		Title:         product.Title,
		Description:   product.Description,
		CoverURL:      product.CoverURL,
		CategoryID:    product.CategoryID,
		Status:        product.Status,
		SellingPoints: domain.CloneStringSlice(product.SellingPoints),
		SKUs:          make([]SKUView, 0),
	}
	for _, sku := range s.repo.ListSKUsByProduct(product.ID) {
		detail.SKUs = append(detail.SKUs, s.toSKUView(sku))
	}
	return detail
}

func (s *Service) toSKUView(sku domain.SKU) SKUView {
	return SKUView{
		ID:          sku.ID,
		ProductID:   sku.ProductID,
		SKUName:     sku.SKUName,
		SKUAttrs:    domain.CloneMap(sku.SKUAttrs),
		PriceCent:   sku.PriceCent,
		Stock:       sku.Stock,
		LockedStock: sku.LockedStock,
		Status:      sku.Status,
	}
}

func (s *Service) toCartItemView(item domain.CartItem) CartItemView {
	product, _ := s.repo.GetProduct(item.ProductID)
	sku, _ := s.repo.GetSKU(item.SKUID)
	return CartItemView{
		ID:            item.ID,
		ProductID:     item.ProductID,
		ProductTitle:  product.Title,
		CoverURL:      product.CoverURL,
		SKUID:         item.SKUID,
		SKUName:       sku.SKUName,
		PriceCent:     sku.PriceCent,
		Quantity:      item.Quantity,
		Selected:      item.Selected,
		Stock:         sku.Stock - sku.LockedStock,
		Status:        product.Status,
		SellingPoints: domain.CloneStringSlice(product.SellingPoints),
	}
}

func (s *Service) toAITaskView(task domain.AIGenerationTask) AITaskView {
	return AITaskView{
		ID:           task.ID,
		TaskType:     task.TaskType,
		Status:       task.Status,
		Input:        task.Input,
		Output:       task.Output,
		ErrorMessage: task.ErrorMessage,
		CreatedAt:    task.CreatedAt,
		UpdatedAt:    task.UpdatedAt,
	}
}

func (s *Service) issueSession(user domain.User) (*AuthSession, error) {
	token := fmt.Sprintf("redcart-%d-%d", user.ID, s.now().UnixNano())
	s.repo.SaveSession(token, user.ID)
	view := s.toUserView(user)
	return &AuthSession{Token: token, User: view}, nil
}

func hashPassword(password string) string {
	sum := sha256.Sum256([]byte(password))
	return hex.EncodeToString(sum[:])
}

func refundRate(summary *DashboardSummary) float64 {
	if summary.PaidOrderCount == 0 {
		return 0
	}
	return float64(summary.RefundOrderCount) / float64(summary.PaidOrderCount)
}

func (s *Service) primaryMerchantID(productIDs []int64) int64 {
	for _, productID := range productIDs {
		if product, ok := s.repo.GetProduct(productID); ok {
			return product.MerchantID
		}
	}
	return 0
}
