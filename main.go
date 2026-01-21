package main

import (
	"fmt"
	"log"
)

func main() {
	log.Println("Initializing Elevator....")
	// - - - - - - Initilizing - - - - - - -

	// - - - - - - Channels - - - - - - - - -
	peerTx := make(chan bool)
	peerRx_state := make(chan )
	peerRx_order := make(chan )

}