package fsm

import (
	"elevatorControl/elevator"
	"elevatorControl/requests"

	//"networkDriver/peers"
	//"os"
	"log"
)

// Finite state machine loop

const debug = true

func StateMachineLoop(startFloor int,
	buttonEvent chan elevator.ButtonEvent, floorEvent chan int, obstructionEvent chan bool,
	doorTimeout chan bool, setFloorIndicator chan int, inactivityTimeout chan bool, keepObstructed chan bool, obstructionTimeout chan bool,
	setLights chan [elevator.N_FLOORS][elevator.N_BUTTONS]bool,
	assignEvent chan [elevator.N_FLOORS][elevator.N_BUTTONS]bool,
	changeMotorDirection chan elevator.MotorDirection,
	servicedRequest chan elevator.ButtonEvent,
	newRequest chan elevator.ButtonEvent,
	openDoor chan bool, closeDoor chan bool, keepDoorOpen chan bool,
	stillActive chan bool, localStateChange chan elevator.Elevator) { // peersRx_status chan peers.PeerUpdate) {

	thisElevator := elevator.NewStartElevator(startFloor)
	var newState elevator.Elevator

	for {
		select {
		case buttonPressed := <-buttonEvent:
			//log.Printf("BRO SOMEONE JUST PRESSED A BUTTON")
			newState = OnRequestButtonPress(thisElevator, buttonPressed.Floor, buttonPressed.Button, keepDoorOpen, stillActive, newRequest)

		case newAssignment := <-assignEvent:
			//log.Printf("OMG I JUST GOT AN ORDER!")
			newState = OnNewAssignment(thisElevator, newAssignment, changeMotorDirection, servicedRequest, openDoor, keepDoorOpen, stillActive)

		case newFloor := <-floorEvent:
			log.Printf("SOMEONE LIKES THE FLOOR????")
			newState = OnFloorArrival(thisElevator, newFloor, setFloorIndicator, changeMotorDirection, servicedRequest, openDoor, stillActive)

		case <-doorTimeout:
			log.Printf("WTF DOOR????")
			newState = OnDoorTimeout(thisElevator, setLights, changeMotorDirection, servicedRequest, closeDoor, keepDoorOpen, stillActive)
		case <-inactivityTimeout:
			log.Printf("ARE YOU KIDDING ME????")
			/* Debugging
			if len(peersRx_state) > 1 {
				os.Exit(2)
			} else {
				stillActive <- true
			}
			*/
			stillActive <- true
			// ObstructionTimeout probably unneccesary, only need event, we must wait for obstruction to clear anyway
		case <-obstructionTimeout:
			log.Printf("OBSSSSSSS????")
			newState = OnObstructionTimeout(thisElevator, keepObstructed)
		case isObstructed := <-obstructionEvent:
			log.Printf("!!! OBSTRUCTION EVENT: %v !!!", isObstructed)
			newState = OnObstructionEvent(thisElevator, isObstructed, keepDoorOpen, keepObstructed)
		}

		// Notify elevators on network that we have done something
		if newState != thisElevator {
			localStateChange <- newState
			thisElevator = newState
		}
	}
}

// Event handling functions

func OnRequestButtonPress(currentState elevator.Elevator, btnFloor int, btnType elevator.Button,
	keepDoorOpen chan bool, stillActive chan bool, newRequest chan elevator.ButtonEvent) elevator.Elevator {

	// Copy of current state
	nextState := currentState

	// State transformation and action outputs via message passing
	switch nextState.Behaviour {
	case elevator.EB_DoorOpen:
		if requests.ShouldClearImmediately(nextState, btnFloor, btnType) {
			newRequest <- elevator.ButtonEvent{
				Floor:  btnFloor,
				Button: btnType,
			}
			keepDoorOpen <- true
			stillActive <- true
		} else {
			newRequest <- elevator.ButtonEvent{
				Floor:  btnFloor,
				Button: btnType,
			}
		}

	default:
		newRequest <- elevator.ButtonEvent{
			Floor:  btnFloor,
			Button: btnType,
		}
	}

	// Return transformed state
	return nextState
}

func OnNewAssignment(currentState elevator.Elevator, assignment [elevator.N_FLOORS][elevator.N_BUTTONS]bool,
	changeMotorDirection chan elevator.MotorDirection,
	servicedRequest chan elevator.ButtonEvent,
	openDoor chan bool, keepDoorOpen chan bool, stillActive chan bool) elevator.Elevator {

	// Copy of current state
	nextState := currentState
	nextState.Requests = assignment

	// State transformation and action outputs via message passing
	switch nextState.Behaviour {
	case elevator.EB_DoorOpen:

		shouldClearUpButton, shouldClearDownButton, shouldClearCabButton := requests.WhichButtonsShouldClear(nextState)

		if shouldClearUpButton {
			keepDoorOpen <- true
			stillActive <- true
			servicedRequest <- elevator.ButtonEvent{
				Floor:  nextState.Floor,
				Button: elevator.B_HallUp}
		}
		if shouldClearDownButton {
			keepDoorOpen <- true
			stillActive <- true
			servicedRequest <- elevator.ButtonEvent{
				Floor:  nextState.Floor,
				Button: elevator.B_HallDown}
		}
		if shouldClearCabButton {
			keepDoorOpen <- true
			stillActive <- true
			servicedRequest <- elevator.ButtonEvent{
				Floor:  nextState.Floor,
				Button: elevator.B_Cab}
		}

		nextState = requests.ClearAtCurrentFloor(nextState)

	case elevator.EB_Moving:

	case elevator.EB_Idle:
		nextState.Direction, nextState.Behaviour = requests.ChooseDirection(nextState)

		// ! DEBUG PRINTING
		if debug {
			if nextState.Behaviour == elevator.EB_Moving {
				//log.Println("Elevator should be moving!")
			} else {
				//log.Println("Elevator should not be moving!")
			}
		}

		switch nextState.Behaviour {
		case elevator.EB_DoorOpen:
			openDoor <- true

			shouldClearUpButton, shouldClearDownButton, shouldClearCabButton := requests.WhichButtonsShouldClear(nextState)

			if shouldClearUpButton {
				stillActive <- true
				servicedRequest <- elevator.ButtonEvent{
					Floor:  nextState.Floor,
					Button: elevator.B_HallUp}
			}
			if shouldClearDownButton {
				stillActive <- true
				servicedRequest <- elevator.ButtonEvent{
					Floor:  nextState.Floor,
					Button: elevator.B_HallDown}
			}
			if shouldClearCabButton {
				stillActive <- true
				servicedRequest <- elevator.ButtonEvent{
					Floor:  nextState.Floor,
					Button: elevator.B_Cab}
			}

			nextState = requests.ClearAtCurrentFloor(nextState)

		case elevator.EB_Moving:
			changeMotorDirection <- nextState.Direction

		case elevator.EB_Idle:
		}
	}
	return nextState
}

func OnFloorArrival(currentState elevator.Elevator, newFloor int,
	setFloorIndicator chan int,
	changeMotorDirection chan elevator.MotorDirection,
	servicedRequest chan elevator.ButtonEvent,
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

			shouldClearUpButton, shouldClearDownButton, shouldClearCabButton := requests.WhichButtonsShouldClear(nextState)
			if shouldClearUpButton {
				servicedRequest <- elevator.ButtonEvent{
					Floor:  nextState.Floor,
					Button: elevator.B_HallUp}
			}
			if shouldClearDownButton {
				servicedRequest <- elevator.ButtonEvent{
					Floor:  nextState.Floor,
					Button: elevator.B_HallDown}
			}
			if shouldClearCabButton {
				servicedRequest <- elevator.ButtonEvent{
					Floor:  nextState.Floor,
					Button: elevator.B_Cab}
			}

			nextState = requests.ClearAtCurrentFloor(nextState)
			nextState.Behaviour = elevator.EB_DoorOpen

			stillActive <- true
		}
	}

	// Return transformed state
	return nextState
}

func OnDoorTimeout(currentState elevator.Elevator,
	setLights chan [elevator.N_FLOORS][elevator.N_BUTTONS]bool,
	changeMotorDirection chan elevator.MotorDirection,
	servicedRequest chan elevator.ButtonEvent,
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

			shouldClearUpButton, shouldClearDownButton, shouldClearCabButton := requests.WhichButtonsShouldClear(nextState)
			if shouldClearUpButton {
				servicedRequest <- elevator.ButtonEvent{
					Floor:  nextState.Floor,
					Button: elevator.B_HallUp}
			}
			if shouldClearDownButton {
				servicedRequest <- elevator.ButtonEvent{
					Floor:  nextState.Floor,
					Button: elevator.B_HallDown}
			}
			if shouldClearCabButton {
				servicedRequest <- elevator.ButtonEvent{
					Floor:  nextState.Floor,
					Button: elevator.B_Cab}
			}

			nextState = requests.ClearAtCurrentFloor(nextState)
			stillActive <- true

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

/*
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

		keepObstructed <- true
	}

	// Return transformed state
	return nextState
}*/

// ! revurder !
func OnObstructionTimeout(currentState elevator.Elevator,
	keepObstructed chan bool) elevator.Elevator {
	return currentState
}

func OnObstructionEvent(currentState elevator.Elevator, isObstructed bool,
	keepDoorOpen chan bool, keepObstructed chan bool) elevator.Elevator {
	nextState := currentState

	if nextState.Behaviour == elevator.EB_DoorOpen && isObstructed {
		keepDoorOpen <- true
		keepObstructed <- true
	}

	return nextState
}
