package syncOrderFSM

import (
	"syncOrders"
)

// Finite state machine loop

func StateMachineLoop(networkDisconnect chan bool, orderToSyncMapMessage chan map[string]syncOrders.Order,
	allAgreeToAddOrder chan syncOrders.Order, allAgreeToDeleteOrder chan syncOrders.Order,
	confirmedRequest chan syncOrders.Order, confirmedDeletion chan syncOrders.Order,
	txMsgUpdate chan map[string]syncOrders.Order, activePeersList []string) {

	localOrderToSyncMap := make(map[string]syncOrders.Order)

	//localOrderToSync := syncOrders.Order{
	//PeerID:            myID,
	//OrderType:         elevator.B_Cab,
	//OrderFloor:        -1,
	//CurrentOrderState: COS_NONE,
	//}

	// TODO: RESET WHEN THERE EXISTS NEW LOCAL ORDER TO SYNC
	isPeerSyncedMap := make(map[string]bool)

	// TODO: Complete implementation of the barrier state counting, spawn correct number of threads in main
	// TODO: For a peerUpdate, we need to update the localOrderToSyncMap

	for {
		select {
		case <-networkDisconnect:
			for ID, _ := range localOrderToSyncMap {
				localOrderToSyncMap = setOrderState(localOrderToSyncMap, syncOrders.COS_UNKNOWN, ID)
			}

		case incomingOrderToSyncMap_shallowCopy := <-orderToSyncMapMessage: // TODO: CHANGE TO receivedOrderToSyncMap ???
			incomingOrderToSyncMap := copyMap(incomingOrderToSyncMap_shallowCopy) // Deep Copy
			for incomingID, incomingOrderToSync := range incomingOrderToSyncMap {
				switch incomingOrderToSync.CurrentOrderState {
				case syncOrders.COS_NONE:

					switch localOrderToSyncMap[incomingID].CurrentOrderState {
					case syncOrders.COS_CONFIRMED_REQUEST:
						iAgree := <-myID
						// Add to agreelist

					case syncOrders.COS_READY_TO_DELETE:
						iAgree := <-myID
						// Add to agreelist

					default:

					}

				case syncOrders.COS_UNCONFIRMED_REQUEST:

					switch localOrderToSyncMap[incomingID].CurrentOrderState {
					case syncOrders.COS_NONE:
						localOrderToSyncMap = setOrderState(localOrderToSyncMap, syncOrders.COS_UNCONFIRMED_REQUEST, incomingID)

					default:

					}

				case syncOrders.COS_UNCONFIRMED_DELETION:

					switch localOrderToSyncMap[incomingID].CurrentOrderState {
					case syncOrders.COS_NONE:
						localOrderToSyncMap = setOrderState(localOrderToSyncMap, syncOrders.COS_UNCONFIRMED_DELETION, incomingID)

					default:

					}

				case syncOrders.COS_CONFIRMED_REQUEST:
					incomingConfirmedRequest(incomingOrderToSync.PeerID)

					switch localOrderToSyncMap[incomingID].CurrentOrderState {
					case syncOrders.COS_UNCONFIRMED_REQUEST:
						localOrderToSyncMap = setOrderState(localOrderToSyncMap, syncOrders.COS_CONFIRMED_REQUEST, incomingID)

					default:

					}

				case syncOrders.COS_READY_TO_DELETE:
					incomingConfirmedDeletion(incomingOrderToSync.PeerID)

					switch localOrderToSyncMap[incomingID].CurrentOrderState {
					case syncOrders.COS_UNCONFIRMED_DELETION:
						localOrderToSyncMap = setOrderState(localOrderToSyncMap, syncOrders.COS_READY_TO_DELETE, incomingID)

					default:

					}

				}
			}

		case orderToAdd := <-allAgreeToAddOrder:
			// Add confirmed order, turn on lights
			// ! Double-check that the order has state completed
			// ? WHERE TO RECEIVE INCOMING ID AND KNOW WHICH ORDER TO ADD
			confirmedRequest <- orderToAdd
			localOrderToSyncMap = setOrderState(localOrderToSyncMap, syncOrders.COS_NONE, orderToAdd.PeerID)

		case orderToDelete := <-allAgreeToDeleteOrder:
			// Remove completed order, turn off lights
			// ! Double-check that the order has state completed
			confirmedDeletion <- orderToDelete
			localOrderToSyncMap = setOrderState(localOrderToSyncMap, syncOrders.COS_NONE, orderToDelete.PeerID)
		}
		// ? INTENTIONS HERE ? WHy send a deep-copy map when there is also confirmedlist for cab and hall orders
		mapToTransmit := copyMap(localOrderToSyncMap)
		txMsgUpdate <- mapToTransmit // Deep copy of map to be sent
	}
}

func isPeersSynced(isPeerSyncedMap map[string]bool, activePeersList []string) bool {
	isAllPeersSynced := true
	for _, ID := range activePeersList {
		isPeerSynced := isPeerSyncedMap[ID]
		if !isPeerSynced {
			isAllPeersSynced = false
		}
	}
	return isAllPeersSynced
}

func confirmedRequestOrderPeerCounter(incomingConfirmedRequest chan string, incomingConfirmedDelete chan string, allAgreeToConfirm chan bool, activePeersList []string) {
	peersThatHaveConfirmedRequest := make([]string, len(activePeersList))

	for {
		select {
		case <-peerUpdate:
			peersThatHaveConfirmedRequest = make([]string, len(activePeersList))

		case peerID := <-incomingConfirmedRequest:
			peersThatHaveConfirmedRequest[peerID] = true

		}
		if allElevatorAgree(peersThatHaveConfirmedRequest) {
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
		if isPeersSynced(peersThatHaveConfirmedDeletion) {
			allAgreeToDeleteOrder <- true
			peersThatHaveConfirmedDeletion = make([]string, len(activePeersList)) // TODO: IMPROVE VARIABLE NAMING!
		}
	}
}

func copyMap(oldMap map[string]syncOrders.Order) map[string]syncOrders.Order {
	newMap := make(map[string]syncOrders.Order, len(oldMap))
	for key, value := range oldMap {
		newMap[key] = value
	}
	return newMap
}

func setOrderState(orderMapToModify map[string]syncOrders.Order, newState syncOrders.CurrentOrderState, id string) map[string]syncOrders.Order {
	tempOrder := orderMapToModify[id]
	tempOrder.CurrentOrderState = newState
	orderMapToModify[id] = tempOrder
	return orderMapToModify
}
