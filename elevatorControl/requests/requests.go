package requests

import "elevator_project/elevatorControl/elevator"

func requestsBelow(e elevator.Elevator) bool {
	for f := 0; f < e.Floor; f++ {
		for btn := 0; btn < elevator.N_BUTTONS; btn++ {
			if e.Requests[f][btn] != 0 {
				return true
			}
		}
	}
	return false
}

func requestsAbove(e elevator.Elevator) bool {
	for f := e.Floor + 1; f < elevator.N_FLOORS; f++ {
		for btn := 0; btn < elevator.N_BUTTONS; btn++ {
			if e.Requests[f][btn] != 0 {
				return true
			}
		}
	}
	return false
}

func ShouldStop(e elevator.Elevator) bool {
	switch e.Direction {
	case elevator.D_Down:
		return (e.Requests[e.Floor][elevator.B_HallDown] != 0) ||
			(e.Requests[e.Floor][elevator.B_Cab] != 0) ||
			!requestsBelow(e)
	case elevator.D_Up:
		return (e.Requests[e.Floor][elevator.B_HallUp] != 0) ||
			(e.Requests[e.Floor][elevator.B_Cab] != 0) ||
			!requestsAbove(e)
	case elevator.D_Stop:
		return true
	default:
		return true
	}
}

func ClearAtCurrentFloor(e elevator.Elevator) elevator.Elevator {
	e.Requests[e.Floor][elevator.B_Cab] = 0
	switch e.Direction {
	case elevator.D_Up:
		if !requestsAbove(e) && !(e.Requests[e.Floor][elevator.B_HallUp] != 0) {
			e.Requests[e.Floor][elevator.B_HallDown] = 0
		}
		e.Requests[e.Floor][elevator.B_HallUp] = 0

	case elevator.D_Down:
		if !requestsBelow(e) && !(e.Requests[e.Floor][elevator.B_HallDown] != 0) {
			e.Requests[e.Floor][elevator.B_HallUp] = 0
		}
		e.Requests[e.Floor][elevator.B_HallDown] = 0

	case elevator.D_Stop:

	default:
		e.Requests[e.Floor][elevator.B_HallUp] = 0
		e.Requests[e.Floor][elevator.B_HallDown] = 0
	}
	return e
}
