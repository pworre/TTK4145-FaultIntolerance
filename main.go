package main

import (
	"networkDriver/peers"
	"elevator_project/config"
	"elevatorControl/elevator"
	"elevatorDriver/elevio"
	"fmt"
	"log"
	"flag"
)

const peersPort int = 34933

func main() {
	cfg := config.ParseFlag()

	// - - - - - - Initilizing - - - - - - -
	log.Println("Initializing Elevator %d with port %d....", cfg.ID, cfg.Port)
	elevio.Init(fmt.Sprintf("localhost:%d", cfg.Port), elevator.N_FLOORS)

	// Event channels for elevator
	obstructionEvent := make(chan bool)
	buttonEvent := make(chan elevio.ButtonEvent)
	reachFloorEvent := make(chan int)

	// Channels for orders


	// Channels for P2P
	peersTx := make(chan bool)
	peersRx_state := make(chan peers.PeerUpdate)
	peersRx_GlobalOrder := make(chan peers.PeerUpdate)

	// - - - - - - Descend to defined state - - - - - - 
	elevio.SetMotorDirection(elevio.MD_Down)
	for elevio.GetFloor() == -1 {
	}
	elevio.SetMotorDirection(elevio.MD_Stop)
	elevatorState := elevator.NewElevator(elevio.GetFloor(), elevio.MD_Stop, elevator.EB_Idle)
	for elevio.GetObstruction() {
		elevio.SetDoorOpenLamp(true)
	}
	elevio.SetDoorOpenLamp(false)

	// - - - - - - GoRoutines - - - - - - 
	go peers.Transmitter(cfg.Port, cfg.ID, peersTx)
	go peers.Receiver(cfg.Port, peersRx_state)
	go peers.Receiver(cfg.Port, peersRx_GlobalOrder)

	go elevio.PollButtons()


}