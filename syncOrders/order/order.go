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
	PeerID     		string
	OrderFloor 		int
	OrderType  		elevator.Button
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

func NewEmptyOrder(id string) Order {
	return Order{
		PeerID:     id,
		OrderFloor: -1,
		OrderType:  elevator.Button(-1),
		OrderState: SOS_NONE,
	}
}

func MapClone[M ~map[string]T, T any](theMap M) M {
	clone := make(M, len(theMap))
	for key, val := range theMap {
		clone[key] = val
	}
	return clone
}

// ! For all map clones, just use the function above, and for slices, just use slices.Clone()

/* 
func CloneOrderMap(mapToClone map[string]Order) map[string]Order {
	clonedMap := make(map[string]Order, len(mapToClone))
	for id, order := range mapToClone {
		clonedMap[id] = order
	}
	return clonedMap
}

func CloneAllElevatorStateMap(mapToClone map[string]elevator.Elevator) map[string]elevator.Elevator {
	clonedMap := make(map[string]elevator.Elevator, len(mapToClone))
	for id, elevator := range mapToClone {
		clonedMap[id] = elevator
	}
	return clonedMap
}


func CloneCabOrders(mapToClone map[string][]Order) map[string][]Order {
	clonedMap := make(map[string][]Order, len(mapToClone))
	for id, orders := range mapToClone {
		clonedOrders := make([]Order, len(orders))
		copy(clonedOrders, orders)
		clonedMap[id] = clonedOrders
	}
	return clonedMap
}

func CloneHallOrders(listToClone []Order) []Order {
	clonedList := make([]Order, len(listToClone))
	copy(clonedList, listToClone)
	return clonedList
}

*/