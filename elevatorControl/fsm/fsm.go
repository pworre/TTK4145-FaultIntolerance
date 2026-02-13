package fsm

import (
	"elevatorControl/elevator"
	"elevatorControl/requests"
	"fmt"
)

func EventLoopTransitionLogic(startFloor int, elevatorShouldStop chan bool,
	requestEvent chan elevator.ButtonEvent, floorEvent chan int,
	setFloorIndicator chan int, doorTimeout chan bool,
	keepDoorOpen chan bool, openDoor chan bool, closeDoor chan bool, setLights chan [elevator.N_FLOORS][elevator.N_BUTTONS]bool,
	changeMotorDirection chan elevator.MotorDirection) {

	elevator := elevator.NewStartElevator(startFloor)

	for {
		fmt.Println("fsmLoop")
		select {
		case newRequest := <-requestEvent:
			elevator = OnRequestButtonPress(elevator, newRequest.Floor, newRequest.Button, keepDoorOpen, openDoor, changeMotorDirection, setLights)

		case newFloor := <-floorEvent:
			elevator = OnFloorArrival(elevator, newFloor, setFloorIndicator, setLights, changeMotorDirection, openDoor)

		case <-doorTimeout:
			// ? Maybe add stopDoorTimer
			elevator = OnDoorTimeout(elevator, changeMotorDirection, keepDoorOpen, closeDoor, setLights)
		}
	}
}

func OnFloorArrival(currentState elevator.Elevator, newFloor int, setFloorIndicator chan int, setLights chan [elevator.N_FLOORS][elevator.N_BUTTONS]bool,
	changeMotorDirection chan elevator.MotorDirection, openDoor chan bool) elevator.Elevator {
	// Copy of current state
	nextState := currentState

	// State transformation and action outputs via message passing to main
	nextState.Floor = newFloor
	setFloorIndicator <- newFloor

	switch nextState.Behaviour {
	case elevator.EB_Moving:
		if requests.ShouldStop(nextState) {
			//elevatorShouldStop <- true // ! Change to action!
			changeMotorDirection <- elevator.D_Stop
			nextState.Direction = elevator.D_Stop
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
	changeMotorDirection chan elevator.MotorDirection,
	keepDoorOpen chan bool, closeDoor chan bool, setLights chan [elevator.N_FLOORS][elevator.N_BUTTONS]bool) elevator.Elevator {
	// Copy of current state
	nextState := currentState

	// State transformation and action outputs via message passing to main
	switch nextState.Behaviour {
	case elevator.EB_DoorOpen:
		//nextState = requests.ClearAtCurrentFloor(nextState)
		nextState.Direction, nextState.Behaviour = requests.ChooseDirection(nextState)
		//changeDirectionBehaviour <- requests.DirectionBehaviourPair{nextState.Direction, nextState.Behaviour}
		fmt.Println("Halla!!!")
		switch nextState.Behaviour {
		case elevator.EB_DoorOpen:
			fmt.Println("Kanskje dette skjer for ofte...?")
			keepDoorOpen <- true
			nextState = requests.ClearAtCurrentFloor(nextState)
			setLights <- nextState.Requests
			break
		case elevator.EB_Moving:
			// ! Check these out!!!
			closeDoor <- true
			nextState = requests.ClearAtCurrentFloor(nextState)
			changeMotorDirection <- nextState.Direction
		case elevator.EB_Idle:
			fmt.Println("Jeg tror ikke dette skjer.....")
			closeDoor <- true
			changeMotorDirection <- nextState.Direction
			break
		}

		break
	default:
		break
	}

	// Return transformed state
	return nextState
}

func OnRequestButtonPress(currentState elevator.Elevator, btnFloor int, btnType elevator.Button,
	keepDoorOpen chan bool, openDoor chan bool,
	changeMotorDirection chan elevator.MotorDirection, setLights chan [elevator.N_FLOORS][elevator.N_BUTTONS]bool) elevator.Elevator {
	// Copy of current state
	nextState := currentState

	// State transformation and action outputs via message passing to main
	switch nextState.Behaviour {
	case elevator.EB_DoorOpen:
		if requests.ShouldClearImmediately(nextState, btnFloor, btnType) {
			keepDoorOpen <- true
			//nextState = requests.ClearAtCurrentFloor(nextState)
			//setLights <- nextState.Requests
		} else {
			//addRequestAction <- elevator.ButtonEvent{btnFloor, btnType}
			nextState.Requests[btnFloor][btnType] = true
		}
		break

	case elevator.EB_Moving:
		//addRequestAction <- elevator.ButtonEvent{btnFloor, btnType}
		nextState.Requests[btnFloor][btnType] = true
		break

	case elevator.EB_Idle:
		//addRequestAction <- elevator.ButtonEvent{btnFloor, btnType}
		nextState.Requests[btnFloor][btnType] = true
		nextState.Direction, nextState.Behaviour = requests.ChooseDirection(nextState)
		//changeDirectionBehaviour <- requests.DirectionBehaviourPair{nextState.Direction, nextState.Behaviour}
		switch nextState.Behaviour {
		case elevator.EB_DoorOpen:
			openDoor <- true
			nextState = requests.ClearAtCurrentFloor(nextState)
			break

		case elevator.EB_Moving:
			changeMotorDirection <- nextState.Direction
			break

		case elevator.EB_Idle:
			break
		}
		break
	}

	setLights <- nextState.Requests

	// Return transformed state
	return nextState
}
