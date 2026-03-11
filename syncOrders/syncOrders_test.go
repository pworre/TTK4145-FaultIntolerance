package syncOrders

import (
	"elevatorControl/elevator"
	"syncOrders/order"
	"testing"
)

func TestIsCabOrder(t *testing.T) {
	tests := []struct {
		name     string
		input    order.Order
		expected bool
	}{
		{
			name: "Valid cab order",
			input: order.Order{
				PeerID:     "0",
				OrderFloor: 2,
				OrderType:  elevator.B_Cab,
				OrderState: order.SOS_CONFIRMED_REQUEST,
			},
			expected: true,
		},
		{
			name: "Other orderType",
			input: order.Order{
				PeerID:     "1",
				OrderFloor: 1,
				OrderType:  elevator.B_HallUp,
				OrderState: order.SOS_CONFIRMED_REQUEST,
			},
			expected: false,
		},
	}

	for _, testCase := range tests {
		result := IsCabOrder(testCase.input)

		if result != testCase.expected {
			t.Errorf("%s: exp. %v, got %v", testCase.name, testCase.expected, result)
		}
	}
}

func TestOrderListsToRequestArray(t *testing.T) { // Constraints: OrderType is of type elevator.ButtonType {B_HallUp, B_HallDown} excluding B_Cab
	tests := []struct {
		name       string
		hallOrders []order.Order
		cabOrders  []order.Order
		expected   [elevator.N_FLOORS][elevator.N_BUTTONS]bool
	}{
		{
			name: "Hall and cab orders present",
			hallOrders: []order.Order{
				{
					PeerID:     "0",
					OrderFloor: 2,
					OrderType:  elevator.B_HallUp,
					OrderState: order.SOS_CONFIRMED_REQUEST,
				},
				{
					PeerID:     "1",
					OrderFloor: 1,
					OrderType:  elevator.B_HallDown,
					OrderState: order.SOS_UNCONFIRMED_REQUEST,
				},
				{
					PeerID:     "2",
					OrderFloor: 3,
					OrderType:  elevator.B_HallUp,
					OrderState: order.SOS_CONFIRMED_REQUEST,
				},
			},
			cabOrders: []order.Order{
				{
					PeerID:     "0",
					OrderFloor: 0,
					OrderType:  elevator.B_Cab,
					OrderState: order.SOS_CONFIRMED_REQUEST,
				},
			},
			expected: [elevator.N_FLOORS][elevator.N_BUTTONS]bool{
				{false, false, true},
				{false, true, false},
				{true, false, false},
				{true, false, false},
			},
		},
		{
			name:       "No orders",
			hallOrders: []order.Order{},
			cabOrders:  []order.Order{},
			expected: [elevator.N_FLOORS][elevator.N_BUTTONS]bool{
				{false, false, false},
				{false, false, false},
				{false, false, false},
				{false, false, false},
			},
		},
	}
	for _, testCase := range tests {
		result := OrderListsToRequestArray(testCase.hallOrders, testCase.cabOrders)

		if result != testCase.expected {
			t.Errorf("%s: exp. %v, got %v", testCase.name, testCase.expected, result)
		}
	}
}

/*
func TestRequestsAbove(t *testing.T) {}

func TestRequestsBelow(t *testing.T) {}

func TestWhichButtonsShouldClear(t *testing.T) {}

func TestIsAlreadyInConfirmedList(t *testing.T) {}

func TestHasNoOrders(t *testing.T) {}

func TestNormalizeOrderMapWithoutMyself(t *testing.T) {}
*/
