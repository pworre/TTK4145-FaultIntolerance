package globalordersync

import (
	"fmt"
	"log"
	"elevatorControl/elevator"
)

// ? Peer routing table [1, 2, 3, 4, ..., n] - Makes order of who transmits to who

type globalOrders struct {
	// STRUCT OF MAP:  [floor : Direction]
	hallOrders map[int]elevator.MotorDirection

	// STRUCT OF MAP: 	[ID of responsible elev : floor]
	cabOrders map[int]int
}

type msgState struct {
	GlobalID 		int
	TimeStamp 		uint64
	ElevState 		elevator.Elevator
	GlobalOrders	globalOrders
}
