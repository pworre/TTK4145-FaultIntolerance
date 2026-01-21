package fsm

import (
	"elevator_project/elevatorControl/elevator"
	"elevator_project/elevatorControl/requests"
	"fmt"
)

func SetAllLights(elevatorChannel chan elevator.Elevator, requestLightUp chan int, doneWithLights chan bool) {
	e := <-elevatorChannel
	switch <-requestLightUp {
	case 1:
		for floor := 0; floor < elevator.N_FLOORS; floor++ {
			for btn := 0; btn < elevator.N_BUTTONS; btn++ {
				elevator.RequestButtonLight(floor, elevator.Button(btn), e.Requests[floor][btn])
			}
		}
		doneWithLights <- true
		return
	case 0:
		doneWithLights <- true
		return
	}
}

func OnFloorArrival(e elevator.Elevator, newFloor int,
	elevatorChannel chan elevator.Elevator,
	stopInactivityTimer chan int,
	requestLightUp chan int,
	resetDoorTimer chan bool) elevator.Elevator {

	fmt.Printf("Arrived at floor %d\n", newFloor)

	elevator.FloorIndicator(newFloor)
	e.Floor = newFloor

	switch e.Behaviour {
	case elevator.EB_Moving:
		if requests.ShouldStop(e) {
			stopInactivityTimer <- 1
			elevator.SetMotorDirection(elevator.D_Stop)
			elevator.DoorLight(true)
			e = requests.ClearAtCurrentFloor(e)
			resetDoorTimer <- true
			elevatorChannel <- e
			requestLightUp <- 0
			e.Behaviour = elevator.EB_DoorOpen
		} else {
			requestLightUp <- 0
		}
	default:
		requestLightUp <- 0
	}

	fmt.Println("Exiting floor arrival function")

	return e
}

