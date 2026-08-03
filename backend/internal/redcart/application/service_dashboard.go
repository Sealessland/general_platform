package application

import (
	"context"
	"sort"

	orderdomain "github.com/example/redcart-copilot/backend/internal/order/domain"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
)

// DashboardFunnel 统计当前商家各转化阶段的行为事件数（浏览→点击→加购→下单→支付→退款）。
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

// DashboardProducts 按商品维度聚合行为事件与可用库存，结果按商品 ID 升序返回。
func (s *Service) DashboardProducts(ctx context.Context, actor Actor) ([]DashboardProductStat, error) {
	_ = ctx
	if actor.Role != domain.RoleMerchant {
		return nil, newError(ErrorForbidden, "merchant access required")
	}
	stats := make(map[int64]*DashboardProductStat)
	for _, product := range s.repo.ListProducts(0, 0) {
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

// DashboardSummary 汇总商家经营指标：商品/订单数量、GMV、退款数与库存预警 SKU 数（库存 <= 5 视为预警）。
func (s *Service) DashboardSummary(ctx context.Context, actor Actor) (*DashboardSummary, error) {
	_ = ctx
	if actor.Role != domain.RoleMerchant {
		return nil, newError(ErrorForbidden, "merchant access required")
	}
	summary := &DashboardSummary{}
	for _, product := range s.repo.ListProducts(0, 0) {
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
	for _, order := range s.repo.ListOrdersByMerchant(actor.MerchantID, 0, 0) {
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

// refundRate 计算退款率：已支付订单数为 0 时返回 0 以避免除零。
func refundRate(summary *DashboardSummary) float64 {
	if summary.PaidOrderCount == 0 {
		return 0
	}
	return float64(summary.RefundOrderCount) / float64(summary.PaidOrderCount)
}
