package requests

import "elevator_project/elevatorControl/elevator"

type DirectionBehaviourPair struct {
	Direction elevator.MotorDirection
	Behaviour elevator.ElevatorBehaviour
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

func requestsHere(e elevator.Elevator) bool {
	for btn := 0; btn < elevator.N_BUTTONS; btn++ {
		if e.Requests[e.Floor][btn] {
			return true
		}
	}
	return false
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

	default:
		e.Requests[e.Floor][elevator.B_HallUp] = false
		e.Requests[e.Floor][elevator.B_HallDown] = false
	}
	return e
}

func ChooseDirection(e elevator.Elevator) DirectionBehaviourPair {
	switch e.Direction {

	case elevator.D_Up:
		if requestsAbove(e) {
			return DirectionBehaviourPair{elevator.D_Up, elevator.EB_Moving}
		} else if requestsHere(e) {
			return DirectionBehaviourPair{elevator.D_Down, elevator.EB_DoorOpen}
		} else if requestsBelow(e) {
			return DirectionBehaviourPair{elevator.D_Down, elevator.EB_Moving}
		} else {
			return DirectionBehaviourPair{elevator.D_Stop, elevator.EB_Idle}
		}

	case elevator.D_Down:
		if requestsBelow(e) {
			return DirectionBehaviourPair{elevator.D_Down, elevator.EB_Moving}
		} else if requestsHere(e) {
			return DirectionBehaviourPair{elevator.D_Up, elevator.EB_DoorOpen}
		} else if requestsAbove(e) {
			return DirectionBehaviourPair{elevator.D_Up, elevator.EB_Moving}
		} else {
			return DirectionBehaviourPair{elevator.D_Stop, elevator.EB_Idle}
		}

	case elevator.D_Stop:
		if requestsHere(e) {
			return DirectionBehaviourPair{elevator.D_Stop, elevator.EB_DoorOpen}
		} else if requestsAbove(e) {
			return DirectionBehaviourPair{elevator.D_Up, elevator.EB_Moving}
		} else if requestsBelow(e) {
			return DirectionBehaviourPair{elevator.D_Down, elevator.EB_Moving}
		} else {
			return DirectionBehaviourPair{elevator.D_Stop, elevator.EB_Idle}
		}

	default:
		return DirectionBehaviourPair{elevator.D_Stop, elevator.EB_Idle}
	}
}
