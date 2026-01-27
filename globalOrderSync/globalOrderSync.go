package globalordersync

import (
	"fmt"
	"log"
)

// ? Peer routing table [1, 2, 3, 4, ..., n] - Makes order of who transmits to who

// TODO: Define "dir" struct in elevator and import. DIR = UP / DOWN / BOTH
type globalOrders struct {
	hallOrders map[int]dir
	cabOrders map[string]int
}

type msgState struct {
	globalID int
	timeStamp uint64
	elevState Elevator
	globalOrders
}
