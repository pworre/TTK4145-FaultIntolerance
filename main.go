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
	//"os"
	//"syscall"
)

const PEERS_PORT int = 40131

func main() {

	cfg := config.ParseFlag()

	// - - - - - - ProcessPairs logic - - - - - - -

	worldViewCh := make(chan syncOrders.WorldView, 64)
	takeOverWorldViewCh := make(chan syncOrders.WorldView, 64)
	syncRestoreWorldViewCh := make(chan syncOrders.WorldView, 64)
	becamePrimaryCh := make(chan bool, 64)
	restart := make(chan bool, 64)

	go processPairs.RunProcessPairs(cfg, worldViewCh, takeOverWorldViewCh, becamePrimaryCh, restart)

	if cfg.Backup {
		log.Printf("Starting PASSIVE BACKUP for elevator %s with port %d....", cfg.ID, cfg.Port)

		takeOverWorldView := <-takeOverWorldViewCh
		<-becamePrimaryCh // Block until we get promoted to primary

		log.Printf("Backup for elevator %s taking over as primary", cfg.ID)
		syncRestoreWorldViewCh <- takeOverWorldView

	} else {
		log.Printf("Starting PRIMARY elevator %s with port %d....", cfg.ID, cfg.Port)
	}

	// - - - - - - Initializing - - - - - - -
	log.Printf("Initializing Elevator %s with port %d....", cfg.ID, cfg.Port)
	startFloor := elevator.HardwareInit(fmt.Sprintf("localhost:%d", cfg.Port), elevator.N_FLOORS)

	//for elevator.GetObstruction() {
	//	elevator.SetDoorLight(true)
	//}
	//elevator.SetDoorLight(false)

	log.Printf("Elevator %s is now at floor %d! Joining network for service...", cfg.ID, startFloor)

	// - - - - - - Channels - - - - - - - - -

	// Input message channels for events in finite state machine
	obstructionEvent := make(chan bool)
	buttonEvent := make(chan elevator.ButtonEvent)
	floorEvent := make(chan int)
	doorTimeout := make(chan bool)
	inactivityTimeout := make(chan bool)
	motorStallTimeout := make(chan bool)

	// Output message channels for performing actions on elevator hardware
	setFloorIndicator := make(chan int)
	setLights := make(chan [elevator.N_FLOORS][elevator.N_BUTTONS]bool)
	changeMotorDirection := make(chan elevator.MotorDirection)
	openDoor := make(chan bool)
	closeDoor := make(chan bool)
	keepDoorOpen := make(chan bool)
	stillActive := make(chan bool)

	// Output message channel for performing actions on timer instance
	resetDoorTimer := make(chan bool)
	resetInactivityTimer := make(chan bool)
	startMotorStallTimer := make(chan bool)
	stopMotorStallTimer := make(chan bool)
	resetMotorStallTimer := make(chan bool)
	noMotorStall := make(chan bool)

	// Channels for orders
	assignEvent := make(chan [elevator.N_FLOORS][elevator.N_BUTTONS]bool, 512)

	localRequest := make(chan elevator.ButtonEvent)
	localClearing := make(chan elevator.ButtonEvent)

	localStateChange := make(chan elevator.Elevator, 512)

	// Channels for P2P
	peersTx_enable := make(chan bool)
	peerUpdate := make(chan peers.PeerUpdate, 512)
	allActivePeers := make(chan []string, 1)
	// - - - - - - Deploying network communication and order synchronization - - - - - -
	go peers.Transmitter(PEERS_PORT, cfg.ID, peersTx_enable)
	go peers.Receiver(PEERS_PORT, cfg.ID, peerUpdate)

	go syncOrders.SynchronizationLoop(startFloor, cfg, localStateChange, assignEvent, localRequest, localClearing, peerUpdate, setLights, allActivePeers, inactivityTimeout, restart, stillActive, worldViewCh, syncRestoreWorldViewCh)

	// - - - - - - Deploying hardware sensors and timers  - - - - - - -

	go timer.Timers(resetDoorTimer, resetInactivityTimer, resetMotorStallTimer, stopMotorStallTimer, doorTimeout, inactivityTimeout, motorStallTimeout)
	go elevator.PollButtons(buttonEvent)
	go elevator.PollFloorSensor(floorEvent)
	go elevator.PollObstructionSwitch(obstructionEvent)

	// Local finite state machine transition logic
	go fsm.StateMachineLoop(startFloor, buttonEvent,
		assignEvent, localRequest, localClearing,
		floorEvent, setFloorIndicator, changeMotorDirection,
		openDoor, closeDoor, keepDoorOpen, doorTimeout,
		obstructionEvent, motorStallTimeout,
		startMotorStallTimer, noMotorStall, stillActive,
		localStateChange, allActivePeers)

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

		case <-stillActive:
			resetInactivityTimer <- true

		case <-noMotorStall:
			stopMotorStallTimer <- true

		case <-startMotorStallTimer:
			resetMotorStallTimer <- true

		}
	}
}
