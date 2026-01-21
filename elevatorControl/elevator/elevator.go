package elevator

import "elevatorDriver/elevio"

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

type ElevatorBehaviour int

const (
	EB_Idle     = 0
	EB_DoorOpen = 1
	EB_Moving   = 2
)

type Elevator struct {
	Floor     int
	Direction MotorDirection
	Requests  [N_FLOORS][N_BUTTONS]int
	Behaviour ElevatorBehaviour
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
