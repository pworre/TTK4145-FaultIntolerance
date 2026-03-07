package syncOrderFSM

import (
	"log"
	"elevatorControl/elevator"
	"networkDriver/peers"
	"slices"
	"syncOrders/order"
)

// ! Hope to change this struct or remove it

type acknowledgeBarrier struct {
	ownerID string
	ackID   string
}

// ! End

// Finite state machine loop

func StateMachineLoop(myID string, newOrderStateTransition chan map[string]order.Order, newOrderStateReceival chan map[string]order.Order, confirmedRequest chan order.Order, confirmedDeletion chan order.Order, networkDisconnect chan bool, clearAllConfirmedOrders chan bool, peerUpdateInSyncOrdersFSM chan peers.PeerUpdate, peerUpdateInRequestBarrierStateCounter chan peers.PeerUpdate, peerUpdateInDeletionBarrierStateCounter chan peers.PeerUpdate, waitForReconnection chan peers.PeerUpdate) {

	activePeersList := make([]string, 0) // Most likely not needed???

	localOrderToSyncMap := make(map[string]order.Order)

	// TODO: Random channels, sort later

	iAmAtRequestBarrier := make(chan acknowledgeBarrier)
	iAmAtDeleteBarrier := make(chan acknowledgeBarrier)

	allAgreeToAddOrder := make(chan string)
	allAgreeToDeleteOrder := make(chan string)



	// TODO: End channels

	go requestBarrierStateCounter(peerUpdateInRequestBarrierStateCounter, iAmAtRequestBarrier, allAgreeToAddOrder)
	go deletionBarrierStateCounter(peerUpdateInDeletionBarrierStateCounter, iAmAtDeleteBarrier, allAgreeToDeleteOrder)


// ! VERY IMPORTANT ! Go over and see every time that a map or slice is sent on a channel!!! In both cases they must be copied...




// ! VERY IMPORTANT ! When new peer initializes and joins, it sets itself as none and everyone else as unknown
// ! Is this handled by default, or must we explicitly enforce this?

// Probably done // TODO: Complete implementation of the barrier state counting, spawn correct number of threads in syncOrders
// Probably done // TODO: For a peerUpdate, we need to update the localOrderToSyncMap
// Probably done // TODO: Might be necessary to periodically send the map, not just with changes... then we also need a currentMessageToSend, most likely

	for {
		select {
		case newPeerUpdate := <-peerUpdateInSyncOrdersFSM:
			activePeersList = newPeerUpdate.Peers
			// Update localOrderToSyncMap
			if newPeerUpdate.New != "" {
				if !(isKeyInMap(newPeerUpdate.New, localOrderToSyncMap)) {
					localOrderToSyncMap[newPeerUpdate.New] = order.NewOrder(newPeerUpdate.New, 0, elevator.B_Cab, order.SOS_NONE)
				}
			}
			for _ , peerID := range newPeerUpdate.Lost {
				if isKeyInMap(peerID, localOrderToSyncMap) {
					delete(localOrderToSyncMap, peerID)
				}
			}
			// activePeersList and the localOrderToSyncMap map keys should always have the same elements in them
		


		case <-networkDisconnect:
			for ID, _ := range localOrderToSyncMap {
				updateOrderStateInMap(localOrderToSyncMap, ID, order.SOS_UNKNOWN)
			}
			clearAllConfirmedOrders <- true
			<-waitForReconnection // Blocks and does nothing until we are reconnected or restart

		case incomingOrderToSyncMapShallowCopy := <-newOrderStateReceival:
			incomingOrderToSyncMap := copyMap(incomingOrderToSyncMapShallowCopy)
			for incomingID, incomingOrderToSync := range incomingOrderToSyncMap {
				switch incomingOrderToSync.OrderState {
				case order.SOS_NONE:

					switch localOrderToSyncMap[incomingID].OrderState {
					case order.SOS_CONFIRMED_REQUEST:
						// Add confirmed order, turn on lights
						// ! Double-check that the order has state completed
						confirmedRequest <- localOrderToSyncMap[myID]
						updateOrderStateInMap(localOrderToSyncMap, myID, order.SOS_NONE)

					case order.SOS_CONFIRMED_DELETION:
						// Remove completed order, turn off lights
						// ! Double-check that the order has state completed
						confirmedDeletion <- localOrderToSyncMap[myID]
						updateOrderStateInMap(localOrderToSyncMap, myID, order.SOS_NONE)
						
					default:

					}

				
				case order.SOS_UNCONFIRMED_REQUEST:
					
					switch localOrderToSyncMap[incomingID].OrderState {
					case order.SOS_NONE:
						updateOrderStateInMap(localOrderToSyncMap, incomingID, order.SOS_UNCONFIRMED_REQUEST)

					default:

					}
				
				case order.SOS_UNCONFIRMED_DELETION:

					switch localOrderToSyncMap[incomingID].OrderState {
					case order.SOS_NONE:
						updateOrderStateInMap(localOrderToSyncMap, incomingID, order.SOS_UNCONFIRMED_DELETION)

					default:

					}
				
				case order.SOS_CONFIRMED_REQUEST:
					//incomingConfirmedRequest(incomingOrderToSync.PeerID)

					switch localOrderToSyncMap[incomingID].OrderState {
					case order.SOS_UNCONFIRMED_REQUEST:
						updateOrderStateInMap(localOrderToSyncMap, incomingID, order.SOS_CONFIRMED_REQUEST)
						iAmAtRequestBarrier <- acknowledgeBarrier{ownerID : incomingID, ackID : myID}

					default:

					}
				
				case order.SOS_CONFIRMED_DELETION:
					//incomingConfirmedDeletion(incomingOrderToSync.PeerID)
					
					switch localOrderToSyncMap[incomingID].OrderState {
					case order.SOS_UNCONFIRMED_DELETION:
						updateOrderStateInMap(localOrderToSyncMap, incomingID, order.SOS_UNCONFIRMED_DELETION)
						iAmAtDeleteBarrier <- acknowledgeBarrier{ownerID : incomingID, ackID : myID}

					default:

					}

				}
		}
		
		case peerThatCanAddOrder := <-allAgreeToAddOrder:
			// Add confirmed order, turn on lights
			// ! Double-check that the order has state completed
			confirmedRequest <- localOrderToSyncMap[peerThatCanAddOrder]
			updateOrderStateInMap(localOrderToSyncMap, peerThatCanAddOrder, order.SOS_NONE)
		
		case peerThatCanDeleteOrder := <-allAgreeToDeleteOrder:
			// Remove completed order, turn off lights
			// ! Double-check that the order has state completed
			confirmedDeletion <- localOrderToSyncMap[peerThatCanDeleteOrder]
			updateOrderStateInMap(localOrderToSyncMap, peerThatCanDeleteOrder, order.SOS_NONE)
		}
		newOrderStateTransition <- copyMap(localOrderToSyncMap) // Deep copy of map to be sent

		for _, str := range activePeersList {
				log.Printf("Peer number: %s", str)
			}
	}
}

//func doAllElevatorsAgree(allElevatorsThatAgree []string) bool {
//	if fullpeerlist == allElevatorsThatAgree {
//		return true
//	} else {
//		return false
//	}
//}

func containSameElements(firstList []string, secondList []string) bool {
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

func isElementInList(elem string, list []string) bool {
	for _, listElem := range(list) {
		if elem == listElem {
			return true
		}
	}
	return false
}

func isKeyInMap[T any](key string, theMap map[string]T) bool {
	_ , isInMap := theMap[key]
	return isInMap
}

// ! These should probably both be moved to OrderSync and spawned as threads within there
// ! peerUpdate should prob be own channel
func requestBarrierStateCounter(peerUpdateInRequestBarrierStateCounter chan peers.PeerUpdate, iAmAtRequestBarrier chan acknowledgeBarrier, allAgreeToAddOrder chan string) {
	activePeersList := make([]string, 0)
	peersThatHaveConfirmedRequest := make(map[string][]string)

	for {
		select {
		case newPeerUpdate := <-peerUpdateInRequestBarrierStateCounter:
			activePeersList = newPeerUpdate.Peers
			// Update map
			if newPeerUpdate.New != "" {
				if !(isKeyInMap(newPeerUpdate.New, peersThatHaveConfirmedRequest)) {
					peersThatHaveConfirmedRequest[newPeerUpdate.New] = []string{}
				}
			}
			for _ , peerID := range newPeerUpdate.Lost {
				if isKeyInMap(peerID, peersThatHaveConfirmedRequest) {
					delete(peersThatHaveConfirmedRequest, peerID)
				}
			}
			// activePeersList and the peersThatHaveConfirmedRequest map keys should always have the same elements in them
		
		case acknowledgement := <-iAmAtRequestBarrier:
			ownerID := acknowledgement.ownerID
			ackID := acknowledgement.ackID
			if !(isElementInList(ackID, peersThatHaveConfirmedRequest[ownerID])) {
				peersThatHaveConfirmedRequest[ownerID] = append(peersThatHaveConfirmedRequest[ownerID], ackID)
			}
		}
		// Check if everyone has reached barrier state, for each order in map
		for _ , peerID := range activePeersList {
			if containSameElements(activePeersList, peersThatHaveConfirmedRequest[peerID]) {
				allAgreeToAddOrder <- peerID
				peersThatHaveConfirmedRequest[peerID] = make([]string, 0)
			}
		}
	}
}

func deletionBarrierStateCounter(peerUpdateInDeletionBarrierStateCounter chan peers.PeerUpdate, iAmAtDeleteBarrier chan acknowledgeBarrier, allAgreeToDeleteOrder chan string) {
	activePeersList := make([]string, 0)
	peersThatHaveConfirmedDelete := make(map[string][]string)

	for {
		select {
		case newPeerUpdate := <-peerUpdateInDeletionBarrierStateCounter:
			activePeersList = newPeerUpdate.Peers
			// Update map
			if newPeerUpdate.New != "" {
				if !(isKeyInMap(newPeerUpdate.New, peersThatHaveConfirmedDelete)) {
					peersThatHaveConfirmedDelete[newPeerUpdate.New] = []string{}
				}
			}
			for _ , peerID := range newPeerUpdate.Lost {
				if isKeyInMap(peerID, peersThatHaveConfirmedDelete) {
					delete(peersThatHaveConfirmedDelete, peerID)
				}
			}
			// activePeersList and the peersThatHaveConfirmedDelete map keys should always have the same elements in them
		
		case acknowledgement := <-iAmAtDeleteBarrier:
			ownerID := acknowledgement.ownerID
			ackID := acknowledgement.ackID
			if !(isElementInList(ackID, peersThatHaveConfirmedDelete[ownerID])) {
				peersThatHaveConfirmedDelete[ownerID] = append(peersThatHaveConfirmedDelete[ownerID], ackID)
			}
		}
		// Check if everyone has reached barrier state, for each order in map
		for _ , peerID := range activePeersList {
			if containSameElements(activePeersList, peersThatHaveConfirmedDelete[peerID]) {
				allAgreeToDeleteOrder <- peerID
				peersThatHaveConfirmedDelete[peerID] = make([]string, 0)
			}
		}
	}
}

func copyMap(oldMap map[string]order.Order) map[string]order.Order {
    newMap := make(map[string]order.Order, len(oldMap))
    for key, value := range oldMap {
        newMap[key] = value
    }
    return newMap
}

func updateOrderStateInMap(theMap map[string]order.Order, key string, state order.SyncOrderState) {
	currentOrder := theMap[key]
	theMap[key] = order.NewOrder(key, currentOrder.OrderFloor, currentOrder.OrderType, state)
}