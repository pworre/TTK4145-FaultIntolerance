package main

import (
	"network/peers"
	"fmt"
	"log"
)

const peersPort int = 34933

func main() {
	log.Println("Initializing Elevator....")
	// - - - - - - Initilizing - - - - - - -

	// - - - - - - Channels - - - - - - - - -
	peerTx := make(chan bool)
	peerRx_state := make(chan )
	peerRx_order := make(chan )

	buttonEvent := make(chan )
	reachFloorEvent := make(chan )
	stopEvent := make(chan )
	obstructionEvent := make(chan )

}