package application

import (
	orderdomain "github.com/example/redcart-copilot/backend/internal/order/domain"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
)

// 以下守卫用于幂等：同一状态流转重复触发（如重复支付/取消/发货）时不报错，直接返回当前订单视图。

// payAlreadyApplied 判断"已支付"是否已生效：已记录支付时间，或订单已处于支付后的状态。
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

// cancelAlreadyApplied 判断取消是否已生效（幂等守卫）。
func cancelAlreadyApplied(order domain.Order) bool {
	return order.CancelledAt != nil || order.Status == orderdomain.StatusCancelled
}

// finishAlreadyApplied 判断确认收货是否已生效（幂等守卫）。
func finishAlreadyApplied(order domain.Order) bool {
	return order.FinishedAt != nil || order.Status == orderdomain.StatusFinished
}

// refundRequestAlreadyApplied 判断"申请退款"是否已生效（幂等守卫）。
func refundRequestAlreadyApplied(order domain.Order) bool {
	return order.Status == orderdomain.StatusRefunding || order.Status == orderdomain.StatusRefunded
}

// shipAlreadyApplied 判断发货是否已生效（幂等守卫）。
func shipAlreadyApplied(order domain.Order) bool {
	return order.ShippedAt != nil || order.Status == orderdomain.StatusShipped || order.Status == orderdomain.StatusFinished
}

// refundApprovalAlreadyApplied 判断"审批退款"是否已生效（与申请退款的守卫区分，幂等分支返回现状）。
func refundApprovalAlreadyApplied(order domain.Order) bool {
	return order.Status == orderdomain.StatusRefunded
}
