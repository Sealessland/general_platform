package event

import "testing"

func TestEventTypeTopic(t *testing.T) {
	cases := []struct {
		typ   Type
		topic string
	}{
		{TypeOrderCreated, "order.created"},
		{TypeOrderPaid, "order.paid"},
		{TypeOrderCancelled, "order.cancelled"},
		{TypeOrderShipped, "order.shipped"},
		{TypeOrderFinished, "order.finished"},
		{TypeOrderRefundRequested, "order.refund_requested"},
		{TypeOrderRefunded, "order.refunded"},
		{TypeBehaviorNoteView, "behavior.note_view"},
		{TypeBehaviorProductClick, "behavior.product_click"},
		{TypeBehaviorAddToCart, "behavior.add_to_cart"},
		{TypeBehaviorOrderCreate, "behavior.order_create"},
		{TypeBehaviorOrderPay, "behavior.order_pay"},
		{TypeBehaviorOrderCancel, "behavior.order_cancel"},
		{TypeBehaviorOrderRefund, "behavior.order_refund"},
		{Type("UNKNOWN"), "unknown"},
	}
	for _, c := range cases {
		if got := c.typ.Topic(); got != c.topic {
			t.Errorf("%s.Topic() = %q, want %q", c.typ, got, c.topic)
		}
	}
}
