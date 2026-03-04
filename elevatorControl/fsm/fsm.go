package fsm

import (
	"elevatorControl/elevator"
	"elevatorControl/requests"
)

// Finite state machine loop

func StateMachineLoop(startFloor int,
	buttonEvent chan elevator.ButtonEvent, floorEvent chan int,
	doorTimeout chan bool, setFloorIndicator chan int,
	setLights chan [elevator.N_FLOORS][elevator.N_BUTTONS]bool,
	assignEvent chan [elevator.N_FLOORS][elevator.N_BUTTONS]bool,
	changeMotorDirection chan elevator.MotorDirection,
	reachFloorEvent chan elevator.FloorDirectionPair,
	requestEvent chan elevator.ButtonEvent,
	openDoor chan bool, closeDoor chan bool, keepDoorOpen chan bool, resetInactivityTimer chan bool) {

	elevator := elevator.NewStartElevator(startFloor)

	for {
		select {
		case buttonPressed := <-buttonEvent:
			elevator = OnRequestButtonPress(elevator, buttonPressed.Floor, buttonPressed.Button, keepDoorOpen, resetInactivityTimer, requestEvent)

		case newAssignment := <-assignEvent:
			elevator = OnNewAssignment(elevator, newAssignment, setLights, changeMotorDirection, reachFloorEvent, openDoor, keepDoorOpen, resetInactivityTimer)

		case newFloor := <-floorEvent:
			elevator = OnFloorArrival(elevator, newFloor, setFloorIndicator, setLights, changeMotorDirection, reachFloorEvent, openDoor, resetInactivityTimer)

		case <-doorTimeout:
			elevator = OnDoorTimeout(elevator, setLights, changeMotorDirection, reachFloorEvent, closeDoor, keepDoorOpen, resetInactivityTimer)
		}
	}
}

// Event handling functions

func OnRequestButtonPress(currentState elevator.Elevator, btnFloor int, btnType elevator.Button, keepDoorOpen chan bool,
							resetInactivityTimer chan bool, requestEvent chan elevator.ButtonEvent) elevator.Elevator {

	// Copy of current state
	nextState := currentState

	// State transformation and action outputs via message passing
	switch nextState.Behaviour {
	case elevator.EB_DoorOpen:
		if requests.ShouldClearImmediately(nextState, btnFloor, btnType) {
			keepDoorOpen <- true
			resetInactivityTimer <- true
		} else {
			requestEvent <- elevator.ButtonEvent{btnFloor, btnType}
		}

	default:
		requestEvent <- elevator.ButtonEvent{btnFloor, btnType}
	}

	// Return transformed state
	return nextState
}

func OnNewAssignment(currentState elevator.Elevator, assignment [elevator.N_FLOORS][elevator.N_BUTTONS]bool,
	setLights chan [elevator.N_FLOORS][elevator.N_BUTTONS]bool,
	changeMotorDirection chan elevator.MotorDirection,
	reachFloorEvent chan elevator.FloorDirectionPair,
	openDoor chan bool, keepDoorOpen chan bool, resetInactivityTimer chan bool) elevator.Elevator {

	// Copy of current state
	nextState := currentState
	nextState.Requests = assignment

	// State transformation and action outputs via message passing
	switch nextState.Behaviour {
	case elevator.EB_Idle:
		nextState.Direction, nextState.Behaviour = requests.ChooseDirection(nextState)

		switch nextState.Behaviour {
		case elevator.EB_DoorOpen:
			openDoor <- true
			reachFloorEvent <- elevator.FloorDirectionPair{nextState.Floor, nextState.Direction}
			//nextState = requests.ClearAtCurrentFloor(nextState)
			resetInactivityTimer <- true

		case elevator.EB_Moving:
			changeMotorDirection <- nextState.Direction

		case elevator.EB_Idle:
		}
	}

	// ! This should be moved to syncOrders, and it should probably take the global orderlist, not the local one
	// TODO: Maybe make a "barrier" case in syncOrders? This could globally ask everyone to add/remove orders, and tell everyone to set lights
	setLights <- nextState.Requests

	// Return transformed state
	return nextState
}

func OnFloorArrival(currentState elevator.Elevator, newFloor int,
	setFloorIndicator chan int,
	setLights chan [elevator.N_FLOORS][elevator.N_BUTTONS]bool,
	changeMotorDirection chan elevator.MotorDirection,
	reachFloorEvent chan elevator.FloorDirectionPair,
	openDoor chan bool, resetInactivityTimer chan bool) elevator.Elevator {

	// Copy of current state
	nextState := currentState

	// State transformation and action outputs via message passing to main
	nextState.Floor = newFloor
	setFloorIndicator <- newFloor

	switch nextState.Behaviour {
	case elevator.EB_Moving:
		if requests.ShouldStop(nextState) {
			changeMotorDirection <- elevator.D_Stop
			openDoor <- true
			reachFloorEvent <- elevator.FloorDirectionPair{nextState.Floor, nextState.Direction}
			//nextState = requests.ClearAtCurrentFloor(nextState)
			resetInactivityTimer <- true
			// ! No setLights!!!
			// TODO: Same fix as above
			setLights <- nextState.Requests
			nextState.Behaviour = elevator.EB_DoorOpen
		}
	}

	// Return transformed state
	return nextState
}

func OnDoorTimeout(currentState elevator.Elevator,
	setLights chan [elevator.N_FLOORS][elevator.N_BUTTONS]bool,
	changeMotorDirection chan elevator.MotorDirection,
	reachFloorEvent chan elevator.FloorDirectionPair,
	closeDoor chan bool, keepDoorOpen chan bool, resetInactivityTimer chan bool) elevator.Elevator {

	// Copy of current state
	nextState := currentState

	// State transformation and action outputs via message passing to main
	switch nextState.Behaviour {
	case elevator.EB_DoorOpen:
		nextState.Direction, nextState.Behaviour = requests.ChooseDirection(nextState)

		switch nextState.Behaviour {
		case elevator.EB_DoorOpen:
			keepDoorOpen <- true
			reachFloorEvent <- elevator.FloorDirectionPair{nextState.Floor, nextState.Direction}
			//nextState = requests.ClearAtCurrentFloor(nextState)
			resetInactivityTimer <- true
			// ! No setLights!!!
			// TODO: Same fix as above
			setLights <- nextState.Requests

		case elevator.EB_Moving:
			closeDoor <- true
			changeMotorDirection <- nextState.Direction

		case elevator.EB_Idle:
			closeDoor <- true
			changeMotorDirection <- nextState.Direction
		}
	}

	// Return transformed state
	return nextState
}
