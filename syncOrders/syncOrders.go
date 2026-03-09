package syncOrders

import (
	"config"
	"elevatorControl/elevator"
	"elevatorControl/hallRequestAssigner"
	"encoding/json"
	"log"
	"networkDriver/bcast"
	"networkDriver/peers"
	"reflect"
	"runtime/debug"
	"slices"
	"syncOrders/order"
	"syncOrders/syncOrderFSM"
	"time"
	//"syncOrders/syncOrderFSM"
)

// ! Personlig brainfart: Hadde vel egt ikke trengt å sende en orderToSyncMap...? Kunne bare sett på ID-en til heisen og tatt hensyn til det deretter...? Eller nei, det går jo ikke........ Men kunne hatt en ordre-ID??? Kanskje like komplisert

// TODO: Fix all channel needs, go through last logic (like ex. hra), go over overall and see if things should be moved, added or restructured. Tie together with orderSyncFSM and start testing
// TODO: Consider what threads should live where. peers.Transmitter is for instance spawned in main, but networkSending is spawned in here

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

type OrderNetworkMsg struct {
	PeerID               string                       `json:"peerID"`
	AllElevatorStates    map[string]elevator.Elevator `json:"elevatorState"`
	OrderToSyncMap       map[string]order.Order       `json:"orderToSyncMap"`
	OrdersConfirmed_HALL []order.Order                `json:"ordersConfirmed_HALL"`
	OrdersConfirmed_CAB  map[string][]order.Order     `json:"ordersConfirmed_CAB"`
}

const debug_sync = true

const TRANSMIT_INTERVAL = 500 * time.Millisecond

const G_BCAST_PORT = 25532

func OrderSync(startFloor int, localStateChange <-chan elevator.Elevator, assignEvent chan<- [elevator.N_FLOORS][elevator.N_BUTTONS]bool, newRequest <-chan elevator.ButtonEvent, servicedRequest <-chan elevator.ButtonEvent, cfg config.Config, peerUpdate <-chan peers.PeerUpdate, setLights chan [elevator.N_FLOORS][elevator.N_BUTTONS]bool) {
	myID := cfg.ID

	// TODO: Not initialize the ordersyncmap with an actual order, since it vill be completed... Leave empty until assigned

	// TODO: Random channels, sort later

	networkDisconnect := make(chan bool)
	clearAllConfirmedOrders := make(chan bool)

	confirmedRequest := make(chan order.Order)
	confirmedDeletion := make(chan order.Order)

	newOrderStateTransition := make(chan map[string]order.Order, 1024)

	newOrderStateReceival := make(chan order.OrderStateMessage, 1024)

	peerUpdateInSyncOrders := make(chan peers.PeerUpdate, 1024)
	peerUpdateInSyncOrdersFSM := make(chan peers.PeerUpdate, 1024)
	peerUpdateInRequestBarrierStateCounter := make(chan peers.PeerUpdate, 1024)
	peerUpdateInDeletionBarrierStateCounter := make(chan peers.PeerUpdate, 1024)
	peerUpdateInUnconfirmedRequestBarrierStateCounter := make(chan peers.PeerUpdate, 1024)
	peerUpdateInUnconfirmedDeletionBarrierStateCounter := make(chan peers.PeerUpdate, 1024)
	//waitForReconnection := make(chan peers.PeerUpdate)

	// TODO: End channels

	networkRx := make(chan []byte, 1024)
	networkTx := make(chan []byte, 1024)

	// Not used yet, but could possibly be used for networkDisconnect instead of current solution, in that case function must be rewritten
	//transmitEnable := make(chan bool)

	updateTransmitMessage := make(chan OrderNetworkMsg, 1024)

	go bcast.Transmitter(G_BCAST_PORT, networkTx)
	go bcast.Receiver(G_BCAST_PORT, networkRx)
	go orderMessageTransmitter(myID, networkTx, updateTransmitMessage)

	// TODO: Check if necessary to make deep copies of the peerUpdates
	go peersUpdateRepeater(peerUpdate, peerUpdateInSyncOrders, peerUpdateInSyncOrdersFSM, peerUpdateInRequestBarrierStateCounter, peerUpdateInDeletionBarrierStateCounter, peerUpdateInUnconfirmedRequestBarrierStateCounter, peerUpdateInUnconfirmedDeletionBarrierStateCounter)

	go syncOrderFSM.StateMachineLoop(myID, newOrderStateTransition, newOrderStateReceival, confirmedRequest, confirmedDeletion, networkDisconnect, clearAllConfirmedOrders, peerUpdateInSyncOrdersFSM, peerUpdateInRequestBarrierStateCounter, peerUpdateInDeletionBarrierStateCounter, peerUpdateInUnconfirmedRequestBarrierStateCounter, peerUpdateInUnconfirmedDeletionBarrierStateCounter)

	// ! VERY IMPORTANT ! When new peer initializes and joins, it should set itself as none and everyone else as unknown
	// ! Is this handled by default, or must we explicitly enforce this?

	orderSyncBuffer := make(chan order.Order, 1024)

	// MAP for syncronization use
	orderToSyncMap := make(map[string]order.Order)

	ordersConfirmed_HALL := make([]order.Order, 0)
	ordersConfirmed_CAB := make(map[string][]order.Order)

	activePeersList := make([]string, 0)

	allElevatorStates := make(map[string]elevator.Elevator)
	allElevatorStates[myID] = elevator.NewStartElevator(startFloor)

	latestLocalElevatorState := elevator.NewStartElevator(startFloor)

	for {
		select {
		case newPeerUpdateShallowCopy := <-peerUpdateInSyncOrders:
			newPeerUpdate := peers.PeerUpdateClone(newPeerUpdateShallowCopy)
			log.Printf("OMG I HAVE A FRIEND!")
			activePeersList = newPeerUpdate.Peers
			// ! Will do for now, but should check if we lose several at a time, because if we lose one by one we can assume we are alone and can still operate, not disconnected ourselves... Maybe
			if len(activePeersList) == 0 {
				networkDisconnect <- true
			}
			for _, str := range activePeersList {
				log.Printf("Peer number: %s", str)
			}

		default:
			select {
			case requestToAdd := <-newRequest:
				//log.Printf("OMG I GOT A REQUEST!!!")
				orderToAdd := newOrder(myID, requestToAdd.Floor, requestToAdd.Button, order.SOS_UNCONFIRMED_REQUEST)
				orderSyncBuffer <- orderToAdd

			case requestToRemove := <-servicedRequest:
				//log.Printf("OMG I DID AN ORDER!!!")
				orderToRemove := newOrder(myID, requestToRemove.Floor, requestToRemove.Button, order.SOS_UNCONFIRMED_DELETION)
				orderSyncBuffer <- orderToRemove

			case orderToSyncMap = <-newOrderStateTransition:
				//log.Println("syncOrders orderToSyncMap after state transition: ", orderToSyncMap)
				// TODO: Try commenting the first if statement
				if !isKeyInMap(myID, orderToSyncMap) {
					orderToSyncMap[myID] = order.NewEmptyOrder(myID)
				} else if orderToSyncMap[myID].OrderState == order.SOS_NONE {
					select {
					case nextLocalOrder := <-orderSyncBuffer:
						orderToSyncMap[myID] = nextLocalOrder
						
						newOrderStateReceival <- order.OrderStateMessage{
							OrderToSyncMap: 	order.MapClone(orderToSyncMap),
							TransmittedPeerID: 	myID,
						}
					default:
					}
				}
				//log.Println("OMG GUYS I GOT A STATE TRANSITION AND WANT TO SEND A MESSAGE!")
				updateTransmitMessage <- newOrderNetworkMsg(myID, allElevatorStates, orderToSyncMap, ordersConfirmed_HALL, ordersConfirmed_CAB)
				//log.Printf("OMG GUYS I JUST SENT A MESSAGE!")

			case orderToAdd := <-confirmedRequest:
				//log.Printf("GUYS THERE IS A CONFIRMED ORDER")
				log.Printf("CONFIRMED ORDER ADDED: %+v\n", orderToAdd)
				if !isAlreadyInConfirmedList(orderToAdd, ordersConfirmed_HALL, ordersConfirmed_CAB[orderToAdd.PeerID]) {
					if isHallOrder(orderToAdd) {
						ordersConfirmed_HALL = append(ordersConfirmed_HALL, orderToAdd)
					}
					if isCabOrder(orderToAdd) {
						ordersConfirmed_CAB[orderToAdd.PeerID] = append(ordersConfirmed_CAB[orderToAdd.PeerID], orderToAdd)
					}

					// ? Think about this internal scope setlights and tx, same below
					log.Println("OMG GUYS I JUST CONFIRMED AN ORDER AND WANT TO SEND A MESSAGE!")
					updateTransmitMessage <- newOrderNetworkMsg(myID, allElevatorStates, orderToSyncMap, ordersConfirmed_HALL, ordersConfirmed_CAB)
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
					newHallList := []order.Order{}

					for _, order := range ordersConfirmed_HALL {
						if !sameFloorAndDirection(order, orderToDelete) {
							newHallList = append(newHallList, order)
						}
					}
					if len(newHallList) == len(ordersConfirmed_HALL) {
						log.Println("Could not pop hallOrder")
					} else {
						wasDeleted = true
					}

					ordersConfirmed_HALL = newHallList
				}

				// ? Is this scope trixing really necessary?
				if wasDeleted {
					log.Println("OMG GUYS I JUST DELETED AN ORDER AND WANT TO SEND A MESSAGE!")
					updateTransmitMessage <- newOrderNetworkMsg(myID, allElevatorStates, orderToSyncMap, ordersConfirmed_HALL, ordersConfirmed_CAB)
					log.Printf("OMG GUYS I JUST SENT A MESSAGE!")

					// Reached Barrier state, we can now safely do side effects
					buttonsToLight := orderListsToRequestArray(ordersConfirmed_HALL, ordersConfirmed_CAB[myID])
					setLights <- buttonsToLight
				}

				/// !TO CHECK OUT! Where does this logic belong?

			// HMMMMMMM
			case <-clearAllConfirmedOrders:
				ordersConfirmed_HALL = make([]order.Order, 0)
				ordersConfirmed_CAB = make(map[string][]order.Order)

			// ! This logic can maybe be moved to syncOrderFSM? Probably not, that mixes responsibilities, but so does keeping it here...
			case newElevatorState := <-localStateChange:
				latestLocalElevatorState = newElevatorState
				allElevatorStates[myID] = newElevatorState
				//log.Println("OMG GUYS THERE WAS A LOCAL STATE CHANGE AND I WANT TO SEND A MESSAGE!")
				updateTransmitMessage <- newOrderNetworkMsg(myID, allElevatorStates, orderToSyncMap, ordersConfirmed_HALL, ordersConfirmed_CAB)
				//log.Printf("OMG GUYS I JUST SENT A MESSAGE!")

			// ! This logic can maybe be moved to syncOrderFSM? Probably not, that mixes responsibilities, but so does keeping it here...
			case msgReceivedBytes := <-networkRx:
				//log.Printf("OMG I JUST RECEIVED A MESSAGE!")
				msgReceived := Decode(msgReceivedBytes)

				// We discard messages from peers that are not yet recognized by the peers module
				if isElementInList(msgReceived.PeerID, activePeersList) {

					// !!!!!!!!!! Can probably comment out!
					// ! This doesnt make sense?? We should never get nil for these fields?
					if msgReceived.OrderToSyncMap == nil {
						log.Println("This is weird??? Try the thing under!")
						msgReceived.OrderToSyncMap = make(map[string]order.Order)
						// Here:
						//for _ , peerID := range activePeersList {
						//	msgReceived.OrderToSyncMap[peerID] = order.NewEmptyOrder(myID) // Maybe UNKNOWN instead????
						//}
						//msgReceived.OrderToSyncMap[myID] = order.NewEmptyOrder(myID)
					}
					if msgReceived.OrdersConfirmed_CAB == nil {
						msgReceived.OrdersConfirmed_CAB = make(map[string][]order.Order)
					}
					if msgReceived.AllElevatorStates == nil {
						msgReceived.AllElevatorStates = make(map[string]elevator.Elevator)
					}
					// !!!!!!!!!!! End comment

					// ! Probably the big mistake!!!!!!!!!
					//orderToSyncMap = order.MapClone(msgReceived.OrderToSyncMap)

					newOrderSyncMap := order.MapClone(msgReceived.OrderToSyncMap)
					newOrderStateReceival <- order.OrderStateMessage{OrderToSyncMap: newOrderSyncMap, TransmittedPeerID: msgReceived.PeerID}

					// Merging elevator state and ensuring our state is newest from ourself known state
					for id, state := range msgReceived.AllElevatorStates {
						allElevatorStates[id] = state
					}
					allElevatorStates[myID] = latestLocalElevatorState
					
					// allElevatorStates = order.MapClone(msgReceived.AllElevatorStates) // Fine I guess? Your own states should match up

					// If an elevator just joins the network, it accepts the first received lists of confirmed orders
					if hasNoOrders(ordersConfirmed_HALL, ordersConfirmed_CAB) {
						if !hasNoOrders(msgReceived.OrdersConfirmed_HALL, msgReceived.OrdersConfirmed_CAB) {
							ordersConfirmed_HALL = slices.Clone(msgReceived.OrdersConfirmed_HALL)
							ordersConfirmed_CAB = order.MapClone(msgReceived.OrdersConfirmed_CAB)
						}
					}
				}

			}

			/*
				// ! This logic can maybe be moved to syncOrderFSM? Probably not, that mixes responsibilities, but so does keeping it here...
				case newPeerUpdateShallowCopy := <-peerUpdateInSyncOrders:
					newPeerUpdate := peers.PeerUpdateClone(newPeerUpdateShallowCopy)
					log.Printf("OMG I HAVE A FRIEND!")
					activePeersList = newPeerUpdate.Peers
					// ! Will do for now, but should check if we lose several at a time, because if we lose one by one we can assume we are alone and can still operate, not disconnected ourselves... Maybe
					if len(activePeersList) == 0 {
						networkDisconnect <- true
					}
					for _, str := range activePeersList {
						log.Printf("Peer number: %s", str)
					}
			*/

		}
		SendConfirmedOrdersToHallAssigner(slices.Clone(ordersConfirmed_HALL), slices.Clone(activePeersList), order.MapClone(allElevatorStates), order.MapClone(ordersConfirmed_CAB), myID, assignEvent)
		log.Printf("HALL: %+v 	CAB: %+v\n", ordersConfirmed_HALL, ordersConfirmed_CAB)
		/// !END CHECKOUT!

	}
}

func peersUpdateRepeater(chIn <-chan peers.PeerUpdate, chOut1 chan peers.PeerUpdate, chOut2 chan peers.PeerUpdate, chOut3 chan peers.PeerUpdate, chOut4 chan peers.PeerUpdate, chOut5 chan peers.PeerUpdate, chOut6 chan peers.PeerUpdate) {
	for update := range chIn {
		chOut1 <- peers.PeerUpdateClone(update)
		chOut2 <- peers.PeerUpdateClone(update)
		chOut3 <- peers.PeerUpdateClone(update)
		chOut4 <- peers.PeerUpdateClone(update)
		chOut5 <- peers.PeerUpdateClone(update)
		chOut6 <- peers.PeerUpdateClone(update)
		log.Println("SUCCESSFUL CLONE!!!")
	}

}

func orderMessageTransmitter(myID string, networkTx chan []byte, updateTransmitMessage chan OrderNetworkMsg) {
	orderTransmitMessage := OrderNetworkMsg{PeerID: myID}
	enable := true

	ticker := time.NewTicker(TRANSMIT_INTERVAL)
	defer ticker.Stop()

	for {
		select {
		case proposedMessage := <-updateTransmitMessage:
			if !reflect.DeepEqual(proposedMessage, orderTransmitMessage) {
				orderTransmitMessage = proposedMessage
				enable = true
			}
		case <-ticker.C:
			enable = true
		}
		if enable {
			networkTx <- Encode(orderTransmitMessage)
			//log.Println("SENDT AVGÅRDE! ", orderTransmitMessage)
			enable = false
		}
	}
}

// Pops a order at a given index  and returns a new list of orders, the popped order,
// and a bool telling if a order was popped or not
func PopOrder(orderList []order.Order, index int) ([]order.Order, order.Order, bool) {
	if len(orderList) == 0 {
		return orderList, order.Order{}, false
	}
	poppedOrder := orderList[index]
	orderList = append(orderList[:index], orderList[index+1:]...)

	return orderList, poppedOrder, true
}

// Pops a single order from a list (if there is one) and returns a new list of orders, the popped order,
// and a bool telling if an order was popped or not
func popOrder(orderList []order.Order, ord order.Order) ([]order.Order, order.Order, bool) {
	if len(orderList) == 0 {
		return orderList, order.Order{}, false
	}
	poppedOrder := order.Order{}
	popIndex := -1
	for index, listOrder := range orderList {
		if ord == listOrder {
			popIndex = index
			poppedOrder = orderList[popIndex]
		}
	}

	// If we couldnt find a matching order
	if popIndex == -1 {
		return orderList, order.Order{}, false
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

func SendConfirmedOrdersToHallAssigner(ordersConfirmed_HALL []order.Order, activePeersList []string, allElevatorStates map[string]elevator.Elevator, ordersConfirmed_CAB map[string][]order.Order, myID string, assignEvent chan<- [elevator.N_FLOORS][elevator.N_BUTTONS]bool) {
	hraInput := hallRequestAssigner.HRAInput{
		HallRequests: make([][2]bool, elevator.N_FLOORS),
		States:       make(map[string]hallRequestAssigner.HRAElevState),
	}
	for _, order := range ordersConfirmed_HALL {
		hraInput.HallRequests[order.OrderFloor][order.OrderType] = true
	}
	// Active IDs pluss myself as input for hallRequestAssigner
	ids := append([]string{}, activePeersList...)
	ids = append(ids, myID)

	for _, peerID := range ids {
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
		for _, cabOrder := range ordersConfirmed_CAB[peerID] {
			cabRequests_hra[cabOrder.OrderFloor] = true
		}

		hraInput.States[peerID] = hallRequestAssigner.HRAElevState{
			Behavior:    elevBehaviour_hra,
			Floor:       allElevatorStates[peerID].Floor,
			Direction:   elevDirection_hra,
			CabRequests: cabRequests_hra,
		}

	}
	newAssignmentMap := hallRequestAssigner.Decode(
		hallRequestAssigner.AssignOrders(
			hallRequestAssigner.Encode(hraInput)))
	newAssignment := newAssignmentMap[myID]

	if debug_sync {
		log.Printf("HRA input for hall request: %+v\n", hraInput.HallRequests)
		log.Printf("HRA states=%+v\n", hraInput.States)
		log.Printf("HRA assignemnt map: %+v\n", newAssignmentMap)
		log.Printf("HRA assignment for %s = %+v\n", myID, newAssignment)
	}

	assignEvent <- newAssignment
}

func isCabOrder(order order.Order) bool {
	return order.OrderType == elevator.B_Cab
}

func isHallOrder(order order.Order) bool {
	return order.OrderType == elevator.B_HallDown || order.OrderType == elevator.B_HallUp
}

func sameFloorAndDirection(firstOrder order.Order, secondOrder order.Order) bool {
	return firstOrder.OrderFloor == secondOrder.OrderFloor && firstOrder.OrderType == secondOrder.OrderType
}

func isAlreadyInConfirmedList(order order.Order, confirmedHallList []order.Order, confirmedCabList []order.Order) bool {
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

func hasNoOrders(hallOrders []order.Order, cabOrders map[string][]order.Order) bool {
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

func orderListsToRequestArray(hallOrders []order.Order, cabOrders []order.Order) [elevator.N_FLOORS][elevator.N_BUTTONS]bool {

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

// !Obsolete!
func newOrder(ID string, floor int, button elevator.Button, orderState order.SyncOrderState) order.Order {
	return order.Order{
		PeerID:     ID,
		OrderFloor: floor,
		OrderType:  button,
		OrderState: orderState,
	}
}

func newOrderNetworkMsg(ID string, elevatorStates map[string]elevator.Elevator, orderMap map[string]order.Order, hallList []order.Order, cabList map[string][]order.Order) OrderNetworkMsg {
	return OrderNetworkMsg{
		PeerID:               ID,
		AllElevatorStates:    order.MapClone(elevatorStates),
		OrderToSyncMap:       order.MapClone(orderMap),
		OrdersConfirmed_HALL: slices.Clone(hallList),
		OrdersConfirmed_CAB:  order.MapClone(cabList),
	}
}

func isElementInList(elem string, list []string) bool {
	for _, listElem := range list {
		if elem == listElem {
			return true
		}
	}
	return false
}

func isKeyInMap[T any](key string, theMap map[string]T) bool {
	_, isInMap := theMap[key]
	return isInMap
}
