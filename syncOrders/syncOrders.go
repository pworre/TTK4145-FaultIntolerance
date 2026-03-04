package syncOrders

import (
	"elevatorControl/elevator"
	"networkDriver/bcast"
	"log"
	"encoding/json"
	"config"
	"networkDriver/peers"
)

// ? Peer routing table [1, 2, 3, 4, ..., n] - Makes order of who transmits to who
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
*/

// TODO: Remove all elevio-use (after merged)

type orderType int 
const (
	HALL = 0
	CAB = 1
)

type currentOrderState int
const (
	COS_UNKNOWN = -1
	COS_NONE = 0
	COS_UNCONFIRMED_REQUEST = 1
	COS_CONFIRMED_REQUEST = 2
	COS_UNCONFIRMED_DELETION = 3 
	COS_READY_TO_DELETE = 4
)

type Order struct {
	PeerID				string
	OrderType 			elevator.Button
	OrderFloor			int
	CurrentOrderState 	currentOrderState
}

type OrderNetworkMsg struct {
	PeerID					string					`json:"peerID"`
	OrderToSyncMap			map[string]Order		`json:"orderToSyncMap"`
	OrdersConfirmed_HALL	[]Order					`json:"ordersConfirmed_HALL"`
	OrdersConfirmed_CAB		[]Order					`json:"ordersConfirmed_CAB"`
	StateCounter			uint64					`json:"stateCounter"`
}

// ! MEANT TO BE IMPLEMENTED IN ELEVATOR !
type ReachFloor struct {
	currentFloor			int
	currentDirection		elevator.Button
}
// ! ! ! ! ! ! ! ! ! ! ! ! ! ! ! ! ! ! ! 

const G_bcast_PORT = 25532

func OrderSync(orderSyncBuffer chan Order, buttonEvent <-chan elevator.ButtonEvent, reachFloorEvent <-chan ReachFloor, cfg config.Config, peerUpdate <-chan peers.PeerUpdate) {
	myID := cfg.ID
	
	networkRx := make(chan []byte, 1024)
	networkTx := make(chan []byte, 1024)
	
	go bcast.Transmitter(G_bcast_PORT, networkTx)
	go bcast.Receiver(G_bcast_PORT, networkRx)
	
	orderToSync := Order{
		PeerID:				myID,
		OrderType: 			HALL,
		OrderFloor: 		-1,
		CurrentOrderState: 	COS_NONE,
	}

	// MAP for syncronization use
	orderToSyncMap := make(map[string]Order)
	orderToSyncMap[myID] = orderToSync

	ordersConfirmed_HALL := make([]Order, 0)
	ordersConfirmed_CAB := make(map[string][]Order)

	orderDeleteBuffer := make(chan Order, 1024)
	txMsgUpdate := make(chan bool, 1024)

	activePeersList := make([]string, 0)

	isPeerSynced := make(map[string]bool, 0)
	for _, peerID := range(activePeersList) {
		isPeerSynced[peerID] = false
	}
	
	// This node's transmitting message
	msgTransmitting := OrderNetworkMsg{
		PeerID: 				myID, 
		OrderToSyncMap:			orderToSyncMap,
		OrdersConfirmed_HALL: 	nil,
		OrdersConfirmed_CAB:	nil,
		StateCounter: 			0,
	}


	for {
		select {
		case buttonPressed := <-buttonEvent:
			orderToAdd := Order{
				OrderType: 			buttonPressed.Button,
				OrderFloor: 		buttonPressed.Floor,
				CurrentOrderState: 	COS_UNCONFIRMED_REQUEST,
			}
			orderSyncBuffer <-orderToAdd

		case reachFloor := <-reachFloorEvent:
			currentFloor := reachFloor.currentFloor
			currentDirection := reachFloor.currentDirection

			// CAB ORDERS
			for _, order := range(ordersConfirmed_CAB[myID]) {
				if order.OrderFloor == currentFloor {
					completedOrder := order
					completedOrder.CurrentOrderState = COS_UNCONFIRMED_DELETION
					orderSyncBuffer <-completedOrder
				}
			}
			// HALL ORDERS
			for _, order := range(ordersConfirmed_HALL) {
				if order.OrderFloor == currentFloor && order.OrderType == currentDirection {
					completedOrder := order
					completedOrder.CurrentOrderState = COS_UNCONFIRMED_DELETION
					orderSyncBuffer <-completedOrder
				}
			}

		case orderToHandle := <-orderSyncBuffer:
			orderToSyncMap[myID] = orderToHandle
			txMsgUpdate <- true

		/*
		case networkOrders := <-ordersConfirmed:
			* TODO: Add 'hallassigner' to choose next to do
		*/

		case msgReceivedBytes := <-networkRx:
			msgReceived := Decode(msgReceivedBytes)

			// Save maps if newer state
			if msgReceived.StateCounter > msgTransmitting.StateCounter {
				orderToSyncMap = msgReceived.OrderToSyncMap
				ordersConfirmed_CAB[msgReceived.PeerID] = msgReceived.OrdersConfirmed_CAB
				ordersConfirmed_HALL = msgReceived.OrdersConfirmed_HALL
				msgTransmitting.StateCounter = msgReceived.StateCounter - 1
			}

			// Checks if MY OrderToSync is synced to all peers
			if msgReceived.OrderToSyncMap[myID] != orderToSync{
				isPeerSynced[msgReceived.PeerID] = false
			}
			if msgReceived.OrderToSyncMap[myID] == orderToSync {
				isPeerSynced[msgReceived.PeerID] = true
				isAllPeersSynced := true
				for _, peerID := range(activePeersList) {
					if !isPeerSynced[peerID] {
						isAllPeersSynced = false
					}
				}
				if isAllPeersSynced {
					// Check if unconfirmed: then need to sync it
					if orderToSyncMap[myID].CurrentOrderState == COS_UNCONFIRMED_REQUEST {
						orderToSync.CurrentOrderState = COS_CONFIRMED_REQUEST
						orderSyncBuffer <-orderToSync
					}
					if orderToSync.CurrentOrderState == COS_UNCONFIRMED_DELETION {
						orderToSync.CurrentOrderState = COS_READY_TO_DELETE
						orderDeleteBuffer <-orderToSync
					}

					// Check if confirmed: Then add to confirmed list
					if orderToSync.CurrentOrderState == COS_CONFIRMED_REQUEST {
						if orderToSync.OrderType == HALL {
							ordersConfirmed_HALL = append(ordersConfirmed_HALL, orderToSync)
						}
						if orderToSync.OrderType == CAB {
							ordersConfirmed_CAB[myID] = append(ordersConfirmed_CAB[myID], orderToSync)
						}

						orderToSyncMap[myID] = orderToSync
						networkTx <- Encode(msgTransmitting)
					}
				}
			}
		case orderToDelete := <- orderDeleteBuffer:
			// Check which type of list to delete from
			listToModify := []Order{}
			if orderToDelete.OrderType == HALL {
				listToModify = ordersConfirmed_HALL
			}
			if orderToDelete.OrderType == CAB {
				listToModify = ordersConfirmed_CAB[orderToDelete.PeerID]
			}

			// Remove order
			for i, order := range(listToModify) {
				if order == orderToDelete {
					newOrderList, _, isPopped := PopOrder(listToModify, i)
					if !isPopped {
						log.Println("Could not pop order")
					}
					// Replace list
					if orderToDelete.OrderType == HALL {
						ordersConfirmed_HALL = newOrderList
					}
					if orderToDelete.OrderType == CAB {
						ordersConfirmed_CAB[myID] = newOrderList
					}
				}
			}
			
			txMsgUpdate <-true

		case txChanges := <- txMsgUpdate:
			if txChanges {
				// Set all peers to unsynced status
				for _, peerID := range(activePeersList) {
					isPeerSynced[peerID] = false
				}

				msgTransmitting.OrderToSyncMap = orderToSyncMap
				msgTransmitting.OrdersConfirmed_CAB = ordersConfirmed_CAB[myID]
				msgTransmitting.OrdersConfirmed_HALL = ordersConfirmed_HALL
				msgTransmitting.StateCounter += 1
				networkTx <-Encode(msgTransmitting)
			}

		case newPeerUpdate := <-peerUpdate:
			activePeersList = newPeerUpdate.Peers
		}
	}
}


// Pops a order at a given index  and returns a new list of orders, the popped order, 
// and a bool telling if a order was popped or not
func PopOrder(listOrders []Order, index int) ([]Order, Order, bool) {
	if len(listOrders) == 0 {
		return listOrders, Order{}, false
	}
	poppedOrder := listOrders[index]
	listOrders = append(listOrders[:index], listOrders[index+1:]...)

	return listOrders, poppedOrder, true
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
	err := json.Unmarshal([]byte(out), &incomingMsg)
	if err != nil {
		log.Println("json.Unmarshal error: ", err)
	}
	return incomingMsg
}