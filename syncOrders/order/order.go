package order

import (
	"elevatorControl/elevator"
)

type OrderMap map[string]Order

type CurrentOrderState int
const (
	COS_UNKNOWN              = -1
	COS_NONE                 = 0
	COS_UNCONFIRMED_REQUEST  = 1
	COS_CONFIRMED_REQUEST    = 2
	COS_UNCONFIRMED_DELETION = 3
	COS_READY_TO_DELETE      = 4
)

type Order struct {
	PeerID            string
	OrderFloor        int
	OrderType         elevator.Button
	CurrentOrderState CurrentOrderState
}
