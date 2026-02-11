package main

import (
	"networkDriver/peers"
	"elevator_project/config"
	"elevatorControl/elevator"
	"elevatorDriver/elevio"
	"order"
	"fmt"
	"log"
)

const peersPort int = 34933

func main() {
	cfg := config.ParseFlag()

	// - - - - - - Initilizing - - - - - - -
	log.Println("Initializing Elevator %d with port %d....", cfg.ID, cfg.Port)
	elevio.Init(fmt.Sprintf("localhost:%d", cfg.Port), elevator.N_FLOORS)

	// Event channels for elevator
	obstructionEvent_ch := make(chan bool, 1024)
	buttonEvent_ch := make(chan elevio.ButtonEvent, 1024)
	reachFloorEvent_ch := make(chan int, 1024)

	// Channels for orders
	orderBuffer := make(chan []order.Order)
	ordersConfirmed := make(chan []order.Order)
	globalOrderCompleted_ch := make(chan [][]bool)


	// Channels for P2P
	peersTx_enable := make(chan bool)
	peersRx_state_ch := make(chan peers.PeerUpdate)
	peersRx_GlobalOrder_ch := make(chan peers.PeerUpdate)

	// - - - - - - Descend to defined state - - - - - - 
	reachFloor := false
	elevio.SetMotorDirection(elevio.MD_Down)
	for reachFloor != true {
		if elevio.GetFloor() != 1 {
			reachFloor = true
		}
	}
	elevio.SetMotorDirection(elevio.MD_Stop)
	elevatorState := elevator.NewElevator(elevio.GetFloor(), elevio.MD_Stop, elevator.EB_Idle)
	for elevio.GetObstruction() {
		elevio.SetDoorOpenLamp(true)
	}
	elevio.SetDoorOpenLamp(false)

	log.Printf("Elevator %d is now at floor %d! Joining network for service...", cfg.ID, elevatorState.Floor)

	// - - - - - - GoRoutines - - - - - - 
	go peers.Transmitter(cfg.Port, cfg.ID, peersTx_enable)
	go peers.Receiver(cfg.Port, peersRx_state_ch)
	go peers.Receiver(cfg.Port, peersRx_GlobalOrder_ch)

	go elevio.PollButtons(buttonEvent_ch)
	go elevio.PollObstructionSwitch(obstructionEvent_ch)
	go elevio.PollFloorSensor(reachFloorEvent_ch)
	// TODO: Add "fsm" for goroutine with orderAssignment
}