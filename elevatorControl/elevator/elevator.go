package elevator

import (
	"elevatorDriver/elevio"
)

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
	Floor     int
	Direction MotorDirection
	Requests  [N_FLOORS][N_BUTTONS]bool
	Behaviour ElevatorBehaviour
}

func NewElevator(floor int, dir MotorDirection, behaviour ElevatorBehaviour) Elevator {
	//net.ResolveUDPAddr()
	//elevio.Init()
	return Elevator{
		Floor:     floor,
		Direction: dir,
		Behaviour: behaviour,
		// Assume all new elevators have no requests
	}
}

func NewUninitializedElevator() Elevator {
	//net.ResolveUDPAddr()
	elevio.Init("localhost:15657", N_FLOORS)
	return Elevator{
		Floor:     -1,
		Direction: D_Stop,
		Behaviour: EB_Idle,
	}
}

func SetAllLights(e Elevator) {
	for floor := 0; floor < N_FLOORS; floor++ {
		for btn := 0; btn < N_BUTTONS; btn++ {
			RequestButtonLight(floor, Button(btn), e.Requests[floor][btn])
		}
	}
}

func FloorSensor() int {
	return elevio.GetFloor()
}
func FloorIndicator(newFloor int) {
	elevio.SetFloorIndicator(newFloor)
}
func SetMotorDirection(dir MotorDirection) {
	elevio.SetMotorDirection(elevio.MotorDirection(dir))
}
func DoorLight(value bool) {
	elevio.SetDoorOpenLamp(value)
}
func RequestButtonLight(floor int, button Button, value bool) {
	elevio.SetButtonLamp(elevio.ButtonType(button), floor, value)
}
