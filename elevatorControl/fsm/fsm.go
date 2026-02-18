package fsm

import (
	"elevatorControl/elevator"
	"elevatorControl/requests"
)

// Finite state machine loop

func StateMachineLoop(startFloor int,
	requestEvent chan elevator.ButtonEvent, floorEvent chan int,
	doorTimeout chan bool, setFloorIndicator chan int,
	setLights chan [elevator.N_FLOORS][elevator.N_BUTTONS]bool,
	changeMotorDirection chan elevator.MotorDirection,
	openDoor chan bool, closeDoor chan bool, keepDoorOpen chan bool) {

	elevator := elevator.NewStartElevator(startFloor)

	for {
		select {
		case newRequest := <-requestEvent:
			elevator = OnRequestButtonPress(elevator, newRequest.Floor, newRequest.Button, setLights, changeMotorDirection, openDoor, keepDoorOpen)

		case newFloor := <-floorEvent:
			elevator = OnFloorArrival(elevator, newFloor, setFloorIndicator, setLights, changeMotorDirection, openDoor)

		case <-doorTimeout:
			elevator = OnDoorTimeout(elevator, setLights, changeMotorDirection, closeDoor, keepDoorOpen)
		}
	}
}

// Event handling functions

func OnRequestButtonPress(currentState elevator.Elevator, btnFloor int, btnType elevator.Button,
						  setLights chan [elevator.N_FLOORS][elevator.N_BUTTONS]bool,
						  changeMotorDirection chan elevator.MotorDirection,
						  openDoor chan bool, keepDoorOpen chan bool) elevator.Elevator {

	// Copy of current state
	nextState := currentState

	// State transformation and action outputs via message passing to main
	switch nextState.Behaviour {
	case elevator.EB_DoorOpen:
		if requests.ShouldClearImmediately(nextState, btnFloor, btnType) {
			keepDoorOpen <- true
		} else {
			nextState.Requests[btnFloor][btnType] = true
		}

	case elevator.EB_Moving:
		nextState.Requests[btnFloor][btnType] = true

	case elevator.EB_Idle:
		nextState.Requests[btnFloor][btnType] = true
		nextState.Direction, nextState.Behaviour = requests.ChooseDirection(nextState)

		switch nextState.Behaviour {
		case elevator.EB_DoorOpen:
			openDoor <- true
			nextState = requests.ClearAtCurrentFloor(nextState)

		case elevator.EB_Moving:
			changeMotorDirection <- nextState.Direction

		case elevator.EB_Idle:
		}
	}

	setLights <- nextState.Requests

	// Return transformed state
	return nextState
}

func OnFloorArrival(currentState elevator.Elevator, newFloor int,
					setFloorIndicator chan int,
					setLights chan [elevator.N_FLOORS][elevator.N_BUTTONS]bool,
					changeMotorDirection chan elevator.MotorDirection,
					openDoor chan bool) elevator.Elevator {

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
			nextState = requests.ClearAtCurrentFloor(nextState)
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
				   closeDoor chan bool, keepDoorOpen chan bool) elevator.Elevator {

	// Copy of current state
	nextState := currentState

	// State transformation and action outputs via message passing to main
	switch nextState.Behaviour {
	case elevator.EB_DoorOpen:
		nextState.Direction, nextState.Behaviour = requests.ChooseDirection(nextState)

		switch nextState.Behaviour {
		case elevator.EB_DoorOpen:
			keepDoorOpen <- true
			nextState = requests.ClearAtCurrentFloor(nextState)
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
