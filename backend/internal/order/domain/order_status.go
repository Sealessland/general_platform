package domain

import "fmt"

type OrderStatus string

const (
	StatusCreated   OrderStatus = "CREATED"
	StatusPaid      OrderStatus = "PAID"
	StatusShipped   OrderStatus = "SHIPPED"
	StatusFinished  OrderStatus = "FINISHED"
	StatusCancelled OrderStatus = "CANCELLED"
	StatusRefunding OrderStatus = "REFUNDING"
	StatusRefunded  OrderStatus = "REFUNDED"
)

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

func (s OrderStatus) IsTerminal() bool {
	return s == StatusFinished || s == StatusCancelled || s == StatusRefunded
}

func CanTransition(from, to OrderStatus) bool {
	targets, ok := legalTransitions[from]
	if !ok {
		return false
	}
	_, ok = targets[to]
	return ok
}

func Transition(from, to OrderStatus) error {
	if CanTransition(from, to) {
		return nil
	}
	return fmt.Errorf("illegal order status transition: %s -> %s", from, to)
}

func ReleasesInventory(status OrderStatus) bool {
	return status == StatusCancelled || status == StatusRefunded
}
