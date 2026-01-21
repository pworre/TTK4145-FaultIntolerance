package mainControl

import (
	"elevator_project/elevatorControl/elevator"
	"elevator_project/elevatorControl/fsm"
	"elevator_project/elevatorControl/timer"
	"fmt"
)

func main() {
	fmt.Println("Initializing Elevator....")
	// - - - - - - Initilizing - - - - - - -
	// TODO: INIT
	// ? Question
	// ! Be aware
	// - - - - - - Channels - - - - - - - - -

	// ! Make correct channel types, possibly not bool!
	requestEvent := make(chan bool)
	floorEvent := make(chan bool)

	elevatorChannel := make(chan elevator.Elevator)

	requestLightUp := make(chan int)
	doneWithLights := make(chan bool)

	resetDoorTimer := make(chan bool)
	stopInactivityTimer := make(chan bool)

	go timer.Timers(stopInactivityTimer, resetDoorTimer)

	for {
		select {
		case <-requestEvent:
			// TODO: Handle request

		case <-floorEvent:
			go fsm.SetAllLights(elevatorChannel, requestLightUp, doneWithLights) // Does side effects
			fsm.OnFloorArrival(elevator, newFloor, stopInactivityTimer, requestLightUp, resetDoorTimer)
			<-doneWithLights // Block until lights get turned on or we keep moving

		case <-inactivityTimer.C:
			//TODO: Handle timeOut
		}
	}
}
