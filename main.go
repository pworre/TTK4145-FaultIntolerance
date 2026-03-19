package main

import (
	"elevatorControl/elevator"
	"elevatorControl/fsm"
	"elevatorControl/timer"
	"elevator_project/config"
	"elevator_project/processPairs"
	"elevator_project/syncOrders"
	"fmt"
	"log"
	"networkDriver/peers"
)

const PEERS_PORT int = 40131

func main() {

	cfg := config.ParseFlag()

	// - - - - - - ProcessPairs logic - - - - - - -

	// Channel for storing state to backup
	backupState := make(chan syncOrders.WorldView, 1)

	// Channels for promoting backup to primary
	restart := make(chan bool, 1)
	takeOver := make(chan syncOrders.WorldView, 1)
	restartState := make(chan syncOrders.WorldView, 1)

	go processPairs.RunProcessPairs(cfg, backupState, restart, takeOver)

	if cfg.Backup {
		log.Printf("Started PASSIVE BACKUP for elevator %s with port %d....", cfg.ID, cfg.Port)

		takeOverState := <-takeOver // Block until we get promoted to primary

		log.Printf("Backup for elevator %s taking over as primary", cfg.ID)
		restartState <- takeOverState
	}

	// - - - - - - Initializing - - - - - - -

	log.Printf("Initializing Elevator %s with port %d....", cfg.ID, cfg.Port)
	startFloor := elevator.HardwareInit(fmt.Sprintf("localhost:%d", cfg.Port), elevator.N_FLOORS)
	log.Printf("Elevator %s is now at floor %d! Joining network for service...", cfg.ID, startFloor)

	// - - - - - - Channels - - - - - - - - -

	// Input message channels for events in finite state machine
	buttonEvent := make(chan elevator.ButtonEvent)
	assignEvent := make(chan [elevator.N_FLOORS][elevator.N_BUTTONS]bool, 64)
	floorEvent := make(chan int)
	doorTimeout := make(chan bool)
	obstructionEvent := make(chan bool)
	motorStallTimeout := make(chan bool)
	inactivityTimeout := make(chan bool)

	// Output message channels for performing actions on elevator hardware
	setFloorIndicator := make(chan int)
	setLights := make(chan [elevator.N_FLOORS][elevator.N_BUTTONS]bool)
	changeMotorDirection := make(chan elevator.MotorDirection)
	openDoor := make(chan bool)
	closeDoor := make(chan bool)
	keepDoorOpen := make(chan bool)

	// Output message channel for performing actions on timer instance
	resetDoorTimer := make(chan bool)
	resetInactivityTimer := make(chan bool)
	resetMotorStallTimer := make(chan bool)
	startMotorStallTimer := make(chan bool)
	stopMotorStallTimer := make(chan bool)
	noMotorStall := make(chan bool)
	stillActive := make(chan bool)

	// Input message channels for synchronization
	localRequest := make(chan elevator.ButtonEvent)
	localClearing := make(chan elevator.ButtonEvent)
	localStateChange := make(chan elevator.Elevator, 64)

	// Channels for maintaining which peers are connected to the network
	peersTxEnable := make(chan bool)
	peerUpdate := make(chan peers.PeerUpdate, 64)

	// - - - - - - Deploying network communication and order synchronization - - - - - -

	go peers.Transmitter(PEERS_PORT, cfg.ID, peersTxEnable)
	go peers.Receiver(PEERS_PORT, cfg.ID, peerUpdate)

	go syncOrders.SynchronizationLoop(cfg.ID, startFloor,
		localRequest, localClearing, localStateChange,
		peerUpdate, restartState, stillActive, backupState,
		assignEvent, setLights)

	// - - - - - - Deploying timers and hardware sensors  - - - - - - -

	go timer.Timers(resetDoorTimer, resetInactivityTimer, resetMotorStallTimer, stopMotorStallTimer, doorTimeout, inactivityTimeout, motorStallTimeout)
	go elevator.PollButtons(buttonEvent)
	go elevator.PollFloorSensor(floorEvent)
	go elevator.PollObstructionSwitch(obstructionEvent)

	// - - - - - - Deploying local finite state machine transition logic and hardware handling - - - - - - -

	go fsm.StateMachineLoop(startFloor, buttonEvent,
		assignEvent, localRequest, localClearing, floorEvent,
		setFloorIndicator, changeMotorDirection, openDoor,
		closeDoor, keepDoorOpen, doorTimeout, obstructionEvent,
		motorStallTimeout, inactivityTimeout, startMotorStallTimer,
		noMotorStall, stillActive, restart, localStateChange)

	// Hardware action handling
	for {
		select {
		case newFloor := <-setFloorIndicator:
			elevator.SetFloorIndicator(newFloor)
			resetMotorStallTimer <- true

		case requestList := <-setLights:
			elevator.SetAllLights(requestList)

		case dir := <-changeMotorDirection:
			elevator.SetMotorDirection(dir)

		case <-openDoor:
			elevator.SetDoorLight(true)
			resetDoorTimer <- true

		case <-closeDoor:
			elevator.SetDoorLight(false)

		case <-keepDoorOpen:
			resetDoorTimer <- true

		case <-startMotorStallTimer:
			resetMotorStallTimer <- true

		case <-noMotorStall:
			stopMotorStallTimer <- true

		case <-stillActive:
			resetInactivityTimer <- true

		}
	}
}
