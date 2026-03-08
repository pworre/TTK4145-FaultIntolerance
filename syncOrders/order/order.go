package order

import (
	"elevatorControl/elevator"
)

type SyncOrderState int

const (
	SOS_UNKNOWN              = -1
	SOS_NONE                 = 0
	SOS_UNCONFIRMED_REQUEST  = 1
	SOS_CONFIRMED_REQUEST    = 2
	SOS_UNCONFIRMED_DELETION = 3
	SOS_CONFIRMED_DELETION   = 4
)

type Order struct {
	PeerID     string
	OrderFloor int
	OrderType  elevator.Button
	OrderState      SyncOrderState
}

func NewOrder(ID string, floor int, button elevator.Button, orderState SyncOrderState) Order {
	return Order{
		PeerID:     ID,
		OrderFloor: floor,
		OrderType:  button,
		OrderState: orderState,
	}
}