package fsm

import (
	"elevatorControl/elevator"
	"elevatorControl/requests"
	"fmt"
	"log"
	"os"
)

// Finite state machine loop

func StateMachineLoop(startFloor int,
	buttonEvent chan elevator.ButtonEvent,
	assignEvent chan [elevator.N_FLOORS][elevator.N_BUTTONS]elevator.Order,
	localRequest chan elevator.ButtonEvent,
	localClearing chan elevator.ButtonEvent,
	floorEvent chan int, setFloorIndicator chan int,
	changeMotorDirection chan elevator.MotorDirection,
	openDoor chan bool, closeDoor chan bool,
	keepDoorOpen chan bool, doorTimeout chan bool,
	obstructionEvent chan bool, motorStallEvent chan bool,
	inactivityTimeout chan bool, stillActive chan bool,
	localStateChange chan elevator.Elevator, activePeersChan <-chan []string) {

	var allPeersStillActive []string

	elev := elevator.NewStartElevator(startFloor)

	for {
		newState := elev
		select {
		case buttonPressed := <-buttonEvent:
			log.Println("Someone pressed a button")
			newState = OnButtonPress(elev, buttonPressed.Floor, buttonPressed.Button, localRequest, keepDoorOpen, stillActive)

		case newAssignment := <-assignEvent:
			fmt.Println("I got a new assignment!", newAssignment)
			newState = OnNewAssignment(elev, newAssignment, localClearing, changeMotorDirection, openDoor, keepDoorOpen, stillActive)

		case newFloor := <-floorEvent:
			newState = OnFloorArrival(elev, newFloor, setFloorIndicator, localClearing, changeMotorDirection, openDoor, stillActive)

		case <-doorTimeout:
			newState = OnDoorTimeout(elev, localClearing, changeMotorDirection, closeDoor, keepDoorOpen, stillActive)

		//Debugging, so for now, no obstruction:
		case isObstructed := <-obstructionEvent:
			if isObstructed {
				log.Println("Obstruction actived")
				elev.OutOfService = true
				localStateChange <- elev

				if elev.Behaviour == elevator.EB_Idle {
					keepDoorOpen <- true
					stillActive <- true
				}
			}
			//elev = OnObstructionEvent(elev, isObstructed, keepDoorOpen)

		case updatedActivePeers := <-activePeersChan: // Timing logic incase of only one peer active
			allPeersStillActive = updatedActivePeers
		case <-inactivityTimeout:

			if len(allPeersStillActive) > 1 {
				os.Exit(2)
			} else {
				stillActive <- true
			}

			stillActive <- true

		case <-motorStallEvent:
			elev.OutOfService = true
			localStateChange <- elev
			stillActive <- true

		}

		// Maybe a guard here so it doesnt fire every single time, even without changes
		if elevator.ExtractCabOrderPlacements(newState.Requests) != elevator.ExtractCabOrderPlacements(elev.Requests) {
			localStateChange <- newState //  Pretty sure this channel should be buffered
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
			stillActive <- true
		} else {
			// Add request to worldview
			localRequest <- elevator.ButtonEvent{Floor: btnFloor, Button: btnType}
			nextState = elevator.PlaceOrder(nextState, btnFloor, btnType)

		}

	default:
		// Add request to worldview
		localRequest <- elevator.ButtonEvent{Floor: btnFloor, Button: btnType}
	}

	return nextState
}

func OnNewAssignment(currentState elevator.Elevator,
	assignment [elevator.N_FLOORS][elevator.N_BUTTONS]elevator.Order,
	localClearing chan elevator.ButtonEvent,
	changeMotorDirection chan elevator.MotorDirection,
	openDoor chan bool, keepDoorOpen chan bool, stillActive chan bool) elevator.Elevator {

	nextState := currentState
	nextState.Requests = assignment
	log.Printf("FSM before assignment: %+v", elevator.ExtractOrderPlacementTable(currentState.Requests))
	log.Printf("FSM got assignment: %+v", elevator.ExtractOrderPlacementTable(assignment))

	switch nextState.Behaviour {
	case elevator.EB_DoorOpen:

		shouldClearUpButton, shouldClearDownButton, shouldClearCabButton := requests.WhichButtonsShouldClear(nextState)

		if shouldClearUpButton {
			keepDoorOpen <- true
			stillActive <- true
			// Clear request from worldview
			localClearing <- elevator.ButtonEvent{Floor: nextState.Floor, Button: elevator.B_HallUp}
		}
		if shouldClearDownButton {
			keepDoorOpen <- true
			stillActive <- true
			// Clear request from worldview
			localClearing <- elevator.ButtonEvent{Floor: nextState.Floor, Button: elevator.B_HallDown}
		}
		if shouldClearCabButton {
			keepDoorOpen <- true
			stillActive <- true
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
				stillActive <- true
				// Clear request from worldview
				localClearing <- elevator.ButtonEvent{Floor: nextState.Floor, Button: elevator.B_HallUp}
			}
			if shouldClearDownButton {
				stillActive <- true
				// Clear request from worldview
				localClearing <- elevator.ButtonEvent{Floor: nextState.Floor, Button: elevator.B_HallDown}
			}
			if shouldClearCabButton {
				stillActive <- true
				// Clear request from worldview
				localClearing <- elevator.ButtonEvent{Floor: nextState.Floor, Button: elevator.B_Cab}
			}

		case elevator.EB_Moving:
			changeMotorDirection <- nextState.Direction

		case elevator.EB_Idle:
		}
	}

	return nextState
}

func OnFloorArrival(currentState elevator.Elevator,
	newFloor int, setFloorIndicator chan int,
	localClearing chan elevator.ButtonEvent,
	changeMotorDirection chan elevator.MotorDirection,
	openDoor chan bool, stillActive chan bool) elevator.Elevator {

	nextState := currentState

	nextState.Floor = newFloor
	setFloorIndicator <- newFloor

	switch nextState.Behaviour {
	case elevator.EB_Moving:
		if requests.ShouldStop(nextState) {
			changeMotorDirection <- elevator.D_Stop
			openDoor <- true

			shouldClearUpButton, shouldClearDownButton, shouldClearCabButton := requests.WhichButtonsShouldClear(nextState)

			if shouldClearUpButton {
				stillActive <- true
				// Clear request from worldview
				localClearing <- elevator.ButtonEvent{Floor: nextState.Floor, Button: elevator.B_HallUp}
			}
			if shouldClearDownButton {
				stillActive <- true
				// Clear request from worldview
				localClearing <- elevator.ButtonEvent{Floor: nextState.Floor, Button: elevator.B_HallDown}
			}
			if shouldClearCabButton {
				stillActive <- true
				// Clear request from worldview
				localClearing <- elevator.ButtonEvent{Floor: nextState.Floor, Button: elevator.B_Cab}
			}

			nextState.Behaviour = elevator.EB_DoorOpen
		}
	}

	return nextState
}

func OnDoorTimeout(currentState elevator.Elevator,
	localClearing chan elevator.ButtonEvent,
	changeMotorDirection chan elevator.MotorDirection,
	closeDoor chan bool, keepDoorOpen chan bool, stillActive chan bool) elevator.Elevator {

	nextState := currentState

	switch nextState.Behaviour {
	case elevator.EB_DoorOpen:
		nextState.Direction, nextState.Behaviour = requests.ChooseDirection(nextState)

		switch nextState.Behaviour {
		case elevator.EB_DoorOpen:
			keepDoorOpen <- true

			shouldClearUpButton, shouldClearDownButton, shouldClearCabButton := requests.WhichButtonsShouldClear(nextState)

			if shouldClearUpButton {
				stillActive <- true
				// Clear request from worldview
				localClearing <- elevator.ButtonEvent{Floor: nextState.Floor, Button: elevator.B_HallUp}
			}
			if shouldClearDownButton {
				stillActive <- true
				// Clear request from worldview
				localClearing <- elevator.ButtonEvent{Floor: nextState.Floor, Button: elevator.B_HallDown}
			}
			if shouldClearCabButton {
				stillActive <- true
				// Clear request from worldview
				localClearing <- elevator.ButtonEvent{Floor: nextState.Floor, Button: elevator.B_Cab}
			}

		case elevator.EB_Moving:
			closeDoor <- true
			changeMotorDirection <- nextState.Direction

		case elevator.EB_Idle:
			closeDoor <- true
			changeMotorDirection <- nextState.Direction
		}
	}

	return nextState
}
