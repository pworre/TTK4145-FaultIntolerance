package syncOrderFSM

import (
	"elevatorControl/elevator"
	"log"
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

func StateMachineLoop(myID string, newOrderStateTransition chan map[string]order.Order, newOrderStateReceival chan map[string]order.Order, confirmedRequest chan order.Order, confirmedDeletion chan order.Order, networkDisconnect chan bool, clearAllConfirmedOrders chan bool, peerUpdateInSyncOrdersFSM chan peers.PeerUpdate, peerUpdateInRequestBarrierStateCounter chan peers.PeerUpdate, peerUpdateInDeletionBarrierStateCounter chan peers.PeerUpdate, peerUpdateInUnconfirmedRequestBarrierStateCounter chan peers.PeerUpdate, peerUpdateInUnconfirmedDeletionBarrierStateCounter chan peers.PeerUpdate) {

	activePeersList := make([]string, 0) // Most likely not needed???

	localOrderToSyncMap := make(map[string]order.Order)
	localOrderToSyncMap[myID] = order.NewOrder(myID, 0, elevator.B_Cab, order.SOS_NONE)

	// TODO: Random channels, sort later

	iAmAtRequestBarrier := make(chan acknowledgeBarrier, 64)
	iAmAtDeleteBarrier := make(chan acknowledgeBarrier, 64)
	iAmAtUnconfirmedRequestBarrier := make(chan acknowledgeBarrier, 64)
	iAmAtUnconfirmedDeleteBarrier := make(chan acknowledgeBarrier, 64)

	allAgreeToAddOrder := make(chan string)
	allAgreeToDeleteOrder := make(chan string)
	allHaveUnconfirmedRequest := make(chan string)
	allHaveUnconfirmedDeletion := make(chan string)

	// TODO: End channels

	go requestBarrierStateCounter(myID, peerUpdateInRequestBarrierStateCounter, iAmAtRequestBarrier, allAgreeToAddOrder)
	go deletionBarrierStateCounter(myID, peerUpdateInDeletionBarrierStateCounter, iAmAtDeleteBarrier, allAgreeToDeleteOrder)
	go unconfirmedRequestBarrierStateCounter(myID, peerUpdateInUnconfirmedRequestBarrierStateCounter, iAmAtUnconfirmedRequestBarrier, allHaveUnconfirmedRequest)
	go unconfirmedDeletionBarrierStateCounter(myID, peerUpdateInUnconfirmedDeletionBarrierStateCounter, iAmAtUnconfirmedDeleteBarrier, allHaveUnconfirmedDeletion)

	// ! VERY IMPORTANT ! Go over and see every time that a map or slice is sent on a channel!!! In both cases they must be copied...

	// ! VERY IMPORTANT ! When new peer initializes and joins, it sets itself as none and everyone else as unknown
	// ! Is this handled by default, or must we explicitly enforce this?

	// Probably done // TODO: Complete implementation of the barrier state counting, spawn correct number of threads in syncOrders
	// Probably done // TODO: For a peerUpdate, we need to update the localOrderToSyncMap
	// Probably done // TODO: Might be necessary to periodically send the map, not just with changes... then we also need a currentMessageToSend, most likely

	for {
		select {
		case newPeerUpdateShallowCopy := <-peerUpdateInSyncOrdersFSM:
			newPeerUpdate := peers.PeerUpdateClone(newPeerUpdateShallowCopy)
			activePeersList = newPeerUpdate.Peers
			// Update localOrderToSyncMap
			if newPeerUpdate.New != "" {
				if !(isKeyInMap(newPeerUpdate.New, localOrderToSyncMap)) {
					localOrderToSyncMap[newPeerUpdate.New] = order.NewOrder(newPeerUpdate.New, 0, elevator.B_Cab, order.SOS_NONE)
				}
			}
			for _, peerID := range newPeerUpdate.Lost {
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

			// !OBS! Debugging!
			//<-waitForReconnection // Blocks and does nothing until we are reconnected or restart

		case incomingOrderToSyncMapShallowCopy := <-newOrderStateReceival:
			incomingOrderToSyncMap := MapCopy(incomingOrderToSyncMapShallowCopy)
			log.Println("Entering the orderSync state machine. Incoming map: ", incomingOrderToSyncMap)

			for incomingID, incomingOrderToSync := range incomingOrderToSyncMap {
				if localOrderToSyncMap[incomingID].OrderState == order.SOS_UNKNOWN {
					localOrderToSyncMap[incomingID] = incomingOrderToSync
				} else {
					switch incomingOrderToSync.OrderState {
					case order.SOS_NONE:

						switch localOrderToSyncMap[incomingID].OrderState {
						case order.SOS_CONFIRMED_REQUEST:
							// Add confirmed order, turn on lights
							// ! Double-check that the order has state completed
							confirmedRequest <- localOrderToSyncMap[incomingID]
							updateOrderStateInMap(localOrderToSyncMap, incomingID, order.SOS_NONE)

						case order.SOS_CONFIRMED_DELETION:
							// Remove completed order, turn off lights
							// ! Double-check that the order has state completed
							confirmedDeletion <- localOrderToSyncMap[incomingID]
							updateOrderStateInMap(localOrderToSyncMap, incomingID, order.SOS_NONE)

						default:
							log.Println(incomingID, " told us they have no orders, and we dont care.")

						}

					case order.SOS_UNCONFIRMED_REQUEST:

						log.Printf("Peer %s sees UNCONFIRMED from %s\n", myID, incomingID)

						switch localOrderToSyncMap[incomingID].OrderState {
						case order.SOS_NONE:
							updateOrderStateInMap(localOrderToSyncMap, incomingID, order.SOS_UNCONFIRMED_REQUEST)

							// Need a second barrier, also for the unconfirmation......
							log.Printf("Peer %s sending UNCONFIRMED ACK for owner %s\n", myID, incomingID)
							iAmAtUnconfirmedRequestBarrier <- acknowledgeBarrier{ownerID: incomingID, ackID: myID}
							log.Println(incomingID, " told us they have a request, and we believe them!")

						case order.SOS_UNCONFIRMED_REQUEST:
							log.Printf("Peer %s sending UNCONFIRMED ACK for owner %s\n", myID, incomingID)
							iAmAtUnconfirmedRequestBarrier <- acknowledgeBarrier{ownerID: incomingID, ackID: myID}
							log.Println(incomingID, " told us they have a request, and we re-acknowledged!")

						default:

						}

					case order.SOS_UNCONFIRMED_DELETION:

						switch localOrderToSyncMap[incomingID].OrderState {
						case order.SOS_NONE:
							updateOrderStateInMap(localOrderToSyncMap, incomingID, order.SOS_UNCONFIRMED_DELETION)

							log.Printf("Peer %s sending UNCONFIRMED ACK for owner %s\n", myID, incomingID)
							iAmAtUnconfirmedDeleteBarrier <- acknowledgeBarrier{ownerID: incomingID, ackID: myID}

						case order.SOS_UNCONFIRMED_DELETION:
							log.Printf("Peer %s sending UNCONFIRMED ACK for owner %s\n", myID, incomingID)
							iAmAtUnconfirmedDeleteBarrier <- acknowledgeBarrier{ownerID: incomingID, ackID: myID}

						default:

						}

					case order.SOS_CONFIRMED_REQUEST:
						//incomingConfirmedRequest(incomingOrderToSync.PeerID)

						switch localOrderToSyncMap[incomingID].OrderState {
						case order.SOS_UNCONFIRMED_REQUEST:
							updateOrderStateInMap(localOrderToSyncMap, incomingID, order.SOS_CONFIRMED_REQUEST)
							iAmAtRequestBarrier <- acknowledgeBarrier{ownerID: incomingID, ackID: myID}

						case order.SOS_CONFIRMED_REQUEST:
							iAmAtRequestBarrier <- acknowledgeBarrier{ownerID: incomingID, ackID: myID}

						default:

						}

					case order.SOS_CONFIRMED_DELETION:
						//incomingConfirmedDeletion(incomingOrderToSync.PeerID)

						switch localOrderToSyncMap[incomingID].OrderState {
						case order.SOS_UNCONFIRMED_DELETION:
							updateOrderStateInMap(localOrderToSyncMap, incomingID, order.SOS_CONFIRMED_DELETION)
							iAmAtDeleteBarrier <- acknowledgeBarrier{ownerID: incomingID, ackID: myID}

						case order.SOS_CONFIRMED_DELETION:
							iAmAtDeleteBarrier <- acknowledgeBarrier{ownerID: incomingID, ackID: myID}

						default:

						}
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

		case peerThatCanMoveToConfirmRequest := <-allHaveUnconfirmedRequest:
			updateOrderStateInMap(localOrderToSyncMap, peerThatCanMoveToConfirmRequest, order.SOS_CONFIRMED_REQUEST)
			iAmAtRequestBarrier <- acknowledgeBarrier{ownerID: peerThatCanMoveToConfirmRequest, ackID: myID}
			log.Println(peerThatCanMoveToConfirmRequest, "has an order that is unconfirmed for everyone, so we make the executive decision to move on!")

		case peerThatCanMoveToConfirmDeletion := <-allHaveUnconfirmedDeletion:
			updateOrderStateInMap(localOrderToSyncMap, peerThatCanMoveToConfirmDeletion, order.SOS_CONFIRMED_DELETION)
			iAmAtDeleteBarrier <- acknowledgeBarrier{ownerID: peerThatCanMoveToConfirmDeletion, ackID: myID}

		}
		newOrderStateTransition <- MapCopy(localOrderToSyncMap) // Deep copy of map to be sent

		for _, str := range activePeersList {
			log.Printf("Peer number: %s", str)
		}

		log.Println("FSM localOrderToSyncMap: ", localOrderToSyncMap)
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

// ! These should probably both be moved to OrderSync and spawned as threads within there
// ! peerUpdate should prob be own channel
func requestBarrierStateCounter(myID string, peerUpdateInRequestBarrierStateCounter chan peers.PeerUpdate, iAmAtRequestBarrier chan acknowledgeBarrier, allAgreeToAddOrder chan string) {
	activePeersList := make([]string, 0)
	fullList := make([]string, 0)
	peersThatHaveConfirmedRequest := make(map[string][]string)
	log.Println("Entered the requestBarrierStateCounter!!!")

	for {
		select {
		case newPeerUpdateShallowCopy := <-peerUpdateInRequestBarrierStateCounter:
			newPeerUpdate := peers.PeerUpdateClone(newPeerUpdateShallowCopy)
			activePeersList = newPeerUpdate.Peers
			fullList = append([]string{}, activePeersList...)
			fullList = append(fullList, myID)
			// Update map
			if newPeerUpdate.New != "" {
				if !(isKeyInMap(newPeerUpdate.New, peersThatHaveConfirmedRequest)) {
					peersThatHaveConfirmedRequest[newPeerUpdate.New] = []string{}
				}
			}
			for _, peerID := range newPeerUpdate.Lost {
				if isKeyInMap(peerID, peersThatHaveConfirmedRequest) {
					delete(peersThatHaveConfirmedRequest, peerID)
				}
			}

			// We are also one of the peers that must possibly acknowledge
			peersThatHaveConfirmedRequest[myID] = []string{}

			log.Println("UUUUUHM, DO I HAVE THE RIGHT LIST????? ", fullList)
			// activePeersList and the peersThatHaveConfirmedRequest map keys should always have the same elements in them

		case acknowledgement := <-iAmAtRequestBarrier:
			log.Println("WTF????????? SHOULD PRINT")
			ownerID := acknowledgement.ownerID
			ackID := acknowledgement.ackID
			if !(isElementInList(ackID, peersThatHaveConfirmedRequest[ownerID])) {
				peersThatHaveConfirmedRequest[ownerID] = append(peersThatHaveConfirmedRequest[ownerID], ackID)
			}
			log.Println(ackID, " acknowledged that ", ownerID, " has an order! Full acklist: ", peersThatHaveConfirmedRequest[ownerID])
		}
		log.Println(
			"Barrier check:",
			"fullList:", fullList,
			"acks:", peersThatHaveConfirmedRequest,
		)
		// Check if everyone has reached barrier state, for each order in map
		for _, peerID := range fullList {
			if containSameElements(fullList, peersThatHaveConfirmedRequest[peerID]) {
				allAgreeToAddOrder <- peerID
				peersThatHaveConfirmedRequest[peerID] = make([]string, 0)
				log.Println("WOW, AN ACTUAL CONFIRMED ORDER! ", peerID, " owns it.")
			}
		}
	}
}

func deletionBarrierStateCounter(myID string, peerUpdateInDeletionBarrierStateCounter chan peers.PeerUpdate, iAmAtDeleteBarrier chan acknowledgeBarrier, allAgreeToDeleteOrder chan string) {
	activePeersList := make([]string, 0)
	fullList := make([]string, 0)
	peersThatHaveConfirmedDelete := make(map[string][]string)

	for {
		select {
		case newPeerUpdateShallowCopy := <-peerUpdateInDeletionBarrierStateCounter:
			newPeerUpdate := peers.PeerUpdateClone(newPeerUpdateShallowCopy)
			activePeersList = newPeerUpdate.Peers
			fullList = append([]string{}, activePeersList...)
			fullList = append(fullList, myID)
			// Update map
			if newPeerUpdate.New != "" {
				if !(isKeyInMap(newPeerUpdate.New, peersThatHaveConfirmedDelete)) {
					peersThatHaveConfirmedDelete[newPeerUpdate.New] = []string{}
				}
			}
			for _, peerID := range newPeerUpdate.Lost {
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
		for _, peerID := range fullList {
			if containSameElements(fullList, peersThatHaveConfirmedDelete[peerID]) {
				allAgreeToDeleteOrder <- peerID
				peersThatHaveConfirmedDelete[peerID] = make([]string, 0)
			}
		}
	}
}

func unconfirmedRequestBarrierStateCounter(myID string, peerUpdateInUnconfirmedRequestBarrierStateCounter chan peers.PeerUpdate, iAmAtUnconfirmedRequestBarrier chan acknowledgeBarrier, allHaveUnconfirmedRequest chan string) {
	activePeersList := make([]string, 0)
	fullList := make([]string, 0)
	peersThatHaveUnconfirmedRequest := make(map[string][]string)
	log.Println("Entered the unconfirmedRequestBarrierStateCounter!!!")

	for {
		select {
		case newPeerUpdateShallowCopy := <-peerUpdateInUnconfirmedRequestBarrierStateCounter:
			newPeerUpdate := peers.PeerUpdateClone(newPeerUpdateShallowCopy)
			activePeersList = newPeerUpdate.Peers
			fullList = append([]string{}, activePeersList...)
			fullList = append(fullList, myID)
			// Update map
			if newPeerUpdate.New != "" {
				if !(isKeyInMap(newPeerUpdate.New, peersThatHaveUnconfirmedRequest)) {
					peersThatHaveUnconfirmedRequest[newPeerUpdate.New] = []string{}
				}
			}
			for _, peerID := range newPeerUpdate.Lost {
				if isKeyInMap(peerID, peersThatHaveUnconfirmedRequest) {
					delete(peersThatHaveUnconfirmedRequest, peerID)
				}
			}

			// We are also one of the peers that must possibly acknowledge
			peersThatHaveUnconfirmedRequest[myID] = []string{}

			log.Println("UUUUUHM, DO I HAVE THE RIGHT UNCONFIRMED LIST????? ", fullList)
			// activePeersList and the peersThatHaveConfirmedRequest map keys should always have the same elements in them

		case acknowledgement := <-iAmAtUnconfirmedRequestBarrier:
			log.Println("WTF????????? SHOULD UNCONFIRMED PRINT")
			ownerID := acknowledgement.ownerID
			ackID := acknowledgement.ackID
			if !(isElementInList(ackID, peersThatHaveUnconfirmedRequest[ownerID])) {
				peersThatHaveUnconfirmedRequest[ownerID] = append(peersThatHaveUnconfirmedRequest[ownerID], ackID)
			}
			log.Println(ackID, " acknowledged that ", ownerID, " has an unconfirmed order! Full acklist: ", peersThatHaveUnconfirmedRequest[ownerID])
		}
		log.Println(
			"Barrier check:",
			"fullList:", fullList,
			"acks:", peersThatHaveUnconfirmedRequest,
		)
		// Check if everyone has reached barrier state, for each order in map
		for _, peerID := range fullList {
			if containSameElements(fullList, peersThatHaveUnconfirmedRequest[peerID]) {
				allHaveUnconfirmedRequest <- peerID
				peersThatHaveUnconfirmedRequest[peerID] = make([]string, 0)
				log.Println("WOW, AN ACTUAL CONFIRMED ORDER! ", peerID, " owns it.")
			}
		}
	}
}

func unconfirmedDeletionBarrierStateCounter(myID string, peerUpdateInUnconfirmedDeletionBarrierStateCounter chan peers.PeerUpdate, iAmAtUnconfirmedDeleteBarrier chan acknowledgeBarrier, allHaveUnconfirmedDeletion chan string) {
	activePeersList := make([]string, 0)
	fullList := make([]string, 0)
	peersThatHaveUnconfirmedDelete := make(map[string][]string)

	for {
		select {
		case newPeerUpdateShallowCopy := <-peerUpdateInUnconfirmedDeletionBarrierStateCounter:
			newPeerUpdate := peers.PeerUpdateClone(newPeerUpdateShallowCopy)
			activePeersList = newPeerUpdate.Peers
			fullList = append([]string{}, activePeersList...)
			fullList = append(fullList, myID)
			// Update map
			if newPeerUpdate.New != "" {
				if !(isKeyInMap(newPeerUpdate.New, peersThatHaveUnconfirmedDelete)) {
					peersThatHaveUnconfirmedDelete[newPeerUpdate.New] = []string{}
				}
			}
			for _, peerID := range newPeerUpdate.Lost {
				if isKeyInMap(peerID, peersThatHaveUnconfirmedDelete) {
					delete(peersThatHaveUnconfirmedDelete, peerID)
				}
			}
			// activePeersList and the peersThatHaveConfirmedDelete map keys should always have the same elements in them

		case acknowledgement := <-iAmAtUnconfirmedDeleteBarrier:
			ownerID := acknowledgement.ownerID
			ackID := acknowledgement.ackID
			if !(isElementInList(ackID, peersThatHaveUnconfirmedDelete[ownerID])) {
				peersThatHaveUnconfirmedDelete[ownerID] = append(peersThatHaveUnconfirmedDelete[ownerID], ackID)
			}
		}
		// Check if everyone has reached barrier state, for each order in map
		for _, peerID := range fullList {
			if containSameElements(fullList, peersThatHaveUnconfirmedDelete[peerID]) {
				allHaveUnconfirmedDeletion <- peerID
				peersThatHaveUnconfirmedDelete[peerID] = make([]string, 0)
			}
		}
	}
}

func MapCopy(oldMap map[string]order.Order) map[string]order.Order {
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
