package fsm

import (
	"elevatorControl/elevator"
	"elevatorControl/requests"
	"fmt"
)

func EventLoopTransitionLogic(elevator *elevator.Elevator, elevatorShouldStop chan bool,
	requestEvent chan elevator.ButtonEvent, floorEvent chan int,
	newFloorUpdate chan int, doorTimeout chan bool,
	changeDirectionBehaviour chan requests.DirectionBehaviourPair,
	keepDoorOpen chan bool, openDoor chan bool, closeDoor chan bool, addRequest chan elevator.ButtonEvent,
	changeMotorDirection chan elevator.MotorDirection) {

	for {
		fmt.Println("fsmLoop")
		select {
		case newRequest := <-requestEvent:
			OnRequestButtonPress(*elevator, newRequest.Floor, newRequest.Button, changeDirectionBehaviour, keepDoorOpen, openDoor, addRequest, changeMotorDirection)

		case newFloor := <-floorEvent:
			newFloorUpdate <- newFloor
			OnFloorArrival(*elevator, elevatorShouldStop)

		case <-doorTimeout:
			// ? Maybe add stopDoorTimer
			OnDoorTimeout(*elevator, changeDirectionBehaviour, keepDoorOpen, closeDoor)
		}
	}
}

func OnFloorArrival(e elevator.Elevator, elevatorShouldStop chan bool) {

	switch e.Behaviour {
	case elevator.EB_Moving:
		if requests.ShouldStop(e) {
			elevatorShouldStop <- true
		}
	}
}

func OnDoorTimeout(e elevator.Elevator, changeDirectionBehaviour chan requests.DirectionBehaviourPair,
	keepDoorOpen chan bool, closeDoor chan bool) {

	switch e.Behaviour {
	case elevator.EB_DoorOpen:
		e.Direction, e.Behaviour = requests.ChooseDirection(e)
		changeDirectionBehaviour <- requests.DirectionBehaviourPair{e.Direction, e.Behaviour}
		fmt.Println("Halla!!!")
		switch e.Behaviour {
		case elevator.EB_DoorOpen:
			fmt.Println("Kanskje dette skjer for ofte...?")
			keepDoorOpen <- true
			break
		case elevator.EB_Moving:
		case elevator.EB_Idle:
			fmt.Println("Jeg tror ikke dette skjer.....")
			closeDoor <- true
			break
		}

		break
	default:
		break
	}
}

func OnRequestButtonPress(e elevator.Elevator, btnFloor int, btnType elevator.Button,
	changeDirectionBehaviour chan requests.DirectionBehaviourPair,
	keepDoorOpen chan bool, openDoor chan bool, addRequest chan elevator.ButtonEvent,
	changeMotorDirection chan elevator.MotorDirection) {

	switch e.Behaviour {
	case elevator.EB_DoorOpen:
		if requests.ShouldClearImmediately(e, btnFloor, btnType) {
			keepDoorOpen <- true
		} else {
			addRequest <- elevator.ButtonEvent{btnFloor, btnType}
		}
		break

	case elevator.EB_Moving:
		addRequest <- elevator.ButtonEvent{btnFloor, btnType}
		break

	case elevator.EB_Idle:
		addRequest <- elevator.ButtonEvent{btnFloor, btnType}

		e.Direction, e.Behaviour = requests.ChooseDirection(e)
		changeDirectionBehaviour <- requests.DirectionBehaviourPair{e.Direction, e.Behaviour}
		switch e.Behaviour {
		case elevator.EB_DoorOpen:
			openDoor <- true
			break

		case elevator.EB_Moving:
			changeMotorDirection <- e.Direction
			break

		case elevator.EB_Idle:
			break
		}
		break
	}
}
