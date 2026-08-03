package domain

import "testing"

// TestLegalOrderTransitions 验证状态机中所有合法迁移均被允许。
func TestLegalOrderTransitions(t *testing.T) {
	cases := []struct {
		from OrderStatus
		to   OrderStatus
	}{
		{StatusCreated, StatusPaid},
		{StatusCreated, StatusCancelled},
		{StatusPaid, StatusShipped},
		{StatusPaid, StatusRefunding},
		{StatusShipped, StatusFinished},
		{StatusShipped, StatusRefunding},
		{StatusRefunding, StatusRefunded},
	}

	for _, tc := range cases {
		if err := Transition(tc.from, tc.to); err != nil {
			t.Fatalf("expected %s -> %s to be legal: %v", tc.from, tc.to, err)
		}
	}
}

// TestIllegalOrderTransitions 验证所有非法迁移都会被拒绝。
func TestIllegalOrderTransitions(t *testing.T) {
	cases := []struct {
		from OrderStatus
		to   OrderStatus
	}{
		{StatusCreated, StatusShipped},
		{StatusPaid, StatusCancelled},
		{StatusFinished, StatusRefunding},
		{StatusCancelled, StatusPaid},
		{StatusRefunded, StatusPaid},
	}

	for _, tc := range cases {
		if err := Transition(tc.from, tc.to); err == nil {
			t.Fatalf("expected %s -> %s to be illegal", tc.from, tc.to)
		}
	}
}

// TestTerminalStates 验证终态集合中的状态均被识别为终态。
func TestTerminalStates(t *testing.T) {
	for _, status := range []OrderStatus{StatusFinished, StatusCancelled, StatusRefunded} {
		if !status.IsTerminal() {
			t.Fatalf("expected %s to be terminal", status)
		}
	}
}
