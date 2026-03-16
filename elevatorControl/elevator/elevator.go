package elevator

import (
	"elevatorDriver/elevio"
	"slices"
	"time"
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

// TODO: Consider moving the order struct out into syncOrders, and not have it be part of the elevator.
// TODO: Almost certain this is better, and shouldnt take too long.
type Order struct {
	Placed  bool
	Version uint16
	Unknown bool
	AckList []string
}

type Elevator struct {
	Floor        int
	Direction    MotorDirection
	Behaviour    ElevatorBehaviour
	Requests     [N_FLOORS][N_BUTTONS]Order
	OutOfService bool
}

func NewStartElevator(startFloor int) Elevator {
	return Elevator{
		Floor:     startFloor,
		Direction: D_Stop,
		Behaviour: EB_Idle,
		// Assume all new elevators are version 0,
		// they have no requests, and they know it
	}
}

func PlaceOrder(e Elevator, btnFloor int, btnType Button) Elevator {
	e.Requests[btnFloor][btnType].Placed = true
	e.Requests[btnFloor][btnType].Unknown = false
	return e
}

func ClearOrder(e Elevator, btnFloor int, btnType Button) Elevator {
	e.Requests[btnFloor][btnType].Placed = false
	e.Requests[btnFloor][btnType].Unknown = false
	return e
}

func IncrementOrderVersion(e Elevator, btnFloor int, btnType Button) Elevator {
	if btnType != B_Cab {
		e.Requests[btnFloor][btnType].Version += 1
	}
	return e
}

func ExtractCabOrders(requests [N_FLOORS][N_BUTTONS]Order) [N_FLOORS]Order {
	var cabOrders [N_FLOORS]Order
	for floor := 0; floor < N_FLOORS; floor++ {
		cabOrders[floor] = requests[floor][N_BUTTONS-1]
	}
	return cabOrders
}

func ExtractCabOrderPlacements(requests [N_FLOORS][N_BUTTONS]Order) [N_FLOORS]bool {
	var cabOrderPlacements [N_FLOORS]bool
	for floor := 0; floor < N_FLOORS; floor++ {
		cabOrderPlacements[floor] = requests[floor][N_BUTTONS-1].Placed
	}
	return cabOrderPlacements
}

func ExtractHallOrderPlacements(requests [N_FLOORS][N_BUTTONS]Order) [N_FLOORS][N_BUTTONS - 1]bool {
	var hallOrderPlacements [N_FLOORS][N_BUTTONS - 1]bool
	for floor := 0; floor < N_FLOORS; floor++ {
		for button := 0; button < N_BUTTONS-1; button++ {
			hallOrderPlacements[floor][button] = requests[floor][button].Placed
		}
	}
	return hallOrderPlacements
}

func ExtractOrderPlacementTable(requests [N_FLOORS][N_BUTTONS]Order) [N_FLOORS][N_BUTTONS]bool {
	var placements [N_FLOORS][N_BUTTONS]bool
	for floor := 0; floor < N_FLOORS; floor++ {
		for button := 0; button < N_BUTTONS; button++ {
			placements[floor][button] = requests[floor][button].Placed
		}
	}
	return placements
}

func MergeAckLists(firstAckList []string, secondAckList []string) []string {
	mergeList := []string{}
	for _, ID := range firstAckList {
		mergeList = append(mergeList, ID)
	}
	for _, ID := range secondAckList {
		if !isElementInList(ID, firstAckList) {
			mergeList = append(mergeList, ID)
		}
	}
	return mergeList
}

func isElementInList(elem string, list []string) bool {
	for _, listElem := range list {
		if elem == listElem {
			return true
		}
	}
	return false
}

func ContainSameElements(firstList []string, secondList []string) bool {
	if len(firstList) != len(secondList) {
		return false
	}
	// Must take copies as to not modify the original slices
	firstListCopy := slices.Clone(firstList)
	secondListCopy := slices.Clone(secondList)

	slices.Sort(firstListCopy)
	slices.Sort(secondListCopy)

	return slices.Equal(firstListCopy, secondListCopy)
}

// These functions exist to maintain that all interactions with the psysical world go through the elevator module,
// maintaining good module separation and simplifying module interfaces

func PollFloorSensor(floorEvent chan int) {
	elevio.PollFloorSensor(floorEvent)
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

func SetAllLights(requests [N_FLOORS][N_BUTTONS]bool) {
	for floor := 0; floor < N_FLOORS; floor++ {
		for btn := 0; btn < N_BUTTONS; btn++ {
			RequestButtonLight(floor, Button(btn), requests[floor][btn])
		}
	}
}

func HardwareInit(addr string, numFloors int) int {
	elevio.Init(addr, numFloors)

	allLightsOff := [N_FLOORS][N_BUTTONS]bool{}
	SetAllLights(allLightsOff)
	DoorLight(false)

	SetMotorDirection(D_Down)
	for FloorSensor() == -1 {
	}
	SetMotorDirection(D_Stop)

	return FloorSensor()
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

func PollObstruction(obstructionEvent chan bool) {
	obstrCh := make(chan bool)

	go func() {
		for {
			obstrCh <- elevio.GetObstruction()
			time.Sleep(50 * time.Millisecond)
		}
	}()
}
