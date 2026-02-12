package main

import (
	/*
	"networkDriver/peers"
	"elevator_project/config"
	"elevatorControl/elevator"
	"elevatorDriver/elevio"
	"fmt"
	"log"
	"flag"
	*/
	"elevator_project/elevatorControl/mainControl"
)

const peersPort int = 34933

func main() {
	mainControl.mainControl()
	/*
	cfg := config.ParseFlag()

	// - - - - - - Initilizing - - - - - - -
	log.Println("Initializing Elevator %d with port %d", cfg.ID, cfg.Port)
	


	// - - - - - - Channels - - - - - - - - -
	peerTx := make(chan bool)
	peerRx_state := make(chan )
	peerRx_order := make(chan )

	buttonEvent := make(chan )
	reachFloorEvent := make(chan )
	stopEvent := make(chan )
	obstructionEvent := make(chan )
	*/
}