package main

import (
	"elevatorControl/elevator"
	"elevatorControl/fsm"
	"elevatorControl/timer"
	"fmt"
)

// TODO: INIT
// ? Question
// ! Be aware

func main() {
	fmt.Println("Starting Elevator....")

	// - - - - - - Channels - - - - - - - - -

	// ! Check out if this can be made obsolete using openDoor and changeMotorDirection! That would be cool!
	elevatorShouldStop := make(chan bool)

	// Event channels for finite state machine
	requestEvent := make(chan elevator.ButtonEvent)
	floorEvent := make(chan int)
	doorTimeout := make(chan bool)

	// ! Check out these
	// Message channels for performing actions on timer instance
	resetDoorTimer := make(chan bool)
	stopInactivityTimer := make(chan bool)

	// Message channels for performing actions on elevator instance
	setFloorIndicator := make(chan int)

	//addRequestAction := make(chan elevator.ButtonEvent)
	setLights := make(chan [elevator.N_FLOORS][elevator.N_BUTTONS]bool)

	changeMotorDirection := make(chan elevator.MotorDirection)

	openDoor := make(chan bool)
	closeDoor := make(chan bool)
	keepDoorOpen := make(chan bool)

	//changeDirectionBehaviour := make(chan requests.DirectionBehaviourPair)

	// - - - - - - Initializing - - - - - - -

	startFloor := elevator.HardwareInit()

	// ! Change this
	//thisElevator := elevator.NewUninitializedElevator()
	//fmt.Println("1")
	//if elevator.FloorSensor() == -1 {
	//	elevator.SetMotorDirection(elevator.D_Down)
	//	thisElevator.Direction = elevator.D_Down
	//	thisElevator.Behaviour = elevator.EB_Moving
	//}
	//fmt.Println("2")
	// - - - - - - Deploying - - - - - - -

	go timer.Timers(stopInactivityTimer, resetDoorTimer, doorTimeout)
	go elevator.PollButtons(requestEvent)
	go elevator.PollFloorSensor(floorEvent)

	// Finite state machine transition logic
	go fsm.EventLoopTransitionLogic(startFloor, elevatorShouldStop, requestEvent,
		floorEvent, setFloorIndicator, doorTimeout, keepDoorOpen, openDoor, closeDoor, setLights, changeMotorDirection)

	// Finite state machine action handling
	for {
		fmt.Println("Loop")
		select {
		case newFloor := <-setFloorIndicator:
			elevator.FloorIndicator(newFloor)
			//thisElevator.Floor = newFloor

		//case newRequest := <-addRequestAction:
		//	thisElevator.Requests[newRequest.Floor][newRequest.Button] = true
		//	elevator.SetAllLights(thisElevator)

		case requestList := <-setLights:
			elevator.SetAllLights(requestList)

		//case pair := <-changeDirectionBehaviour:
		//thisElevator.Direction, thisElevator.Behaviour = pair.Direction, pair.Behaviour

		case dir := <-changeMotorDirection:
			elevator.SetMotorDirection(dir)

		case <-openDoor:
			elevator.DoorLight(true)
			resetDoorTimer <- true
			//thisElevator = requests.ClearAtCurrentFloor(thisElevator)

		case <-closeDoor:
			elevator.DoorLight(false)
			//elevator.SetMotorDirection(thisElevator.Direction)

		case <-keepDoorOpen:
			resetDoorTimer <- true
			//thisElevator = requests.ClearAtCurrentFloor(thisElevator)
			//elevator.SetAllLights(thisElevator)

			// ! Check out if this can be made obsolete using openDoor and changeMotorDirection! That would be cool!
			//case <-elevatorShouldStop:
			//	elevator.SetMotorDirection(elevator.D_Stop)
			//	elevator.DoorLight(true)
			//	thisElevator = requests.ClearAtCurrentFloor(thisElevator)
			//	resetDoorTimer <- true
			//	elevator.SetAllLights(thisElevator)
			//	thisElevator.Behaviour = elevator.EB_DoorOpen
		}
	}
}
