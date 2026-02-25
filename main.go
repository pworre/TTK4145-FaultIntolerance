package main

import (
	"elevatorControl/elevator"
	"elevator_project/config"
	"fmt"
	"log"
	"networkDriver/peers"
	"order"
)

const PEERS_PORT int = 34933

func main() {
	mainControl.main()
	cfg := config.ParseFlag()

	// - - - - - - Initilizing - - - - - - -
	log.Println("Initializing Elevator %d with port %d....", cfg.ID, cfg.Port)
	startFloor := elevator.HardwareInit(fmt.Sprintf("localhost:%d", cfg.Port), elevator.N_FLOORS)

	elevatorState := elevator.NewElevator(startFloor, elevator.D_Stop, elevator.EB_Idle)
	for elevator.GetObstruction() {
		elevator.DoorLight(true)
	}
	elevator.DoorLight(false)

	log.Printf("Elevator %d is now at floor %d! Joining network for service...", cfg.ID, elevatorState.Floor)

	// Event channels for elevator
	obstructionEvent := make(chan bool)
	buttonEvent := make(chan elevator.ButtonEvent)
	reachFloorEvent := make(chan int)

	// Channels for orders
	orderBuffer := make(chan []order.Order, 1024)
	ordersConfirmed := make(chan []order.Order, 1024)
	globalOrderCompleted := make(chan [][]bool)

	// Channels for P2P
	peersTx_enable := make(chan bool)
	peersRx_state := make(chan peers.PeerUpdate)
	peersRx_GlobalOrder := make(chan peers.PeerUpdate)

	// - - - - - - GoRoutines - - - - - -
	go peers.Transmitter(cfg.Port, cfg.ID, peersTx_enable)
	go peers.Receiver(cfg.Port, peersRx_state)
	go peers.Receiver(cfg.Port, peersRx_GlobalOrder)

	go elevator.PollButtons(buttonEvent)
	go elevator.PollObstruction(obstructionEvent)
	go elevator.PollFloorSensor(reachFloorEvent)
	// TODO: Add "fsm" for goroutine with orderAssignment
}
