package main

import (
	"config"
	"elevatorControl/elevator"
	"elevatorControl/fsm"
	"elevatorControl/timer"
	"fmt"
	"log"
	"networkDriver/peers"
	"syncOrders"
)

const PEERS_PORT int = 34933

func main() {

	cfg := config.ParseFlag()

	// - - - - - - Initilizing - - - - - - -
	log.Printf("Initializing Elevator %d with port %d....", cfg.ID, cfg.Port)
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
	floorEvent := make(chan int)
	doorTimeout := make(chan bool)

	// Output message channels for performing actions on elevator hardware
	setFloorIndicator := make(chan int)
	setLights := make(chan [elevator.N_FLOORS][elevator.N_BUTTONS]bool)
	changeMotorDirection := make(chan elevator.MotorDirection)
	openDoor := make(chan bool)
	closeDoor := make(chan bool)
	keepDoorOpen := make(chan bool)
	keepObstructed := make(chan bool)
	stillActive := make(chan bool)

	// Output message channel for performing actions on timer instance
	resetDoorTimer := make(chan bool)
	resetInactivityTimer := make(chan bool)
	resetObstructionTimer := make(chan bool)

	inactivityTimeout := make(chan bool)
	obstructionTimeout := make(chan bool)

	// Channels for orders
	assignEvent := make(chan [elevator.N_FLOORS][elevator.N_BUTTONS]bool)
	reachFloorEvent := make(chan elevator.FloorDirectionPair)
	requestEvent := make(chan elevator.ButtonEvent)
	orderSyncBuffer := make(chan syncOrders.Order, 1024)
	elevatorState := make(chan elevator.Elevator)
	//ordersConfirmed := make(chan []syncOrders.Order)
	//globalOrderCompleted_ := make(chan [][]bool)

	// Channels for P2P
	peersTx_enable := make(chan bool)
	peersRx_status := make(chan peers.PeerUpdate, 1024)

	// - - - - - - GoRoutines - - - - - -
	go peers.Transmitter(PEERS_PORT, cfg.ID, peersTx_enable)
	go peers.Receiver(PEERS_PORT, cfg.ID, peersRx_status)

	//go elevator.PollButtons(buttonEvent)
	//go elevator.PollObstruction(obstructionEvent)
	//go elevator.PollFloorSensor(floorEvent)
	// TODO: Add "fsm" for goroutine with orderAssignment
	go syncOrders.OrderSync(orderSyncBuffer, elevatorState, assignEvent, requestEvent, reachFloorEvent, cfg, peersRx_status, setLights)
	// - - - - - - Deploying - - - - - - -

	go timer.Timers(resetObstructionTimer, resetInactivityTimer, resetDoorTimer, doorTimeout, inactivityTimeout, obstructionTimeout)
	go elevator.PollButtons(buttonEvent)
	go elevator.PollFloorSensor(floorEvent)
	go elevator.PollObstruction(obstructionEvent)

	// Finite state machine transition logic
	go fsm.StateMachineLoop(startFloor,
		buttonEvent, floorEvent, obstructionEvent,
		doorTimeout, setFloorIndicator, inactivityTimeout, keepObstructed, obstructionTimeout,
		setLights, assignEvent, changeMotorDirection,
		reachFloorEvent, requestEvent,
		openDoor, closeDoor, keepDoorOpen, stillActive) ///peersRx_status)

	// Finite state machine action handling
	for {
		select {
		case newFloor := <-setFloorIndicator:
			elevator.FloorIndicator(newFloor)

		case requestList := <-setLights:
			log.Printf("SET LIGHTS")
			elevator.SetAllLights(requestList)

		case dir := <-changeMotorDirection:
			log.Printf("CHANGE DIRECTION")
			elevator.SetMotorDirection(dir)

		case <-openDoor:
			log.Printf("OPEN THE DOOR")
			elevator.DoorLight(true)
			resetDoorTimer <- true

		case <-closeDoor:
			log.Printf("CLOSE THE DOOR")
			elevator.DoorLight(false)

		case <-keepDoorOpen:
			resetDoorTimer <- true

		case <-stillActive:
			resetInactivityTimer <- true

		case <-keepObstructed:
			resetObstructionTimer <- true
		}

	}
}
