package main

import (
	"elevatorControl/elevator"
	"elevatorDriver/elevio"
	"log"
)

/*
	"networkDriver/peers"
	"elevator_project/config"
	"elevatorControl/elevator"
	"elevatorDriver/elevio"
	"order"
	"fmt"
	"log"
	"flag"
*/

const peersPort int = 34933

func main() {
	mainControl.mainControl()

	// - - - - - - Descend to defined state - - - - - -
	reachFloor := false
	elevio.SetMotorDirection(elevio.MD_Down)
	for reachFloor != true {
		if elevio.GetFloor() != 1 {
			reachFloor = true
		}
	}
	elevio.SetMotorDirection(elevio.MD_Stop)
	elevatorState := elevator.NewElevator(elevio.GetFloor(), elevio.MD_Stop, elevator.EB_Idle)
	for elevio.GetObstruction() {
		elevio.SetDoorOpenLamp(true)
	}
	elevio.SetDoorOpenLamp(false)

	log.Printf("Elevator %d is now at floor %d! Joining network for service...", cfg.ID, elevatorState.Floor)

	// - - - - - - GoRoutines - - - - - -
	go peers.Transmitter(cfg.Port, cfg.ID, peersTx_enable)
	go peers.Receiver(cfg.Port, peersRx_state_ch)
	go peers.Receiver(cfg.Port, peersRx_GlobalOrder_ch)

	go elevator.PollButtons(buttonEvent_ch)
	go elevator.PollObstructionSwitch(obstructionEvent_ch)
	go elevator.PollFloorSensor(reachFloorEvent_ch)
	// TODO: Add "fsm" for goroutine with orderAssignment
}
