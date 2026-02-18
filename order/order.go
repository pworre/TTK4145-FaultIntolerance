package order

import (
	"elevatorControl/elevator"
	"elevatorDriver/elevio"
	"networkDriver/bcast"
	"fmt"
	"log"
	"order"
	"elevator_project/config"
	"networkDriver/peers"
)

// ? Peer routing table [1, 2, 3, 4, ..., n] - Makes order of who transmits to who
/*
This file contains all struct and functions for order syncronization. 
Each node is sharing a OrderToSyncMap which is a map of their orders to be synced. 
They all share the confirmed list of orders "OrdersConfirmed"

The struct of the OrderToSyncMap is:

{
        Order{ID:"1", OrderType:HALL, OrderFloor:2, State:COS_UNCONFIRMED},
        Order{ID:"2", Floor:3, State:Pending},
        Order{ID:"3", Floor:1, State:Completed},
}


CASE 1 : Increasing order of peerID
List[PeerID] = order
*/

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
	OrderType 			orderType
	OrderFloor			int
	CurrentOrderState 	currentOrderState
}

type OrderNetworkMsg struct {
	PeerID					string
	OrderToSyncMap			map[string]Order
	OrdersConfirmed_HALL	[]Order
	OrdersConfirmed_CAB		[]Order
	StateCounter			uint64
}

const bcast_PORT = 25532

func orderSync(orderSyncBuffer chan Order, buttonEvent <-chan elevio.ButtonEvent, reachFloorEvent <-chan int) {
	cfg := config.ParseFlag()
	myID := cfg.ID
	
	networkRx := make(chan OrderNetworkMsg, 1024)
	networkTx := make(chan OrderNetworkMsg, 1024)
	
	go bcast.Transmitter(bcast_PORT, networkTx)
	go bcast.Receiver(bcast_PORT, networkRx)
	
	orderToSync := Order{
		PeerID:				myID,
		OrderType: 			HALL,
		OrderFloor: 		-1,
		CurrentOrderState: 	COS_NONE,
	}

	// MAPS for syncronization use
	orderToSyncMap := make(map[string]Order)
	orderToSyncMap[myID] = append(myID, orderToSync)

	ordersConfirmed_HALL := make([]Order, 0)
	ordersConfirmed_CAB := make(map[string][]Order)

	orderDeleteBuffer := make(chan Order, 1024)
	txMsgUpdate := make(chan bool, 1024)
	
	// ! TEMPORARY VARIABLES FOR CODING ONLY
	// TODO: This is just example for code, but must be implemented!
	currentOrder := Order{
		PeerID: 			myID,
		OrderType: 			HALL,
		OrderFloor: 		4,
		CurrentOrderState: 	COS_CONFIRMED_REQUEST,
	}
	
	// ? TAKE LIST OF ACTIVE PEERS AS AN INPUT ?
	peers := peers.PeerUpdate{}
	activePeersList := peers.Peers
	//confirmedOrders_HALL = append(confirmedOrders_HALL, currentOrder)
	//
	// ! END OF TEMPORARY VARIABLES FOR CODING ONLY



	isPeerSynced := make(map[string]bool, 0)
	for _, peerID := range(activePeersList) {
		isPeerSynced[peerID] = false
	}
	
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
				OrderType: 			orderType(buttonPressed.Button),
				OrderFloor: 		buttonPressed.Floor,
				CurrentOrderState: 	COS_UNCONFIRMED_REQUEST,
			}
			orderSyncBuffer <-orderToAdd

		case currentFloor := <-reachFloorEvent:
			if currentFloor == currentOrder.OrderFloor {
				completedOrder := currentOrder
				completedOrder.CurrentOrderState = COS_UNCONFIRMED_DELETION
				orderSyncBuffer <-completedOrder
			}

		case orderToHandle := <-orderSyncBuffer:
			orderToSyncMap[myID] = orderToHandle
			txMsgUpdate <- true

		/*
		case networkOrders := <-ordersConfirmed:
			* TODO: Add 'hallassigner' to choose next to do
		*/

		case msgReceived := <-networkRx:
			// Save maps if newer state
			if msgReceived.StateCounter > msgTransmitting.StateCounter {
				orderToSyncMap = msgReceived.OrderToSyncMap
				ordersConfirmed_CAB = msgReceived.OrdersConfirmed_CAB
				ordersConfirmed_HALL = msgReceived.OrdersConfirmed_HALL
				msgTransmitting.StateCounter = msgReceived.StateCounter - 1
			}

			// Checks if my OrderToSync is synced to all peers
			if msgReceived.OrderToSyncMap[myID] == orderToSync {
				isPeerSynced[msgReceived.PeerID] = true
				// ! WILL isPeerSynced be reset each time a new order is added to orderSyncBuffer ????
				isAllPeersSynced := true
				for _, peerID := range(activePeersList) {
					if isPeerSynced[peerID] == false {
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
						networkTx <- msgTransmitting
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
					newOrderList, _, isPopped := popOrder(listToModify, i)
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
				networkTx <-msgTransmitting
			}
		}
	}
}


// Pops a order at a given index  and returns a new list of orders, the popped order, 
// and a bool telling if a order was popped or not
func popOrder(listOrders []Order, index int) ([]Order, Order, bool) {
	if len(listOrders) == 0 {
		return listOrders, Order{}, false
	}
	poppedOrder := listOrders[index]
	listOrders = append(listOrders[:index], listOrders[index+1:]...)

	return listOrders, poppedOrder, true
}
