package requests

import "elevatorControl/elevator"


func requestsAbove(e elevator.Elevator) bool {
	for f := e.Floor + 1; f < elevator.N_FLOORS; f++ {
		for btn := 0; btn < elevator.N_BUTTONS; btn++ {
			if e.Requests[f][btn] {
				return true
			}
		}
	}
	return false
}

func requestsBelow(e elevator.Elevator) bool {
	for f := 0; f < e.Floor; f++ {
		for btn := 0; btn < elevator.N_BUTTONS; btn++ {
			if e.Requests[f][btn] {
				return true
			}
		}
	}
	return false
}

func requestsHere(e elevator.Elevator) bool {
	for btn := 0; btn < elevator.N_BUTTONS; btn++ {
		if e.Requests[e.Floor][btn] {
			return true
		}
	}
	return false
}

func ChooseDirection(e elevator.Elevator) (elevator.MotorDirection, elevator.ElevatorBehaviour) {
	switch e.Direction {
	case elevator.D_Up:
		if requestsAbove(e) {
			return elevator.D_Up, elevator.EB_Moving
		} else if requestsHere(e) {
			return elevator.D_Down, elevator.EB_DoorOpen
		} else if requestsBelow(e) {
			return elevator.D_Down, elevator.EB_Moving
		} else {
			return elevator.D_Stop, elevator.EB_Idle
		}
	case elevator.D_Down:
		if requestsBelow(e) {
			return elevator.D_Down, elevator.EB_Moving
		} else if requestsHere(e) {
			return elevator.D_Up, elevator.EB_DoorOpen
		} else if requestsAbove(e) {
			return elevator.D_Up, elevator.EB_Moving
		} else {
			return elevator.D_Stop, elevator.EB_Idle
		}
	case elevator.D_Stop:
		if requestsHere(e) {
			return elevator.D_Stop, elevator.EB_DoorOpen
		} else if requestsAbove(e) {
			return elevator.D_Up, elevator.EB_Moving
		} else if requestsBelow(e) {
			return elevator.D_Down, elevator.EB_Moving
		} else {
			return elevator.D_Stop, elevator.EB_Idle
		}
	default:
		return elevator.D_Stop, elevator.EB_Idle
	}
}

func ShouldStop(e elevator.Elevator) bool {
	switch e.Direction {
	case elevator.D_Down:
		return e.Requests[e.Floor][elevator.B_HallDown] ||
			e.Requests[e.Floor][elevator.B_Cab] ||
			!requestsBelow(e)
	case elevator.D_Up:
		return e.Requests[e.Floor][elevator.B_HallUp] ||
			e.Requests[e.Floor][elevator.B_Cab] ||
			!requestsAbove(e)
	case elevator.D_Stop:
		return true
	default:
		return true
	}
}

func ShouldClearImmediately(e elevator.Elevator, btnFloor int, btnType elevator.Button) bool {
	return e.Floor == btnFloor &&
		((e.Direction == elevator.D_Up && btnType == elevator.B_HallUp) ||
			(e.Direction == elevator.D_Down && btnType == elevator.B_HallDown) ||
			e.Direction == elevator.D_Stop ||
			(btnType == elevator.B_Cab))
}

func ClearAtCurrentFloor(e elevator.Elevator) elevator.Elevator {
	e.Requests[e.Floor][elevator.B_Cab] = false
	switch e.Direction {
	case elevator.D_Up:
		if !requestsAbove(e) && !e.Requests[e.Floor][elevator.B_HallUp] {
			e.Requests[e.Floor][elevator.B_HallDown] = false
		}
		e.Requests[e.Floor][elevator.B_HallUp] = false

	case elevator.D_Down:
		if !requestsBelow(e) && !e.Requests[e.Floor][elevator.B_HallDown] {
			e.Requests[e.Floor][elevator.B_HallUp] = false
		}
		e.Requests[e.Floor][elevator.B_HallDown] = false

	case elevator.D_Stop:
		fallthrough

	default:
		e.Requests[e.Floor][elevator.B_HallUp] = false
		e.Requests[e.Floor][elevator.B_HallDown] = false
	}
	return e
}
