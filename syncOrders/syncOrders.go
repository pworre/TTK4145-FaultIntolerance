package syncOrders

import (
	"config"
	"elevatorControl/elevator"
	"elevatorControl/hallRequestAssigner"
	"encoding/json"
	"log"
	"networkDriver/bcast"
	"networkDriver/peers"
	//"syncOrders/syncOrderFSM"
)

// TODO: Fix all channel needs, go through last logic (like ex. hra), go over overall and see if things should be moved, added or restructured. Tie together with orderSyncFSM and start testing

// ! NB: Massive changes ongoing, this comment overview may no longer be valid
/*
This file contains all struct and functions for order syncronization between peers on the network.
Each node is sending a OrderToSyncMap which is a map of what each node's version of the different
nodes and their order to be synced. After a order has been synced, either confirmed_request or ready_to_delete,
it is added/removed from the confirmed hall-/cab-order list. Every node has a local list for confirmed hall-orders,
and a map for cab-orders where the key is the peer-id.

The struct of the OrderToSyncMap is:
{
        "Peer1": Order{ID:"Peer1", OrderType:HALL, OrderFloor:2, State:COS_UNCONFIRMED_REQUEST},
        "Peer2": Order{ID:"Peer2", OrderType:CAB, Floor:3, State:COS_UNCONFIRMED_DELETION},
        "Peer3": Order{ID:"Peer3", OrderType:HALL, Floor:-1, State:COS_NONE},
		.
		.
		.
		"PeerN": Order{ID:"PeerN", OrderType:HALL, Floor:-1, State:COS_NONE},
}

for-select overview:
	- buttonEvent					-	Take button press and add it to orderSyncBuffer
	- reachFloorEvent				-	Check if orders are completed and add it to orderSyncBuffer for deletion
	- orderSyncBuffer				-	Sync orders with other peers on network
	- elevatorState					-	Receives elevator states for use in use for hallRequestAssigner
	- networkRx						-	Receives messages from other peers
	- orderConfirmedBuffer			-	Receives confirmedOrders and adds it to confirmed lists
	- orderDeleteBuffer				-	Receives orders to delete and removed t
	- txMsgUpdate					- 	Adds all variables to the transmitting object and transmits
	- peerUpdate					-	Receives info about all peers on network
*/

type currentOrderState int

const (
	COS_UNKNOWN              = -1
	COS_NONE                 = 0
	COS_UNCONFIRMED_REQUEST  = 1
	COS_CONFIRMED_REQUEST    = 2
	COS_UNCONFIRMED_DELETION = 3
	COS_READY_TO_DELETE      = 4
)

type Order struct {
	PeerID            string
	OrderFloor        int
	OrderType         elevator.Button
	CurrentOrderState currentOrderState
}

type OrderNetworkMsg struct {
	PeerID               string                       `json:"peerID"`
	AllElevatorStates    map[string]elevator.Elevator `json:"elevatorState"`
	OrderToSyncMap       map[string]Order             `json:"orderToSyncMap"`
	OrdersConfirmed_HALL []Order                      `json:"ordersConfirmed_HALL"`
	OrdersConfirmed_CAB  map[string][]Order           `json:"ordersConfirmed_CAB"`
}

const G_BCAST_PORT = 25532

func OrderSync(startFloor int, elevatorState <-chan elevator.Elevator, assignEvent chan<- [elevator.N_FLOORS][elevator.N_BUTTONS]bool, requestEvent <-chan elevator.ButtonEvent, reachFloorEvent <-chan elevator.FloorDirectionPair, cfg config.Config, peerUpdate <-chan peers.PeerUpdate, setLights chan [elevator.N_FLOORS][elevator.N_BUTTONS]bool) {
	myID := cfg.ID

	networkRx := make(chan []byte, 1024)
	networkTx := make(chan []byte, 1024)

	go bcast.Transmitter(G_BCAST_PORT, networkTx)
	go bcast.Receiver(G_BCAST_PORT, networkRx)

	//orderToSync := Order{
	//	PeerID:            myID,
	//	OrderFloor:        -1,
	//	OrderType:         elevator.B_Cab,
	//	CurrentOrderState: COS_UNKNOWN,
	//}

	orderSyncBuffer := make(chan Order, 1024)

	// MAP for syncronization use
	orderToSyncMap := make(map[string]Order)

	ordersConfirmed_HALL := make([]Order, 0)
	ordersConfirmed_CAB := make(map[string][]Order)

	activePeersList := make([]string, 0)

	allElevatorStates := make(map[string]elevator.Elevator)
	allElevatorStates[myID] = elevator.NewStartElevator(startFloor)

	for {
		select {
		case requestToAdd := <-newRequest:
			log.Printf("OMG I GOT A REQUEST!!!")
			orderToAdd := newOrder(myID, requestToAdd.Floor, requestToAdd.Button, COS_UNCONFIRMED_REQUEST)
			orderSyncBuffer <- orderToAdd

		case requestToRemove := <-servicedRequest:
			log.Printf("OMG I DID AN ORDER!!!")
			orderToRemove := newOrder(myID, requestToRemove.Floor, requestToRemove.Button, COS_UNCONFIRMED_DELETION)
			orderSyncBuffer <- orderToRemove

		case orderToSyncMap = <-youCanTransmitNow:
			if orderToSyncMap[myID].CurrentOrderState == COS_NONE {
				select {
				case orderToSyncMap[myID] = <-orderSyncBuffer:

				default:
				}

				networkTx <- Encode(newOrderNetworkMsg(myID, allElevatorStates, orderToSyncMap, ordersConfirmed_HALL, ordersConfirmed_CAB))
				log.Printf("OMG GUYS I JUST SENT A MESSAGE!")
			} else {
				networkTx <- Encode(newOrderNetworkMsg(myID, allElevatorStates, orderToSyncMap, ordersConfirmed_HALL, ordersConfirmed_CAB))
				log.Printf("OMG GUYS I JUST SENT A MESSAGE!")
			}

		case orderToAdd := <-confirmedRequest:
			log.Printf("GUYS THERE IS A CONFIRMED ORDER")
			if !isAlreadyInConfirmedList(orderToAdd, ordersConfirmed_HALL, ordersConfirmed_CAB[myID]) {
				if isHallOrder(orderToAdd) {
					ordersConfirmed_HALL = append(ordersConfirmed_HALL, orderToAdd)
				}
				if isCabOrder(orderToAdd) {
					ordersConfirmed_CAB[myID] = append(ordersConfirmed_CAB[myID], orderToAdd)
				}

				// ? Think about this internal scope setlights and tx, same below
				networkTx <- Encode(newOrderNetworkMsg(myID, allElevatorStates, orderToSyncMap, ordersConfirmed_HALL, ordersConfirmed_CAB))
				log.Printf("OMG GUYS I JUST SENT A MESSAGE!")

				// Reached Barrier state, we can now safely do side effects
				buttonsToLight := orderListsToRequestArray(ordersConfirmed_HALL, ordersConfirmed_CAB[myID])
				setLights <- buttonsToLight
			}

		case orderToDelete := <-confirmedDeletion:

			wasDeleted := false

			if isCabOrder(orderToDelete) {

				// For cabOrders we should only pop the order for the elevator the cab belongs to
				newCabList, _, isPopped := popOrder(ordersConfirmed_CAB[orderToDelete.PeerID], orderToDelete)
				if !isPopped {
					log.Println("Could not pop cabOrder")
				} else {
					wasDeleted = true
				}
				ordersConfirmed_CAB[orderToDelete.PeerID] = newCabList

			} else if isHallOrder(orderToDelete) {

				// For hallOrders we assume everyone gets on when the elevator comes,
				// so we remove all orders that have the same floor and buttontype,
				// regardless of which elevator placed the request
				newHallList := []Order{}

				for _, order := range ordersConfirmed_HALL {
					if !sameFloorAndDirection(order, orderToDelete) {
						newHallList = append(newHallList, order)
					}
				}
				if !(len(newHallList) == len(ordersConfirmed_HALL)) {
					log.Println("Could not pop hallOrder")
				} else {
					wasDeleted = true
				}

				ordersConfirmed_HALL = newHallList
			}

			// ? Is this scope trixing really necessary?
			if wasDeleted {
				networkTx <- Encode(newOrderNetworkMsg(myID, allElevatorStates, orderToSyncMap, ordersConfirmed_HALL, ordersConfirmed_CAB))
				log.Printf("OMG GUYS I JUST SENT A MESSAGE!")

				// Reached Barrier state, we can now safely do side effects
				buttonsToLight := orderListsToRequestArray(ordersConfirmed_HALL, ordersConfirmed_CAB[myID])
				setLights <- buttonsToLight
			}

			/// !TO CHECK OUT! Where does this logic belong?

		// HMMMMMMM

		// ! This logic can maybe be moved to syncOrderFSM? Probably not, that mixes responsibilities, but so does keeping it here...
		case newElevatorState := <-localStateChange:
			allElevatorStates[myID] = newElevatorState
			networkTx <- Encode(newOrderNetworkMsg(myID, allElevatorStates, orderToSyncMap, ordersConfirmed_HALL, ordersConfirmed_CAB))
			log.Printf("OMG GUYS I JUST SENT A MESSAGE!")

		// ! This logic can maybe be moved to syncOrderFSM? Probably not, that mixes responsibilities, but so does keeping it here...
		case msgReceivedBytes := <-networkRx:
			log.Printf("OMG I JUST RECEIVED A MESSAGE!")
			msgReceived := Decode(msgReceivedBytes)

			orderToSyncMapMessage <- msgReceived.OrderToSyncMap
			orderToSyncMap = msgReceived.OrderToSyncMap
			allElevatorStates = msgReceived.AllElevatorStates // Fine I guess? Your own states should match up

			// If an elevator just joins the network, it accepts the first received lists of confirmed orders
			if hasNoOrders(ordersConfirmed_HALL, ordersConfirmed_CAB) {
				if !hasNoOrders(msgReceived.OrdersConfirmed_HALL, msgReceived.OrdersConfirmed_CAB) {
					ordersConfirmed_HALL = msgReceived.OrdersConfirmed_HALL
					ordersConfirmed_CAB = msgReceived.OrdersConfirmed_CAB
				}
			}

		// ! This logic can maybe be moved to syncOrderFSM? Probably not, that mixes responsibilities, but so does keeping it here...
		case newPeerUpdate := <-peerUpdate:
			log.Printf("OMG I HAVE A FRIEND!")
			activePeersList = newPeerUpdate.Peers
			for _, str := range activePeersList {
				log.Printf("Peer number: %s", str)
			}
		}
		SendConfirmedOrdersToHallAssigner(ordersConfirmed_HALL, activePeersList, allElevatorStates, ordersConfirmed_CAB, myID, assignEvent)

		/// !END CHECKOUT!

	}
}

// Pops a order at a given index  and returns a new list of orders, the popped order,
// and a bool telling if a order was popped or not
func PopOrder(orderList []Order, index int) ([]Order, Order, bool) {
	if len(orderList) == 0 {
		return orderList, Order{}, false
	}
	poppedOrder := orderList[index]
	orderList = append(orderList[:index], orderList[index+1:]...)

	return orderList, poppedOrder, true
}

// Pops a single order from a list (if there is one) and returns a new list of orders, the popped order,
// and a bool telling if an order was popped or not
func popOrder(orderList []Order, order Order) ([]Order, Order, bool) {
	if len(orderList) == 0 {
		return orderList, Order{}, false
	}
	poppedOrder := Order{}
	popIndex := -1
	for index, listOrder := range orderList {
		if order == listOrder {
			popIndex = index
			poppedOrder = orderList[popIndex]
		}
	}

	// If we couldnt find a matching order
	if popIndex == -1 {
		return orderList, Order{}, false
	}

	orderList = append(orderList[:popIndex], orderList[popIndex+1:]...)
	return orderList, poppedOrder, true
}

func Encode(input OrderNetworkMsg) []byte {

	jsonBytes, err := json.Marshal(input)
	if err != nil {
		log.Println("json.Marshal error: ", err)
	}
	return jsonBytes
}

func Decode(out []byte) OrderNetworkMsg {

	var incomingMsg OrderNetworkMsg
	err := json.Unmarshal(out, &incomingMsg)
	if err != nil {
		log.Println("json.Unmarshal error: ", err)
	}
	return incomingMsg
}

func SendConfirmedOrdersToHallAssigner(ordersConfirmed_HALL []Order, activePeersList []string, allElevatorStates map[string]elevator.Elevator, ordersConfirmed_CAB map[string][]Order, myID string, assignEvent chan<- [elevator.N_FLOORS][elevator.N_BUTTONS]bool) {
	hraInput := hallRequestAssigner.HRAInput{
		HallRequests: make([][2]bool, elevator.N_FLOORS),
		States:       make(map[string]hallRequestAssigner.HRAElevState),
	}
	for _, order := range ordersConfirmed_HALL {
		hraInput.HallRequests[order.OrderFloor][order.OrderType] = true
	}
	for _, peerID := range activePeersList {
		// convert elevator.behaviour [int] to hra.behaviour [string]
		var elevBehaviour_hra string
		switch allElevatorStates[peerID].Behaviour {
		case elevator.EB_Idle:
			elevBehaviour_hra = "idle"
		case elevator.EB_Moving:
			elevBehaviour_hra = "moving"
		case elevator.EB_DoorOpen:
			elevBehaviour_hra = "doorOpen"
		}

		var elevDirection_hra string
		switch allElevatorStates[peerID].Direction {
		case elevator.D_Up:
			elevDirection_hra = "up"
		case elevator.D_Stop:
			elevDirection_hra = "stop"
		case elevator.D_Down:
			elevDirection_hra = "down"
		}

		cabRequests_hra := make([]bool, elevator.N_FLOORS)
		for _, cabOrder := range ordersConfirmed_CAB[myID] {
			cabRequests_hra[cabOrder.OrderFloor] = true
		}

		hraInput.States[peerID] = hallRequestAssigner.HRAElevState{
			Behavior:    elevBehaviour_hra,
			Floor:       allElevatorStates[peerID].Floor,
			Direction:   elevDirection_hra,
			CabRequests: cabRequests_hra,
		}

		newAssignmentMap := hallRequestAssigner.Decode(hallRequestAssigner.AssignOrders(hallRequestAssigner.Encode(hraInput)))
		newAssignment := newAssignmentMap[myID]
		assignEvent <- newAssignment
	}
}

func isCabOrder(order Order) bool {
	return order.OrderType == elevator.B_Cab
}

func isHallOrder(order Order) bool {
	return order.OrderType == elevator.B_HallDown || order.OrderType == elevator.B_HallUp
}

func sameFloorAndDirection(firstOrder Order, secondOrder Order) bool {
	return firstOrder.OrderFloor == secondOrder.OrderFloor && firstOrder.OrderType == secondOrder.OrderType
}

func isAlreadyInConfirmedList(order Order, confirmedHallList []Order, confirmedCabList []Order) bool {
	switch order.OrderType {
	case elevator.B_Cab:
		for _, confirmedOrder := range confirmedCabList {
			if order == confirmedOrder {
				return true
			}
		}

	default:
		for _, confirmedOrder := range confirmedHallList {
			if order == confirmedOrder {
				return true
			}
		}
	}

	return false
}

func hasNoOrders(hallOrders []Order, cabOrders map[string][]Order) bool {
	if !(len(hallOrders) == 0) {
		return false
	}
	for _, cabOrderList := range cabOrders {
		if !(len(cabOrderList) == 0) {
			return false
		}
	}
	return true
}

func orderListsToRequestArray(hallOrders []Order, cabOrders []Order) [elevator.N_FLOORS][elevator.N_BUTTONS]bool {

	requestArray := [elevator.N_FLOORS][elevator.N_BUTTONS]bool{}

	for _, order := range hallOrders {
		requestArray[order.OrderFloor][int(order.OrderType)] = true
	}
	for _, order := range cabOrders {
		requestArray[order.OrderFloor][int(elevator.B_Cab)] = true
	}

	return requestArray
}

func whichButtonsShouldClear(floor int, direction elevator.MotorDirection, requests [elevator.N_FLOORS][elevator.N_BUTTONS]bool) (bool, bool) {

	shouldClearUpButton := false
	shouldClearDownButton := false

	switch direction {
	case elevator.D_Up:
		if !requestsAbove(floor, direction, requests) && !requests[floor][elevator.B_HallUp] {
			shouldClearDownButton = true
		}
		shouldClearUpButton = true

	case elevator.D_Down:
		if !requestsBelow(floor, direction, requests) && !requests[floor][elevator.B_HallDown] {
			shouldClearUpButton = true
		}
		shouldClearDownButton = true

	case elevator.D_Stop:
		shouldClearUpButton = true
		shouldClearDownButton = true
	}

	return shouldClearUpButton, shouldClearDownButton
}

func requestsAbove(floor int, direction elevator.MotorDirection, requests [elevator.N_FLOORS][elevator.N_BUTTONS]bool) bool {
	for f := floor + 1; f < elevator.N_FLOORS; f++ {
		for btn := 0; btn < elevator.N_BUTTONS; btn++ {
			if requests[f][btn] {
				return true
			}
		}
	}
	return false
}

func requestsBelow(floor int, direction elevator.MotorDirection, requests [elevator.N_FLOORS][elevator.N_BUTTONS]bool) bool {
	for f := 0; f < floor; f++ {
		for btn := 0; btn < elevator.N_BUTTONS; btn++ {
			if requests[f][btn] {
				return true
			}
		}
	}
	return false
}

func newOrder(ID string, floor int, button elevator.Button, orderState currentOrderState) Order {
	return Order{
		PeerID:            ID,
		OrderFloor:        floor,
		OrderType:         button,
		CurrentOrderState: orderState,
	}
}

func newOrderNetworkMsg(ID string, elevatorStates map[string]elevator.Elevator, orderMap map[string]Order, hallList []Order, cabList map[string][]Order) OrderNetworkMsg {
	return OrderNetworkMsg{
		PeerID:               ID,
		AllElevatorStates:    elevatorStates,
		OrderToSyncMap:       orderMap,
		OrdersConfirmed_HALL: hallList,
		OrdersConfirmed_CAB:  cabList,
	}
}
