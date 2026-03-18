package fsm

import (
	"elevatorControl/elevator"
	"elevatorControl/requests"
	"fmt"
	"log"
)

// CONTENT: This module contains finite state machine. It contains inputs such as button presses, new order-assignments, arriving floors,
//			when door should close, when obstruction is activated, and motor stalling.

func StateMachineLoop(startFloor int,
	buttonEvent chan elevator.ButtonEvent,
	assignEvent chan [elevator.N_FLOORS][elevator.N_BUTTONS]bool,
	localRequest chan elevator.ButtonEvent,
	localClearing chan elevator.ButtonEvent,
	floorEvent chan int, setFloorIndicator chan int,
	changeMotorDirection chan elevator.MotorDirection,
	openDoor chan bool, closeDoor chan bool,
	keepDoorOpen chan bool, doorTimeout chan bool,
	obstructionEvent chan bool, motorStallTimeout chan bool,
	startMotorStallTimer chan bool,
	noMotorStall chan bool, stillActive chan bool,
	localStateChange chan elevator.Elevator, activePeersChan <-chan []string) {

	elev := elevator.NewStartElevator(startFloor)

	for {
		newState := elev
		select {
		case buttonPressed := <-buttonEvent:
			log.Println("Someone pressed a button")
			newState = OnButtonPress(elev, buttonPressed.Floor, buttonPressed.Button, localRequest, keepDoorOpen, stillActive)

		case newAssignment := <-assignEvent:
			fmt.Println("I got a new assignment!", newAssignment)
			if !newState.OutOfService {
				newState = OnNewAssignment(elev, newAssignment, localClearing, changeMotorDirection, openDoor, keepDoorOpen, startMotorStallTimer, stillActive)
			} else {
				log.Println("OUT OF SERVICE:, will not take assignments:", newAssignment)
			}
			stillActive <- true

		case newFloor := <-floorEvent:
			newState = OnFloorArrival(elev, newFloor, setFloorIndicator, localClearing, changeMotorDirection, openDoor, startMotorStallTimer, noMotorStall, stillActive)

		case <-doorTimeout:
			newState = OnDoorTimeout(elev, localClearing, changeMotorDirection, openDoor, closeDoor, keepDoorOpen, startMotorStallTimer, stillActive)

		case isObstructed := <-obstructionEvent:
			newState.OutOfService = isObstructed
			if isObstructed {
				log.Println("We are obstructed")
				if newState.Behaviour == elevator.EB_Idle {
					openDoor <- true
					newState.Behaviour = elevator.EB_DoorOpen
				} else if newState.Behaviour == elevator.EB_DoorOpen {
					keepDoorOpen <- true
				}
			} else {
				log.Println("Obstruction cleared")
				if newState.Behaviour == elevator.EB_Idle {
					openDoor <- true
					newState.Behaviour = elevator.EB_DoorOpen
				} else if newState.Behaviour == elevator.EB_DoorOpen {
					keepDoorOpen <- true
				}
			}
			stillActive <- true

		case <-motorStallTimeout:
			log.Println("Motorstall timeout, but elevator not moving")
			if elev.Behaviour == elevator.EB_Moving {
				log.Printf("Motor stalling while moving!")
				newState.OutOfService = true
			}
			stillActive <- true

		}

		// Guard so it doesnt fire every single time, even without changes
		if newState != elev {
			localStateChange <- newState
		}
		elev = newState

	}
}

// Event handling functions

func OnButtonPress(currentState elevator.Elevator, btnFloor int, btnType elevator.Button,
	localRequest chan elevator.ButtonEvent, keepDoorOpen chan bool, stillActive chan bool) elevator.Elevator {

	nextState := currentState

	switch nextState.Behaviour {
	case elevator.EB_DoorOpen:
		if requests.ShouldClearImmediately(nextState, btnFloor, btnType) {
			keepDoorOpen <- true
		} else {
			// Add request to worldview
			localRequest <- elevator.ButtonEvent{Floor: btnFloor, Button: btnType}
		}

	default:
		// Add request to worldview
		localRequest <- elevator.ButtonEvent{Floor: btnFloor, Button: btnType}
	}

	stillActive <- true

	return nextState
}

func OnNewAssignment(currentState elevator.Elevator,
	assignment [elevator.N_FLOORS][elevator.N_BUTTONS]bool,
	localClearing chan elevator.ButtonEvent,
	changeMotorDirection chan elevator.MotorDirection,
	openDoor chan bool, keepDoorOpen chan bool, startMotorStallTimer chan bool, stillActive chan bool) elevator.Elevator {

	nextState := currentState
	nextState.Requests = assignment

	switch nextState.Behaviour {
	case elevator.EB_DoorOpen:

		shouldClearUpButton, shouldClearDownButton, shouldClearCabButton := requests.WhichButtonsShouldClear(nextState)

		if shouldClearUpButton {
			keepDoorOpen <- true
			// Clear request from worldview
			localClearing <- elevator.ButtonEvent{Floor: nextState.Floor, Button: elevator.B_HallUp}
		}
		if shouldClearDownButton {
			keepDoorOpen <- true
			// Clear request from worldview
			localClearing <- elevator.ButtonEvent{Floor: nextState.Floor, Button: elevator.B_HallDown}
		}
		if shouldClearCabButton {
			keepDoorOpen <- true
			// Clear request from worldview
			localClearing <- elevator.ButtonEvent{Floor: nextState.Floor, Button: elevator.B_Cab}
		}

	case elevator.EB_Moving:

	case elevator.EB_Idle:

		nextState.Direction, nextState.Behaviour = requests.ChooseDirection(nextState)

		switch nextState.Behaviour {
		case elevator.EB_DoorOpen:
			openDoor <- true

			shouldClearUpButton, shouldClearDownButton, shouldClearCabButton := requests.WhichButtonsShouldClear(nextState)

			if shouldClearUpButton {
				// Clear request from worldview
				localClearing <- elevator.ButtonEvent{Floor: nextState.Floor, Button: elevator.B_HallUp}
			}
			if shouldClearDownButton {
				// Clear request from worldview
				localClearing <- elevator.ButtonEvent{Floor: nextState.Floor, Button: elevator.B_HallDown}
			}
			if shouldClearCabButton {
				// Clear request from worldview
				localClearing <- elevator.ButtonEvent{Floor: nextState.Floor, Button: elevator.B_Cab}
			}

		case elevator.EB_Moving:
			changeMotorDirection <- nextState.Direction
			startMotorStallTimer <- true
			log.Println("Motorstall timer started!")

		case elevator.EB_Idle:
		}

	}

	stillActive <- true

	return nextState
}

func OnFloorArrival(currentState elevator.Elevator,
	newFloor int, setFloorIndicator chan int,
	localClearing chan elevator.ButtonEvent,
	changeMotorDirection chan elevator.MotorDirection,
	openDoor chan bool, startMotorStallTimer chan bool, noMotorStall chan bool, stillActive chan bool) elevator.Elevator {

	nextState := currentState

	noMotorStall <- true
	if !elevator.GetObstruction() {
		nextState.OutOfService = false
	}
	log.Println("Motorstall timer stopped!")

	nextState.Floor = newFloor
	setFloorIndicator <- newFloor

	switch nextState.Behaviour {
	case elevator.EB_Moving:
		if requests.ShouldStop(nextState) {
			changeMotorDirection <- elevator.D_Stop
			openDoor <- true

			shouldClearUpButton, shouldClearDownButton, shouldClearCabButton := requests.WhichButtonsShouldClear(nextState)

			if shouldClearUpButton {
				// Clear request from worldview
				localClearing <- elevator.ButtonEvent{Floor: nextState.Floor, Button: elevator.B_HallUp}
			}
			if shouldClearDownButton {
				// Clear request from worldview
				localClearing <- elevator.ButtonEvent{Floor: nextState.Floor, Button: elevator.B_HallDown}
			}
			if shouldClearCabButton {
				// Clear request from worldview
				localClearing <- elevator.ButtonEvent{Floor: nextState.Floor, Button: elevator.B_Cab}
			}

			nextState.Behaviour = elevator.EB_DoorOpen
		} else {
			if !nextState.OutOfService {
				startMotorStallTimer <- true
				log.Println("Motorstall timer started!")
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
	localClearing chan elevator.ButtonEvent,
	changeMotorDirection chan elevator.MotorDirection,
	openDoor chan bool, closeDoor chan bool, keepDoorOpen chan bool, startMotorStallTimer chan bool, stillActive chan bool) elevator.Elevator {

	nextState := currentState

	switch nextState.Behaviour {
	case elevator.EB_DoorOpen:
		nextState.Direction, nextState.Behaviour = requests.ChooseDirection(nextState)

		switch nextState.Behaviour {
		case elevator.EB_DoorOpen:
			keepDoorOpen <- true

			shouldClearUpButton, shouldClearDownButton, shouldClearCabButton := requests.WhichButtonsShouldClear(nextState)

			if shouldClearUpButton {
				// Clear request from worldview
				localClearing <- elevator.ButtonEvent{Floor: nextState.Floor, Button: elevator.B_HallUp}
			}
			if shouldClearDownButton {
				// Clear request from worldview
				localClearing <- elevator.ButtonEvent{Floor: nextState.Floor, Button: elevator.B_HallDown}
			}
			if shouldClearCabButton {
				// Clear request from worldview
				localClearing <- elevator.ButtonEvent{Floor: nextState.Floor, Button: elevator.B_Cab}
			}

		case elevator.EB_Moving:
			if !nextState.OutOfService {
				closeDoor <- true
				changeMotorDirection <- nextState.Direction
				startMotorStallTimer <- true
				log.Println("Motorstall timer started!")
			}

		case elevator.EB_Idle:
			if !nextState.OutOfService {
				closeDoor <- true
				changeMotorDirection <- nextState.Direction
			}
		}

		if nextState.OutOfService {
			keepDoorOpen <- true
			nextState.Behaviour = elevator.EB_DoorOpen
			nextState.Direction = elevator.D_Stop
		}
	}

	stillActive <- true

	return nextState
}
