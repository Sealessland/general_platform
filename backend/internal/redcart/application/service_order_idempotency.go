package application

import (
	orderdomain "github.com/example/redcart-copilot/backend/internal/order/domain"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
)

func (s *Service) currentOrderView(order domain.Order) (*OrderView, error) {
	view, err := s.enrichOrderView(order)
	if err != nil {
		return nil, err
	}
	return &view, nil
}

func payAlreadyApplied(order domain.Order) bool {
	if order.PaidAt != nil {
		return true
	}
	switch order.Status {
	case orderdomain.StatusPaid, orderdomain.StatusShipped, orderdomain.StatusFinished, orderdomain.StatusRefunding, orderdomain.StatusRefunded:
		return true
	default:
		return false
	}
}

func cancelAlreadyApplied(order domain.Order) bool {
	return order.CancelledAt != nil || order.Status == orderdomain.StatusCancelled
}

func finishAlreadyApplied(order domain.Order) bool {
	return order.FinishedAt != nil || order.Status == orderdomain.StatusFinished
}

func refundAlreadyApplied(order domain.Order) bool {
	return order.Status == orderdomain.StatusRefunding || order.Status == orderdomain.StatusRefunded
}

func shipAlreadyApplied(order domain.Order) bool {
	return order.ShippedAt != nil || order.Status == orderdomain.StatusShipped || order.Status == orderdomain.StatusFinished
}

func refundApproveAlreadyApplied(order domain.Order) bool {
	return order.Status == orderdomain.StatusRefunded
}
