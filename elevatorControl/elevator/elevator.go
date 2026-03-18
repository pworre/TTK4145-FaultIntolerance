package elevator

import (
	"elevatorDriver/elevio"
)

// CONTENT: This module takes polling from button and elevator functions from elevio
//			and takes it into elevator-struct for use in other modules.

const N_FLOORS = 4
const N_BUTTONS = 3

type MotorDirection int

const (
	D_Down = -1
	D_Stop = 0
	D_Up   = 1
)

type Button int

const (
	B_HallUp   = 0
	B_HallDown = 1
	B_Cab      = 2
)

type ButtonEvent struct {
	Floor  int
	Button Button
}

type ElevatorBehaviour int

const (
	EB_Idle     = 0
	EB_DoorOpen = 1
	EB_Moving   = 2
)

type Elevator struct {
	Floor        int
	Direction    MotorDirection
	Behaviour    ElevatorBehaviour
	Requests     [N_FLOORS][N_BUTTONS]bool
	OutOfService bool
}

func NewStartElevator(startFloor int) Elevator {
	return Elevator{
		Floor:     startFloor,
		Direction: D_Stop,
		Behaviour: EB_Idle,
		// Assume all new elevators have no requests
	}
}

// These functions exist to maintain that all interactions with the psysical world go through the elevator module,
// maintaining good module separation and simplifying module interfaces

func PollFloorSensor(floorEvent chan int) {
	elevio.PollFloorSensor(floorEvent)
}
func PollObstructionSwitch(obstructionEvent chan bool) {
	elevio.PollObstructionSwitch(obstructionEvent)
}
func GetFloor() int {
	return elevio.GetFloor()
}
func GetObstruction() bool {
	return elevio.GetObstruction()
}
func SetFloorIndicator(newFloor int) {
	elevio.SetFloorIndicator(newFloor)
}
func SetMotorDirection(dir MotorDirection) {
	elevio.SetMotorDirection(elevio.MotorDirection(dir))
}
func SetDoorLight(value bool) {
	elevio.SetDoorOpenLamp(value)
}
func setRequestButtonLight(floor int, button Button, value bool) {
	elevio.SetButtonLamp(elevio.ButtonType(button), floor, value)
}

func SetAllLights(requests [N_FLOORS][N_BUTTONS]bool) {
	for floor := 0; floor < N_FLOORS; floor++ {
		for btn := 0; btn < N_BUTTONS; btn++ {
			setRequestButtonLight(floor, Button(btn), requests[floor][btn])
		}
	}
}

func HardwareInit(addr string, numFloors int) int {
	elevio.Init(addr, numFloors)

	allLightsOff := [N_FLOORS][N_BUTTONS]bool{}
	SetAllLights(allLightsOff)
	SetDoorLight(false)

	SetMotorDirection(D_Down)
	for GetFloor() == -1 {
	}
	SetMotorDirection(D_Stop)

	return GetFloor()
}

func PollButtons(buttonEvent chan ButtonEvent) {
	btnEvent := make(chan elevio.ButtonEvent)

	// Passing all elevio ButtonEvents to elevator ButtonEvents
	go elevio.PollButtons(btnEvent)
	for {
		event := <-btnEvent
		buttonEvent <- ButtonEvent{event.Floor, Button(event.Button)}
	}
}
