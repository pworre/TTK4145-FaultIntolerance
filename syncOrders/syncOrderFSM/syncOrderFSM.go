package syncOrderFSM

import (
	"syncOrders/order"
	"log"
)

// Finite state machine loop

func StateMachineLoop(networkDisconnect chan bool, receivedOrderToSyncMap chan order.OrderMap, 
	allAgreeToAddOrder chan order.Order, allAgreeToDeleteOrder chan order.Order, 
	confirmedRequest chan order.Order, confirmedDeletion chan order.Order, 
	txMsgUpdate chan order.OrderMap, activePeersListCh chan []string) {
	
	localOrderToSyncMap := make(order.OrderMap)

	//localOrderToSync := order.Order{
	//PeerID:            myID,
	//OrderType:         elevator.B_Cab,
	//OrderFloor:        -1,
	//CurrentOrderState: COS_NONE,
	//}

	// TODO: RESET WHEN THERE EXISTS NEW LOCAL ORDER TO SYNC
	isPeerSyncedMap := make(map[string]bool)

	activePeersList := make([]string, 0)

// TODO: Complete implementation of the barrier state counting, spawn correct number of threads in main


	for {
		select {
		case <-networkDisconnect:
			for ID, _ := range localOrderToSyncMap {
				localOrderToSyncMap = setOrderState(localOrderToSyncMap, order.COS_UNKNOWN, ID)
			}

		case incomingOrderToSyncMap_shallowCopy := <-receivedOrderToSyncMap: // TODO: CHANGE TO receivedOrderToSyncMap ???
			incomingOrderToSyncMap := copyMap(incomingOrderToSyncMap_shallowCopy)// Deep Copy
			for incomingID, incomingOrderToSync := range incomingOrderToSyncMap {
				switch incomingOrderToSync.CurrentOrderState {
				case order.COS_NONE:

					// ! DID NOT UNDERSTAND !
					/*
					switch localOrderToSyncMap[incomingID].CurrentOrderState {
					case order.COS_CONFIRMED_REQUEST:
						iAgree := <- myID
						// Add to agreelist

					case order.COS_READY_TO_DELETE:
						iAgree := <- myID
						// Add to agreelist

					default:

					}*/

				
				case order.COS_UNCONFIRMED_REQUEST:
					
					switch localOrderToSyncMap[incomingID].CurrentOrderState {
					case order.COS_NONE:
						localOrderToSyncMap = setOrderState(localOrderToSyncMap, order.COS_UNCONFIRMED_REQUEST, incomingID)

					default:

					}
				
				case order.COS_UNCONFIRMED_DELETION:

					switch localOrderToSyncMap[incomingID].CurrentOrderState {
					case order.COS_NONE:
						localOrderToSyncMap = setOrderState(localOrderToSyncMap, order.COS_UNCONFIRMED_DELETION, incomingID)

					default:

					}
				
				case order.COS_CONFIRMED_REQUEST:
					if isAllPeersSynced(isPeerSyncedMap, activePeersList) {
						log.Println("All peers synced with my order to add")
						allAgreeToAddOrder <- incomingOrderToSync
					}

					switch localOrderToSyncMap[incomingID].CurrentOrderState {
					case order.COS_UNCONFIRMED_REQUEST:
						localOrderToSyncMap = setOrderState(localOrderToSyncMap, order.COS_CONFIRMED_REQUEST, incomingID)

					default:

					}
				
				case order.COS_READY_TO_DELETE:
					if isAllPeersSynced(isPeerSyncedMap, activePeersList) {
						log.Println("All peers synced with my order to remove")
						allAgreeToDeleteOrder <- incomingOrderToSync
					}
					
					switch localOrderToSyncMap[incomingID].CurrentOrderState {
					case order.COS_UNCONFIRMED_DELETION:
						localOrderToSyncMap = setOrderState(localOrderToSyncMap, order.COS_READY_TO_DELETE, incomingID)

					default:

					}

				}
		}
		
		case orderToAdd := <-allAgreeToAddOrder:
			// Add confirmed order, turn on lights
			// ! Double-check that the order has state completed
			// ? WHERE TO RECEIVE INCOMING ID AND KNOW WHICH ORDER TO ADD
			confirmedRequest <- orderToAdd
			localOrderToSyncMap = setOrderState(localOrderToSyncMap, order.COS_NONE, orderToAdd.PeerID)
		
		case orderToDelete := <-allAgreeToDeleteOrder:
			// Remove completed order, turn off lights
			// ! Double-check that the order has state completed
			confirmedDeletion <- orderToDelete
			localOrderToSyncMap = setOrderState(localOrderToSyncMap, order.COS_NONE, orderToDelete.PeerID)

		case newActivePeersList := <- activePeersListCh:
			activePeersList = newActivePeersList
		}
		// ? INTENTIONS HERE ? WHy send a deep-copy map when there is also confirmedlist for cab and hall orders
		mapToTransmit := copyMap(localOrderToSyncMap)
		txMsgUpdate <- mapToTransmit // Deep copy of map to be sent
		}
		
}

func isAllPeersSynced(isPeerSyncedMap map[string]bool, activePeersList []string) bool {
	isAllPeersSynced := true
	for _, ID := range(activePeersList) {
		isPeerSynced := isPeerSyncedMap[ID]
		if !isPeerSynced {
			isAllPeersSynced = false
		}
	}
	return isAllPeersSynced
}
/*
func confirmedRequestOrderPeerCounter(incomingConfirmedRequest chan string, incomingConfirmedDelete chan string, allAgreeToConfirm chan bool, activePeersList []string) {
	peersThatHaveConfirmedRequest := make([]string, len(activePeersList))
	
	for {
		select {
		case <-peerUpdate:
			peersThatHaveConfirmedRequest = make([]string, len(activePeersList))

		case peerID := <-incomingConfirmedRequest:
			peersThatHaveConfirmedRequest[peerID] = true

		}
		if isAllPeersSynced(peersThatHaveConfirmedRequest) {
			allAgreeToAddOrder <- true
			peersThatHaveConfirmedRequest := make([]string, len(activePeersList))
		}
	}
}

func confirmedDeletionOrderPeerCounter(incomingConfirmedRequest chan string, incomingConfirmedDelete chan string, allAgreeToDelete chan bool, activePeersList []string) {
	peersThatHaveConfirmedDeletion := make([]string, len(activePeersList))

	for {
		select {
		case <-peerUpdate:
			peersThatHaveConfirmedDeletion = make([]string, len(activePeersList))
		
		case peerID := <-incomingConfirmedDelete:
			peersThatHaveConfirmedDeletion[peerID] = true

		}
		if isAllPeersSynced(peersThatHaveConfirmedDeletion) {
			allAgreeToDeleteOrder <- true
			peersThatHaveConfirmedDeletion := make([]string, len(activePeersList))	// TODO: IMPROVE VARIABLE NAMING!
		}
	}
}
*/

func copyMap(oldMap order.OrderMap) order.OrderMap {
    newMap := make(order.OrderMap, len(oldMap))
    for key, value := range oldMap {
        newMap[key] = value
    }
    return newMap
}

func setOrderState(orderMapToModify order.OrderMap, newState order.CurrentOrderState, id string) order.OrderMap {
	tempOrder := orderMapToModify[id]
	tempOrder.CurrentOrderState = newState
	orderMapToModify[id] = tempOrder
	return orderMapToModify
}