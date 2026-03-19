package fsm

// - - - - - - Overview - - - - - - - - -

// This module contains a finite state machine for controlling an elevator.
// It takes in inputs via event-channels in a for/select loop, calculates the state transition,
// and performs outputs via message passing on output channels.

import (
	"elevatorControl/elevator"
	"elevatorControl/requests"
	"log"
)

func StateMachineLoop(startFloor int,
	buttonEvent chan elevator.ButtonEvent,
	assignEvent chan [elevator.N_FLOORS][elevator.N_BUTTONS]bool,
	newRequest chan elevator.ButtonEvent,
	newClearing chan elevator.ButtonEvent,
	floorEvent chan int, setFloorIndicator chan int,
	changeMotorDirection chan elevator.MotorDirection,
	openDoor chan bool, closeDoor chan bool,
	keepDoorOpen chan bool, doorTimeout chan bool,
	obstructionEvent chan bool, motorStallTimeout chan bool,
	inactivityTimeout chan bool, startMotorStallTimer chan bool,
	noMotorStall chan bool, stillActive chan bool,
	restart chan bool, stateChange chan elevator.Elevator) {

	elev := elevator.NewStartElevator(startFloor)

	for {
		newState := elev
		select {
		case buttonPressed := <-buttonEvent:
			newState = OnButtonPress(elev, buttonPressed.Floor, buttonPressed.Button, newRequest, keepDoorOpen, stillActive)

		case newAssignment := <-assignEvent:
			if !newState.OutOfService {
				newState = OnNewAssignment(elev, newAssignment, newClearing, changeMotorDirection, openDoor, keepDoorOpen, startMotorStallTimer, stillActive)
			} else {
				log.Println("OUT OF SERVICE, will not take assignments")
			}
			stillActive <- true

		case newFloor := <-floorEvent:
			newState = OnFloorArrival(elev, newFloor, setFloorIndicator, newClearing, changeMotorDirection, openDoor, startMotorStallTimer, noMotorStall, stillActive)

		case <-doorTimeout:
			newState = OnDoorTimeout(elev, newClearing, changeMotorDirection, openDoor, closeDoor, keepDoorOpen, startMotorStallTimer, stillActive)

		case isObstructed := <-obstructionEvent:
			newState.OutOfService = isObstructed

			if newState.Behaviour == elevator.EB_Idle || newState.Behaviour == elevator.EB_DoorOpen {
				openDoor <- true
				newState.Behaviour = elevator.EB_DoorOpen
			}
			stillActive <- true

		case <-motorStallTimeout:
			if elev.Behaviour == elevator.EB_Moving {
				newState.OutOfService = true
			}
			stillActive <- true

		case <-inactivityTimeout:
			restart <- true

		}

		// Only output statechange if we actually changed state
		if newState != elev {
			stateChange <- newState
		}
		elev = newState

	}
}

// Event handling functions

func OnButtonPress(currentState elevator.Elevator, btnFloor int, btnType elevator.Button,
	newRequest chan elevator.ButtonEvent, keepDoorOpen chan bool, stillActive chan bool) elevator.Elevator {

	nextState := currentState

	switch nextState.Behaviour {
	case elevator.EB_DoorOpen:
		if requests.ShouldClearImmediately(nextState, btnFloor, btnType) {
			keepDoorOpen <- true
		} else {
			newRequest <- elevator.ButtonEvent{Floor: btnFloor, Button: btnType}
		}

	default:
		newRequest <- elevator.ButtonEvent{Floor: btnFloor, Button: btnType}
	}

	stillActive <- true

	return nextState
}

func OnNewAssignment(currentState elevator.Elevator,
	assignment [elevator.N_FLOORS][elevator.N_BUTTONS]bool,
	newClearing chan elevator.ButtonEvent,
	changeMotorDirection chan elevator.MotorDirection,
	openDoor chan bool, keepDoorOpen chan bool, startMotorStallTimer chan bool, stillActive chan bool) elevator.Elevator {

	nextState := currentState
	nextState.Requests = assignment

	switch nextState.Behaviour {
	case elevator.EB_DoorOpen:

		shouldClearUpButton, shouldClearDownButton, shouldClearCabButton := requests.WhichButtonsShouldClear(nextState)

		if shouldClearUpButton {
			keepDoorOpen <- true
			newClearing <- elevator.ButtonEvent{Floor: nextState.Floor, Button: elevator.B_HallUp}
		}
		if shouldClearDownButton {
			keepDoorOpen <- true
			newClearing <- elevator.ButtonEvent{Floor: nextState.Floor, Button: elevator.B_HallDown}
		}
		if shouldClearCabButton {
			keepDoorOpen <- true
			newClearing <- elevator.ButtonEvent{Floor: nextState.Floor, Button: elevator.B_Cab}
		}

	case elevator.EB_Moving:

	case elevator.EB_Idle:

		nextState.Direction, nextState.Behaviour = requests.ChooseDirection(nextState)

		switch nextState.Behaviour {
		case elevator.EB_DoorOpen:
			openDoor <- true

			shouldClearUpButton, shouldClearDownButton, shouldClearCabButton := requests.WhichButtonsShouldClear(nextState)

			if shouldClearUpButton {
				newClearing <- elevator.ButtonEvent{Floor: nextState.Floor, Button: elevator.B_HallUp}
			}
			if shouldClearDownButton {
				newClearing <- elevator.ButtonEvent{Floor: nextState.Floor, Button: elevator.B_HallDown}
			}
			if shouldClearCabButton {
				newClearing <- elevator.ButtonEvent{Floor: nextState.Floor, Button: elevator.B_Cab}
			}

		case elevator.EB_Moving:
			changeMotorDirection <- nextState.Direction
			startMotorStallTimer <- true

		case elevator.EB_Idle:
		}

	}

	stillActive <- true

	return nextState
}

func OnFloorArrival(currentState elevator.Elevator,
	newFloor int, setFloorIndicator chan int,
	newClearing chan elevator.ButtonEvent,
	changeMotorDirection chan elevator.MotorDirection,
	openDoor chan bool, startMotorStallTimer chan bool,
	noMotorStall chan bool, stillActive chan bool) elevator.Elevator {

	nextState := currentState

	noMotorStall <- true
	if !elevator.GetObstruction() {
		// Only claim that we are not out of service if we are also not obstructed
		nextState.OutOfService = false
	}

	nextState.Floor = newFloor
	setFloorIndicator <- newFloor

	switch nextState.Behaviour {
	case elevator.EB_Moving:
		if requests.ShouldStop(nextState) {
			changeMotorDirection <- elevator.D_Stop
			openDoor <- true

			shouldClearUpButton, shouldClearDownButton, shouldClearCabButton := requests.WhichButtonsShouldClear(nextState)

			if shouldClearUpButton {
				newClearing <- elevator.ButtonEvent{Floor: nextState.Floor, Button: elevator.B_HallUp}
			}
			if shouldClearDownButton {
				newClearing <- elevator.ButtonEvent{Floor: nextState.Floor, Button: elevator.B_HallDown}
			}
			if shouldClearCabButton {
				newClearing <- elevator.ButtonEvent{Floor: nextState.Floor, Button: elevator.B_Cab}
			}

			nextState.Behaviour = elevator.EB_DoorOpen

		} else {
			if !nextState.OutOfService {
				// If not stopping, restart motor stall timer
				startMotorStallTimer <- true
			}
		}
	}

	if nextState.OutOfService {
		changeMotorDirection <- elevator.D_Stop
		nextState.Direction = elevator.D_Stop
		openDoor <- true
		nextState.Behaviour = elevator.EB_DoorOpen
	}

	stillActive <- true

	return nextState
}

func OnDoorTimeout(currentState elevator.Elevator,
	newClearing chan elevator.ButtonEvent,
	changeMotorDirection chan elevator.MotorDirection,
	openDoor chan bool, closeDoor chan bool, keepDoorOpen chan bool,
	startMotorStallTimer chan bool, stillActive chan bool) elevator.Elevator {

	nextState := currentState

	switch nextState.Behaviour {
	case elevator.EB_DoorOpen:
		nextState.Direction, nextState.Behaviour = requests.ChooseDirection(nextState)

		switch nextState.Behaviour {
		case elevator.EB_DoorOpen:
			keepDoorOpen <- true
			if !nextState.OutOfService {
				stillActive <- true
			}

			shouldClearUpButton, shouldClearDownButton, shouldClearCabButton := requests.WhichButtonsShouldClear(nextState)

			if shouldClearUpButton {
				newClearing <- elevator.ButtonEvent{Floor: nextState.Floor, Button: elevator.B_HallUp}
			}
			if shouldClearDownButton {
				newClearing <- elevator.ButtonEvent{Floor: nextState.Floor, Button: elevator.B_HallDown}
			}
			if shouldClearCabButton {
				newClearing <- elevator.ButtonEvent{Floor: nextState.Floor, Button: elevator.B_Cab}
			}

		case elevator.EB_Moving:
			if !nextState.OutOfService {
				closeDoor <- true
				changeMotorDirection <- nextState.Direction
				startMotorStallTimer <- true
				stillActive <- true
			}

		case elevator.EB_Idle:
			if !nextState.OutOfService {
				closeDoor <- true
				changeMotorDirection <- nextState.Direction
				stillActive <- true
			}
		}

		if nextState.OutOfService {
			keepDoorOpen <- true
			nextState.Behaviour = elevator.EB_DoorOpen
			nextState.Direction = elevator.D_Stop
		}
	}

	return nextState
}
