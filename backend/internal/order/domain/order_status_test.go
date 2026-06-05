package domain

import "testing"

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

func TestTerminalStates(t *testing.T) {
	for _, status := range []OrderStatus{StatusFinished, StatusCancelled, StatusRefunded} {
		if !status.IsTerminal() {
			t.Fatalf("expected %s to be terminal", status)
		}
	}
}

func TestInventoryReleaseStates(t *testing.T) {
	if !ReleasesInventory(StatusCancelled) {
		t.Fatal("cancelled orders should release inventory")
	}
	if !ReleasesInventory(StatusRefunded) {
		t.Fatal("refunded orders should release inventory")
	}
	if ReleasesInventory(StatusPaid) {
		t.Fatal("paid orders should not release inventory")
	}
}
