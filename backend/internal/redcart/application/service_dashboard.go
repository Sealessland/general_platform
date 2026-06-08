package application

import (
	"context"
	"sort"

	orderdomain "github.com/example/redcart-copilot/backend/internal/order/domain"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
)

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

func refundRate(summary *DashboardSummary) float64 {
	if summary.PaidOrderCount == 0 {
		return 0
	}
	return float64(summary.RefundOrderCount) / float64(summary.PaidOrderCount)
}
