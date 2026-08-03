// Package domain 定义订单领域模型：订单状态取值及其合法流转规则。
package domain

import "fmt"

// OrderStatus 表示订单的领域状态，取值与持久层存储的状态字符串一致。
type OrderStatus string

// 订单状态取值：FINISHED / CANCELLED / REFUNDED 为终态（不可再流转），
// REFUNDING 表示退款流程进行中。
const (
	StatusCreated   OrderStatus = "CREATED"
	StatusPaid      OrderStatus = "PAID"
	StatusShipped   OrderStatus = "SHIPPED"
	StatusFinished  OrderStatus = "FINISHED"
	StatusCancelled OrderStatus = "CANCELLED"
	StatusRefunding OrderStatus = "REFUNDING"
	StatusRefunded  OrderStatus = "REFUNDED"
)

// legalTransitions 定义状态机的合法迁移表；未出现在表中的状态均为终态，
// 不允许再发生流转。
var legalTransitions = map[OrderStatus]map[OrderStatus]struct{}{
	StatusCreated: {
		StatusPaid:      {},
		StatusCancelled: {},
	},
	StatusPaid: {
		StatusShipped:   {},
		StatusRefunding: {},
	},
	StatusShipped: {
		StatusFinished:  {},
		StatusRefunding: {},
	},
	StatusRefunding: {
		StatusRefunded: {},
	},
}

// IsTerminal 报告状态是否为终态（订单已完成、已取消或已退款，不可再流转）。
func (s OrderStatus) IsTerminal() bool {
	return s == StatusFinished || s == StatusCancelled || s == StatusRefunded
}

// CanTransition 报告 from -> to 是否为状态机中定义的合法迁移。
func CanTransition(from, to OrderStatus) bool {
	targets, ok := legalTransitions[from]
	if !ok {
		return false
	}
	_, ok = targets[to]
	return ok
}

// Transition 校验状态迁移，非法迁移返回描述性错误，合法迁移返回 nil。
func Transition(from, to OrderStatus) error {
	if CanTransition(from, to) {
		return nil
	}
	return fmt.Errorf("illegal order status transition: %s -> %s", from, to)
}
