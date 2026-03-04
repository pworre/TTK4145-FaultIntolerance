package fsm

import (
	"elevatorControl/elevator"
	"elevatorControl/requests"
	//"networkDriver/peers"
	//"os"
)

// Finite state machine loop

func StateMachineLoop(startFloor int,
	buttonEvent chan elevator.ButtonEvent, floorEvent chan int, obstructionEvent chan bool,
	doorTimeout chan bool, setFloorIndicator chan int, inactivityTimeout chan bool, keepObstructed chan bool, obstructionTimeout chan bool,
	setLights chan [elevator.N_FLOORS][elevator.N_BUTTONS]bool,
	assignEvent chan [elevator.N_FLOORS][elevator.N_BUTTONS]bool,
	changeMotorDirection chan elevator.MotorDirection,
	reachFloorEvent chan elevator.FloorDirectionPair,
	requestEvent chan elevator.ButtonEvent,
	openDoor chan bool, closeDoor chan bool, keepDoorOpen chan bool, stillActive chan bool) { // peersRx_status chan peers.PeerUpdate) {

	thisElevator := elevator.NewStartElevator(startFloor)

	for {
		select {
		case buttonPressed := <-buttonEvent:
			thisElevator = OnRequestButtonPress(thisElevator, buttonPressed.Floor, buttonPressed.Button, keepDoorOpen, stillActive, requestEvent)

		case newAssignment := <-assignEvent:
			thisElevator = OnNewAssignment(thisElevator, newAssignment, setLights, changeMotorDirection, reachFloorEvent, openDoor, keepDoorOpen, stillActive)

		case newFloor := <-floorEvent:
			thisElevator = OnFloorArrival(thisElevator, newFloor, setFloorIndicator, setLights, changeMotorDirection, reachFloorEvent, openDoor, stillActive)

		case <-doorTimeout:
			thisElevator = OnDoorTimeout(thisElevator, setLights, changeMotorDirection, reachFloorEvent, closeDoor, keepDoorOpen, stillActive)
		case <-inactivityTimeout:

			/* Debugging
			if len(peersRx_state) > 1 {
				os.Exit(2)
			} else {
				stillActive <- true
			}
			*/
			stillActive <- true

		case <-obstructionTimeout:
			thisElevator = OnObstructionTimeout(thisElevator, keepObstructed)
		case <-obstructionEvent:
			thisElevator = OnObstructionEvent(thisElevator, keepDoorOpen, keepObstructed)
		}

	}
}

// Event handling functions

func OnRequestButtonPress(currentState elevator.Elevator, btnFloor int, btnType elevator.Button,
	keepDoorOpen chan bool, stillActive chan bool, requestEvent chan elevator.ButtonEvent) elevator.Elevator {

	// Copy of current state
	nextState := currentState

	// State transformation and action outputs via message passing
	switch nextState.Behaviour {
	case elevator.EB_DoorOpen:
		if requests.ShouldClearImmediately(nextState, btnFloor, btnType) {
			keepDoorOpen <- true
			stillActive <- true
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
	openDoor chan bool, keepDoorOpen chan bool, stillActive chan bool) elevator.Elevator {

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
			stillActive <- true

		case elevator.EB_Moving:
			changeMotorDirection <- nextState.Direction

		case elevator.EB_Idle:
		}
	}

	// ! All setLights should now be moved to syncorders
	//setLights <- nextState.Requests

	// Return transformed state
	return nextState
}

func OnFloorArrival(currentState elevator.Elevator, newFloor int,
	setFloorIndicator chan int,
	setLights chan [elevator.N_FLOORS][elevator.N_BUTTONS]bool,
	changeMotorDirection chan elevator.MotorDirection,
	reachFloorEvent chan elevator.FloorDirectionPair,
	openDoor chan bool, stillActive chan bool) elevator.Elevator {

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
			stillActive <- true
			// ! No setLights!!!
			//setLights <- nextState.Requests
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
	closeDoor chan bool, keepDoorOpen chan bool, stillActive chan bool) elevator.Elevator {

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
			stillActive <- true
			// ! No setLights!!!
			//setLights <- nextState.Requests

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

func OnObstructionTimeout(currentState elevator.Elevator,
	keepObstructed chan bool) elevator.Elevator { // peersRx_state chan peers.PeerUpdate) elevator.Elevator {

	nextState := currentState

	switch nextState.Behaviour {

	case elevator.EB_DoorOpen:
		/* Debugging
		if len(peersRx_state) > 1 {
			os.Exit(1)
		} else {
			keepObstructed <- true
		}
		*/
		keepObstructed <- true
	}

	// Return transformed state
	return nextState
}

func OnObstructionEvent(currentState elevator.Elevator,
	keepDoorOpen chan bool, keepObstructed chan bool) elevator.Elevator {
	nextState := currentState

	switch nextState.Behaviour {

	case elevator.EB_DoorOpen:
		keepDoorOpen <- true
		keepObstructed <- true

	}
	return nextState
}
