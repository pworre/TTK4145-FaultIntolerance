package main

import (
	"elevatorControl/elevator"
	"elevatorControl/fsm"
	"elevatorControl/requests"
	"elevatorControl/timer"
	"fmt"
)

// TODO: INIT
// ? Question
// ! Be aware

func main() {
	fmt.Println("Starting Elevator....")

	// - - - - - - Channels - - - - - - - - -

	// Message channels for performing actions on elevator instance
	newFloorUpdate := make(chan int)

	addRequest := make(chan elevator.ButtonEvent)

	// ! Check out if this can be made obsolete using openDoor and changeMotorDirection! That would be cool!
	elevatorShouldStop := make(chan bool)

	changeDirectionBehaviour := make(chan requests.DirectionBehaviourPair)

	changeMotorDirection := make(chan elevator.MotorDirection)

	openDoor := make(chan bool)
	closeDoor := make(chan bool)
	keepDoorOpen := make(chan bool)

	// Event channels for finite state machine
	requestEvent := make(chan elevator.ButtonEvent)
	floorEvent := make(chan int)
	doorTimeout := make(chan bool)

	// Message channels for performing actions on timer instance
	resetDoorTimer := make(chan bool)
	stopInactivityTimer := make(chan bool)



	// - - - - - - Initializing - - - - - - -

	thisElevator := elevator.NewUninitializedElevator()
	fmt.Println("1")
	if elevator.FloorSensor() == -1 {
		elevator.SetMotorDirection(elevator.D_Down)
		thisElevator.Direction = elevator.D_Down
		thisElevator.Behaviour = elevator.EB_Moving
	}
	fmt.Println("2")
	// - - - - - - Deploying - - - - - - -

	go timer.Timers(stopInactivityTimer, resetDoorTimer, doorTimeout)
	go elevator.PollButtons(requestEvent)
	go elevator.PollFloorSensor(floorEvent)

	// Finite state machine transition logic
	go fsm.EventLoopTransitionLogic(&thisElevator, elevatorShouldStop, requestEvent, floorEvent, newFloorUpdate, doorTimeout, changeDirectionBehaviour, keepDoorOpen, openDoor, closeDoor, addRequest, changeMotorDirection)

	// Finite state machine action handling
	for {
		fmt.Println("Loop")
		select {
		case newFloor := <-newFloorUpdate:
			elevator.FloorIndicator(newFloor)
			thisElevator.Floor = newFloor

		case newRequest := <-addRequest:
			thisElevator.Requests[newRequest.Floor][newRequest.Button] = true
			elevator.SetAllLights(thisElevator)

		case pair := <-changeDirectionBehaviour:
			thisElevator.Direction, thisElevator.Behaviour = pair.Direction, pair.Behaviour

		case dir := <-changeMotorDirection:
			elevator.SetMotorDirection(dir)

		case <-openDoor:
			elevator.DoorLight(true)
			resetDoorTimer <- true
			thisElevator = requests.ClearAtCurrentFloor(thisElevator)

		case <-closeDoor:
			elevator.DoorLight(false)
			elevator.SetMotorDirection(thisElevator.Direction)

		case <-keepDoorOpen:
			resetDoorTimer <- true
			thisElevator = requests.ClearAtCurrentFloor(thisElevator)
			elevator.SetAllLights(thisElevator)

		// ! Check out if this can be made obsolete using openDoor and changeMotorDirection! That would be cool!
		case <-elevatorShouldStop:
			elevator.SetMotorDirection(elevator.D_Stop)
			elevator.DoorLight(true)
			thisElevator = requests.ClearAtCurrentFloor(thisElevator)
			resetDoorTimer <- true
			elevator.SetAllLights(thisElevator)
			thisElevator.Behaviour = elevator.EB_DoorOpen
		}
	}
}
