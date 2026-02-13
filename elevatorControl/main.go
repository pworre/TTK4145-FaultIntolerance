package main

import (
	"elevatorControl/elevator"
	"elevatorControl/fsm"
	"elevatorControl/timer"
	"fmt"
)

// - - - - - - Overview - - - - - - - - -

// This package contains an implementation for controlling a single elevator as a finite state machine,
// using message passing between separate threads
//
// The hardware polling, timer instance and fsm logic all run on separate threads,
// using message passing from the hardware and the timer to the fsm loop to signal events for triggering a state transition.
//
// Using message passing allows all fsm helper functions to be pure, since they can calculate the state transitions,
// signal the outputted actions on message channels and return the transitioned states to the main fsm loop.
//
// The action outputs are handled in main, which in turn performs the actions by messaging the timer and elevator packages,
// which actually do the work of setting the timers and executing the elevator commands.
//
// Structuring the program this way allows all the behaviour logic to be entirely contained within the fsm package,
// only needing the other packages for interacting with the outside world,
// and thus maintains a clean concept for what a computer program should do

func main() {
	fmt.Println("Starting Elevator....")

	// - - - - - - Channels - - - - - - - - -

	// Input message channels for events in finite state machine
	requestEvent := make(chan elevator.ButtonEvent)
	floorEvent := make(chan int)
	doorTimeout := make(chan bool)

	// Output message channels for performing actions on elevator hardware
	setFloorIndicator := make(chan int)

	setLights := make(chan [elevator.N_FLOORS][elevator.N_BUTTONS]bool)

	changeMotorDirection := make(chan elevator.MotorDirection)

	openDoor := make(chan bool)
	closeDoor := make(chan bool)
	keepDoorOpen := make(chan bool)

	// Output message channels for performing actions on timer instance
	resetDoorTimer := make(chan bool)
	stopInactivityTimer := make(chan bool)

	// - - - - - - Initializing - - - - - - -

	startFloor := elevator.HardwareInit()

	// - - - - - - Deploying - - - - - - -

	go timer.Timers(stopInactivityTimer, resetDoorTimer, doorTimeout)
	go elevator.PollButtons(requestEvent)
	go elevator.PollFloorSensor(floorEvent)

	// Finite state machine transition logic
	go fsm.StateMachineLoop(startFloor, requestEvent,
		floorEvent, setFloorIndicator, doorTimeout, keepDoorOpen, openDoor, closeDoor, setLights, changeMotorDirection)

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
