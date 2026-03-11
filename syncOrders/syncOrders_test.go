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

func TestOrderListsToRequestArray(t *testing.T) {

}

/*
func TestRequestsAbove(t *testing.T) {}

func TestRequestsBelow(t *testing.T) {}

func TestWhichButtonsShouldClear(t *testing.T) {}

func TestIsAlreadyInConfirmedList(t *testing.T) {}

func TestHasNoOrders(t *testing.T) {}

func TestNormalizeOrderMapWithoutMyself(t *testing.T) {}
*/
