package main

import (
	"elevatorControl/elevator"
	"elevatorControl/fsm"
	"elevatorControl/timer"
	"elevator_project/config"
	"fmt"
	"log"
	"networkDriver/peers"
	"syncOrders"
)

const PEERS_PORT int = 34933

func main() {

	cfg := config.ParseFlag()

	// - - - - - - Initilizing - - - - - - -
	log.Println("Initializing Elevator %d with port %d....", cfg.ID, cfg.Port)
	startFloor := elevator.HardwareInit(fmt.Sprintf("localhost:%d", cfg.Port), elevator.N_FLOORS)

	for elevator.GetObstruction() {
		elevator.DoorLight(true)
	}
	elevator.DoorLight(false)

	log.Printf("Elevator %d is now at floor %d! Joining network for service...", cfg.ID, startFloor)

	// - - - - - - Channels - - - - - - - - -

	// Input message channels for events in finite state machine
	obstructionEvent := make(chan bool)
	buttonEvent := make(chan elevator.ButtonEvent)
	reachFloorEvent := make(chan int)
	doorTimeout := make(chan bool)

	// Output message channels for performing actions on elevator hardware
	setFloorIndicator := make(chan int)
	setLights := make(chan [elevator.N_FLOORS][elevator.N_BUTTONS]bool)
	changeMotorDirection := make(chan elevator.MotorDirection)
	openDoor := make(chan bool)
	closeDoor := make(chan bool)
	keepDoorOpen := make(chan bool)

	// Output message channel for performing actions on timer instance
	resetDoorTimer := make(chan bool)
	stopInactivityTimer := make(chan bool)

	// Channels for orders
	orderBuffer := make(chan syncOrders.Order)
	//ordersConfirmed := make(chan []syncOrders.Order)
	//globalOrderCompleted_ := make(chan [][]bool)


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
	go syncOrders.OrderSync(orderBuffer, buttonEvent, reachFloorEvent, cfg)
	// - - - - - - Deploying - - - - - - -

	go timer.Timers(stopInactivityTimer, resetDoorTimer, doorTimeout)
	go elevator.PollButtons(buttonEvent)
	go elevator.PollFloorSensor(reachFloorEvent)

	// TODO: change the inut parameters from buttonEvent to something else, currently the elevators accept all button presses
	// Finite state machine transition logic
	go fsm.StateMachineLoop(startFloor,
		buttonEvent, reachFloorEvent,
		doorTimeout, setFloorIndicator,
		setLights, changeMotorDirection,
		openDoor, closeDoor, keepDoorOpen)

	// Finite state machine action handling
	for {
		select {
		case newFloor := <-setFloorIndicator:
			elevator.FloorIndicator(newFloor)

		case requestList := <-setLights:
			elevator.SetAllLights(requestList)

		case dir := <-changeMotorDirection:
			elevator.SetMotorDirection(dir)

		case <-openDoor:
			elevator.DoorLight(true)
			resetDoorTimer <- true

		case <-closeDoor:
			elevator.DoorLight(false)

		case <-keepDoorOpen:
			resetDoorTimer <- true
		}
	}
}
