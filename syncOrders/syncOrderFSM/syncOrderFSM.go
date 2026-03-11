package syncOrderFSM

import (
	//"elevatorControl/elevator"
	"log"
	"networkDriver/peers"
	"slices"
	"syncOrders/order"
)

// ! Hope to change this struct or remove it

type acknowledgeBarrier struct {
	ownerID string
	ackID   string
	ord		order.Order
}

// ! End

// Finite state machine loop

// TODO: Assert that both the fsm localOrderToSyncMap and the syncOrders orderToSyncMap have the same members!
// TODO: This could probably warrant combining them both into one long file,
// TODO: but that would make the single file incredibly long and with way too many responsibilities,
// TODO: which is probably bad code quality...
// TODO: We will see what we have to to, but it seems paramount that the maps have the same keys

func StateMachineLoop(myID string, newOrderStateTransition chan map[string]order.Order, newOrderStateReceival chan order.OrderStateMessage, confirmedRequest chan order.Order, confirmedDeletion chan order.Order, networkDisconnect chan bool, clearAllConfirmedOrders chan bool, peerUpdateInSyncOrdersFSM chan peers.PeerUpdate, peerUpdateInRequestBarrierStateCounter chan peers.PeerUpdate, peerUpdateInDeletionBarrierStateCounter chan peers.PeerUpdate, peerUpdateInUnconfirmedRequestBarrierStateCounter chan peers.PeerUpdate, peerUpdateInUnconfirmedDeletionBarrierStateCounter chan peers.PeerUpdate) {

	//activePeersList := make([]string, 0) // Most likely not needed???

	localOrderToSyncMap := make(map[string]order.Order)
	localOrderToSyncMap[myID] = order.NewEmptyOrder(myID)

	// TODO: Random channels, sort later

	iAmAtRequestBarrier := make(chan acknowledgeBarrier, 64)
	iAmAtDeleteBarrier := make(chan acknowledgeBarrier, 64)
	iAmAtUnconfirmedRequestBarrier := make(chan acknowledgeBarrier, 64)
	iAmAtUnconfirmedDeleteBarrier := make(chan acknowledgeBarrier, 64)

	allAgreeToAddOrder := make(chan order.Order)
	allAgreeToDeleteOrder := make(chan order.Order)
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
			//activePeersList = newPeerUpdate.Peers
			// Update localOrderToSyncMap
			if newPeerUpdate.New != "" {
				if !(isKeyInMap(newPeerUpdate.New, localOrderToSyncMap)) {
					localOrderToSyncMap[newPeerUpdate.New] = order.NewUnknownOrder(newPeerUpdate.New)
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

			//<-waitForReconnection // Blocks and does nothing until we are reconnected or restart

			// ! For the reason stated above in the big TODO,
			// ! we need to prioritize peerUpdates and networkDisconnects to ensure that the maps have the same members

		default:
			select {
			case incomingOrderToSyncMessage := <-newOrderStateReceival:
				incomingOrderToSyncMap := MapCopy(incomingOrderToSyncMessage.OrderToSyncMap)
				log.Println("Entering the orderSync state machine. Incoming map: ", incomingOrderToSyncMap)
				incomingID := incomingOrderToSyncMessage.TransmittedPeerID

				for key_ID, incomingOrderToSync := range incomingOrderToSyncMap {
					localOrder, localExists := localOrderToSyncMap[key_ID]

					if !localExists {
						//localOrderToSyncMap[key_ID] = incomingOrderToSync
						continue
					}

					if localOrder.OrderState == order.SOS_UNKNOWN {
						localOrderToSyncMap[key_ID] = incomingOrderToSync
					}

					switch incomingOrderToSync.OrderState {
					case order.SOS_NONE:

						switch localOrderToSyncMap[key_ID].OrderState {
						case order.SOS_CONFIRMED_REQUEST:
							// Add confirmed order, turn on lights
							// TODO: Double-check that the order has state completed
							// ! Update: Probably done!

							if !order.IsValid(incomingOrderToSync) {
								break
							}

							if localOrderToSyncMap[key_ID].OrderState == order.SOS_NONE {
								log.Println("WARNING: Attempt to add NONE order to confirmed request list:", localOrderToSyncMap[key_ID])
							} else {
								confirmedRequest <- localOrderToSyncMap[key_ID]
							}
							updateOrderStateInMap(localOrderToSyncMap, key_ID, order.SOS_NONE)

						case order.SOS_CONFIRMED_DELETION:
							// Remove completed order, turn off lights
							// TODO: Double-check that the order has state completed
							// ! Update: Probably done!

							if order.IsValid(incomingOrderToSync) {
								break
							}

							if localOrderToSyncMap[key_ID].OrderState == order.SOS_NONE {
								log.Println("WARNING: Attempt to add NONE order to confirmed delete list:", localOrderToSyncMap[key_ID])
							} else {
								confirmedDeletion <- localOrderToSyncMap[key_ID]
							}
							updateOrderStateInMap(localOrderToSyncMap, key_ID, order.SOS_NONE)

						default:
							log.Println(incomingID, " told us they have no orders, and we dont care.")

						}

					case order.SOS_UNCONFIRMED_REQUEST:

						iAmAtUnconfirmedRequestBarrier <- acknowledgeBarrier{
							ownerID: key_ID, 
							ackID: incomingID,
							ord: incomingOrderToSync,
						}

						log.Printf("Peer %s sees UNCONFIRMED for owner %s from sender %s\n", myID, key_ID, incomingID)

						switch localOrderToSyncMap[key_ID].OrderState {
						case order.SOS_NONE:
							localOrderToSyncMap[key_ID] = incomingOrderToSync

							// Need a second barrier, also for the unconfirmation......
							log.Printf("Peer %s sending UNCONFIRMED ACK for owner %s from sender %s\n", myID, key_ID, incomingID)
							
							iAmAtUnconfirmedRequestBarrier <- acknowledgeBarrier{
								ownerID: 	key_ID, 
								ackID:		myID,
								ord: 		incomingOrderToSync,
							}

							log.Println(incomingID, " told us they have a request, and we believe them!")

						case order.SOS_UNCONFIRMED_REQUEST:
							// ! DO NOT RE-ACKNOWLEDGE FOR SAME STATE
							/*
								localOrderToSyncMap[key_ID] = incomingOrderToSync
								log.Printf("Peer %s sending UNCONFIRMED ACK for owner %s from sender %s\n", myID, key_ID, incomingID)
								iAmAtUnconfirmedRequestBarrier <- acknowledgeBarrier{ownerID: key_ID, ackID: myID}
								log.Println(incomingID, " told us they have a request, and we re-acknowledged!")
							*/
							localOrderToSyncMap[key_ID] = incomingOrderToSync

						default:

						}

					case order.SOS_UNCONFIRMED_DELETION:

						iAmAtUnconfirmedDeleteBarrier <- acknowledgeBarrier{
							ownerID: key_ID, 
							ackID: incomingID,
							ord: incomingOrderToSync,
						}

						switch localOrderToSyncMap[key_ID].OrderState {
						case order.SOS_NONE:
							localOrderToSyncMap[key_ID] = incomingOrderToSync

							log.Printf("Peer %s sending UNCONFIRMED DELETION ACK for owner %s from sender %s\n", myID, key_ID, incomingID)
							iAmAtUnconfirmedDeleteBarrier <- acknowledgeBarrier{
								ownerID: key_ID, 
								ackID: myID,
								ord: incomingOrderToSync,
							}

						case order.SOS_UNCONFIRMED_DELETION:
							// ! DO NOT RE-ACKNOWLEDGE FOR SAME STATE
							/*
								localOrderToSyncMap[key_ID] = incomingOrderToSync
								log.Printf("Peer %s sending UNCONFIRMED DELETE ACK for owner %s from sender %s\n", myID, key_ID, incomingID)
								iAmAtUnconfirmedDeleteBarrier <- acknowledgeBarrier{ownerID: key_ID, ackID: myID}
							*/
							localOrderToSyncMap[key_ID] = incomingOrderToSync
						default:

						}

					case order.SOS_CONFIRMED_REQUEST:
						//incomingConfirmedRequest(incomingOrderToSync.PeerID)
						iAmAtRequestBarrier <- acknowledgeBarrier{ownerID: key_ID, ackID: incomingID}

						switch localOrderToSyncMap[key_ID].OrderState {
						case order.SOS_UNCONFIRMED_REQUEST:
							localOrderToSyncMap[key_ID] = incomingOrderToSync
							iAmAtRequestBarrier <- acknowledgeBarrier{ownerID: key_ID, ackID: myID}

						case order.SOS_CONFIRMED_REQUEST:
							localOrderToSyncMap[key_ID] = incomingOrderToSync
							//iAmAtRequestBarrier <- acknowledgeBarrier{ownerID: key_ID, ackID: myID}

						default:

						}

					case order.SOS_CONFIRMED_DELETION:
						//incomingConfirmedDeletion(incomingOrderToSync.PeerID)
						iAmAtDeleteBarrier <- acknowledgeBarrier{ownerID: key_ID, ackID: incomingID}

						switch localOrderToSyncMap[key_ID].OrderState {
						case order.SOS_UNCONFIRMED_DELETION:
							localOrderToSyncMap[key_ID] = incomingOrderToSync
							iAmAtDeleteBarrier <- acknowledgeBarrier{ownerID: key_ID, ackID: myID}

						case order.SOS_CONFIRMED_DELETION:
							localOrderToSyncMap[key_ID] = incomingOrderToSync
							//iAmAtDeleteBarrier <- acknowledgeBarrier{ownerID: key_ID, ackID: myID}

						default:

						}
					}
				}

			case orderToAdd := <-allAgreeToAddOrder:
				// Add confirmed order, turn on lights
				log.Printf("FSM adding confirmedRequest %+v\n", orderToAdd)
				if orderToAdd.OrderState != order.SOS_CONFIRMED_REQUEST {
					log.Println("WARNING: FSM attempt to add a non_CONFIRMED_REQUEST order to confirmed request list:", localOrderToSyncMap[peerThatCanAddOrder])
					break
				}
				confirmedRequest <- orderToAdd
				updateOrderStateInMap(localOrderToSyncMap, orderToAdd.PeerID, order.SOS_NONE)
				newOrderStateTransition <- MapCopy(localOrderToSyncMap)
				// !!!!!!!!!!!!!!!IMPORTANT!!!!!!!!!!!!!!!

				// ! For some reason, NONE orders are added to be deleted.
				// ! There must be an error in the fsm logic or the barrier state counter somewhere, but have not found it yet.
				// ! But adding a request seems to be fine, so that is weird i guess...

			case orderToDelete := <-allAgreeToDeleteOrder:
				log.Printf("FSM adding confirmedDeletion %+v\n", orderToDelete)

				if orderToDelete.OrderState != order.SOS_CONFIRMED_DELETION {
					log.Printf("WARNING: refusing a non-confirmed-to-delete order to be deleted")
					break
				}

				confirmedDeletion <- orderToDelete
				updateOrderStateInMap(localOrderToSyncMap, orderToDelete.PeerID, order.SOS_NONE)
				newOrderStateTransition <- MapCopy(localOrderToSyncMap)

			case peerThatCanMoveToConfirmRequest := <-allHaveUnconfirmedRequest:
				_, peerIsInMap := localOrderToSyncMap[peerThatCanMoveToConfirmRequest]
				if !peerIsInMap {
					break
				}

				updateOrderStateInMap(localOrderToSyncMap, peerThatCanMoveToConfirmRequest, order.SOS_CONFIRMED_REQUEST)
				iAmAtRequestBarrier <- acknowledgeBarrier{ownerID: peerThatCanMoveToConfirmRequest, ackID: myID}
				log.Println(peerThatCanMoveToConfirmRequest, "has an order that is unconfirmed for everyone, so we make the executive decision to move on!")

			case peerThatCanMoveToConfirmDeletion := <-allHaveUnconfirmedDeletion:
				_, peerIsInMap := localOrderToSyncMap[peerThatCanMoveToConfirmDeletion]
				if !peerIsInMap {
					break
				}

				updateOrderStateInMap(localOrderToSyncMap, peerThatCanMoveToConfirmDeletion, order.SOS_CONFIRMED_DELETION)
				iAmAtDeleteBarrier <- acknowledgeBarrier{ownerID: peerThatCanMoveToConfirmDeletion, ackID: myID}
			}

		}
		newOrderStateTransition <- MapCopy(localOrderToSyncMap) // Deep copy of map to be sent

		//for _, str := range activePeersList {
		//	log.Printf("Peer number: %s", str)
		//}

		//log.Println("FSM localOrderToSyncMap: ", localOrderToSyncMap)
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
func requestBarrierStateCounter(myID string, peerUpdateInRequestBarrierStateCounter chan peers.PeerUpdate, iAmAtRequestBarrier chan acknowledgeBarrier, allAgreeToAddOrder chan order.Order) {
	activePeersList := make([]string, 0)
	fullList := []string{}
	peersThatHaveConfirmedRequest := make(map[string][]string)
	currentConfirmedOrder := make(map[string]order.Order)
	//peersThatHaveConfirmedRequest[myID] = []string{}
	log.Println("Entered the requestBarrierStateCounter!!!")

	for {
		select {
		case newPeerUpdateShallowCopy := <-peerUpdateInRequestBarrierStateCounter:
			newPeerUpdate := peers.PeerUpdateClone(newPeerUpdateShallowCopy)
			activePeersList = newPeerUpdate.Peers

			fullList = slices.Clone(activePeersList)
			fullList = append(fullList, myID)

			for peerConfirmed := range peersThatHaveConfirmedRequest {
				if !isElementInList(peerConfirmed, fullList) {
					delete(peersThatHaveConfirmedRequest, peerConfirmed)
					delete(currentConfirmedOrder, peerConfirmed)
				}
			}

			for _, peerID := range fullList {
				if !isKeyInMap(peerID, peersThatHaveConfirmedRequest) {
					peersThatHaveConfirmedRequest[peerID] = []string{}
				}
			}

			log.Println("UUUUUHM, DO I HAVE THE RIGHT CONFIRMED LIST????? ", fullList)
			// activePeersList and the peersThatHaveConfirmedRequest map keys should always have the same elements in them

		case acknowledgement := <-iAmAtRequestBarrier:
			log.Println("WTF????????? SHOULD PRINT")
			ownerID := acknowledgement.ownerID
			ackID := acknowledgement.ackID
			ord := acknowledgement.ord

			// ignore acks for peers we don't know about
			if !isElementInList(ownerID, fullList) || !isElementInList(ackID, fullList) {
				break
			}

			if !isKeyInMap(ownerID, peersThatHaveConfirmedRequest) {
				log.Println("Barrier state conf. req. counter does not maintain its peerlist correctly...")
				break
			}

			if !(isElementInList(ackID, peersThatHaveConfirmedRequest[ownerID])) {
				peersThatHaveConfirmedRequest[ownerID] = append(peersThatHaveConfirmedRequest[ownerID], ackID)
			}
			log.Println(ackID, " acknowledged that ", ownerID, " has an order! Full acklist: ", peersThatHaveConfirmedRequest[ownerID])
		
			// decide which barrier the order is about
			if len(peersThatHaveConfirmedRequest[ownerID]) == 0 {
				currentConfirmedOrder[ownerID] = ord
			} else {
				// Ignore acknowledgements which are outdated
				if currentConfirmedOrder[ownerID].OrderFloor != ord.OrderFloor ||
					currentConfirmedOrder[ownerID].OrderType != ord.OrderType ||
					currentConfirmedOrder[ownerID].OrderState != ord.OrderState {
						break
					}
				}
			
			if !isElementInList(ackID, peersThatHaveConfirmedRequest[ownerID]) {
				peersThatHaveConfirmedRequest[ownerID] = append(peersThatHaveConfirmedRequest[ownerID], ackID) 
			}
		}
		log.Println(
			"Barrier check:",
			"fullList:", fullList,
			"acks:", peersThatHaveConfirmedRequest,
		)
		// Check if everyone has reached barrier state, for each order in map
		for _, peerID := range fullList {
			if containSameElements(fullList, peersThatHaveConfirmedRequest[peerID]) {
				ord, orderExists := currentConfirmedOrder[peerID]
				if !orderExists {
					peersThatHaveConfirmedRequest[peerID] = []string{}
					continue
				}
				allAgreeToAddOrder <- ord
				peersThatHaveConfirmedRequest[peerID] = []string{}
				log.Println("WOW, AN ACTUAL CONFIRMED ORDER! ", peerID, " owns it.")
			}
		}
	}
}

func deletionBarrierStateCounter(myID string, peerUpdateInDeletionBarrierStateCounter chan peers.PeerUpdate, iAmAtDeleteBarrier chan acknowledgeBarrier, allAgreeToDeleteOrder chan order.Order) {
	activePeersList := make([]string, 0)
	fullList := []string{}
	peersThatHaveConfirmedDelete := make(map[string][]string)
	currentDeleteOrder := make(map[string]order.Order)
	//peersThatHaveConfirmedDelete[myID] = []string{}
	log.Println("Entered the deletionBarrierStateCounter!!!")

	for {
		select {
		case newPeerUpdateShallowCopy := <-peerUpdateInDeletionBarrierStateCounter:
			newPeerUpdate := peers.PeerUpdateClone(newPeerUpdateShallowCopy)
			activePeersList = newPeerUpdate.Peers

			fullList = slices.Clone(activePeersList)
			fullList = append(fullList, myID)

			for peerConfirmDelete := range peersThatHaveConfirmedDelete {
				if !isElementInList(peerConfirmDelete, fullList) {
					delete(peersThatHaveConfirmedDelete, peerConfirmDelete)
					delete(currentDeleteOrder, peerConfirmDelete)
				}
			}

			for _, peerID := range fullList {
				if !isKeyInMap(peerID, peersThatHaveConfirmedDelete) {
					peersThatHaveConfirmedDelete[peerID] = []string{}
				}
			}

			log.Println("UUUUUHM, DO I HAVE THE RIGHT CONFIRMED DELETE LIST????? ", fullList)
			// activePeersList and the peersThatHaveConfirmedDelete map keys should always have the same elements in them

		case acknowledgement := <-iAmAtDeleteBarrier:
			ownerID := acknowledgement.ownerID
			ackID := acknowledgement.ackID
			ord := acknowledgement.ord

			// ignore acks for peers we don't know about
			if !isElementInList(ownerID, fullList) || !isElementInList(ackID, fullList) {
				break
			}

			if !isKeyInMap(ownerID, peersThatHaveConfirmedDelete) {
				log.Println("Barrier state conf. del. counter does not maintain its peerlist correctly...")
				break
			}

			if !(isElementInList(ackID, peersThatHaveConfirmedDelete[ownerID])) {
				peersThatHaveConfirmedDelete[ownerID] = append(peersThatHaveConfirmedDelete[ownerID], ackID)
			}

			if len(peersThatHaveConfirmedDelete[ownerID]) == 0 {
				currentDeleteOrder[ownerID] = ord
			} else {
				if currentDeleteOrder[ownerID].OrderFloor != ord.OrderFloor ||
					currentDeleteOrder[ownerID].OrderType != ord.OrderType ||
					currentDeleteOrder[ownerID].OrderState != ord.OrderState {
						break
					}
			}

			if !isElementInList(ackID, peersThatHaveConfirmedDelete[ownerID]) {
				peersThatHaveConfirmedDelete[ownerID] = append(peersThatHaveConfirmedDelete[ownerID], ackID)
			}
		}
		// Check if everyone has reached barrier state, for each order in map
		for _, peerID := range fullList {
			if containSameElements(fullList, peersThatHaveConfirmedDelete[peerID]) {
				ord, orderExist:= currentDeleteOrder[peerID]
				if !orderExist {
					peersThatHaveConfirmedDelete[peerID] = []string{}
					continue
				}

				allAgreeToDeleteOrder <- ord
				peersThatHaveConfirmedDelete[peerID] = make([]string, 0)
				delete(currentDeleteOrder, peerID)
			}
		}
	}
}

func unconfirmedRequestBarrierStateCounter(myID string, peerUpdateInUnconfirmedRequestBarrierStateCounter chan peers.PeerUpdate, iAmAtUnconfirmedRequestBarrier chan acknowledgeBarrier, allHaveUnconfirmedRequest chan string) {
	activePeersList := make([]string, 0)
	fullList := []string{}
	peersThatHaveUnconfirmedRequest := make(map[string][]string)
	//peersThatHaveUnconfirmedRequest[myID] = []string{}
	log.Println("Entered the unconfirmedRequestBarrierStateCounter!!!")

	for {
		select {
		case newPeerUpdateShallowCopy := <-peerUpdateInUnconfirmedRequestBarrierStateCounter:
			newPeerUpdate := peers.PeerUpdateClone(newPeerUpdateShallowCopy)
			activePeersList = newPeerUpdate.Peers

			fullList = slices.Clone(activePeersList)
			fullList = append(fullList, myID)

			for key := range peersThatHaveUnconfirmedRequest {
				if !isElementInList(key, fullList) {
					delete(peersThatHaveUnconfirmedRequest, key)
				}
			}

			for _, peerID := range fullList {
				if !isKeyInMap(peerID, peersThatHaveUnconfirmedRequest) {
					peersThatHaveUnconfirmedRequest[peerID] = []string{}
				}
			}

			log.Println("UUUUUHM, DO I HAVE THE RIGHT UNCONFIRMED REQUEST LIST????? ", fullList)
			// activePeersList and the peersThatHaveConfirmedRequest map keys should always have the same elements in them

		case acknowledgement := <-iAmAtUnconfirmedRequestBarrier:
			log.Println("WTF????????? SHOULD UNCONFIRMED PRINT")
			ownerID := acknowledgement.ownerID
			ackID := acknowledgement.ackID

			// ignore acks for peers we don't know about
			if !isElementInList(ownerID, fullList) || !isElementInList(ackID, fullList) {
				break
			}

			if !isKeyInMap(ownerID, peersThatHaveUnconfirmedRequest) {
				log.Println("Barrier state unconf. req. counter does not maintain its peerlist correctly...")
				break
			}

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
			if !isKeyInMap(peerID, peersThatHaveUnconfirmedRequest) {
				//peersThatHaveUnconfirmedDelete[peerID] = []string{}
				log.Println("Barrier state unconf. req. counter does not maintain its peerlist correctly...")
				continue
			}

			if containSameElements(fullList, peersThatHaveUnconfirmedRequest[peerID]) {
				allHaveUnconfirmedRequest <- peerID
				peersThatHaveUnconfirmedRequest[peerID] = make([]string, 0)
				log.Println("WOW, AN ACTUAL UNCONFIRMED REQUEST! ", peerID, " owns it.")
			}
		}
	}
}

func unconfirmedDeletionBarrierStateCounter(myID string, peerUpdateInUnconfirmedDeletionBarrierStateCounter chan peers.PeerUpdate, iAmAtUnconfirmedDeleteBarrier chan acknowledgeBarrier, allHaveUnconfirmedDeletion chan string) {
	activePeersList := []string{}
	fullList := []string{}

	peersThatHaveUnconfirmedDelete := make(map[string][]string)
	//peersThatHaveUnconfirmedDelete[myID] = []string{}

	log.Println("Entered the unconfirmedDeletionBarrierStateCounter!!!")

	for {
		select {
		case newPeerUpdateShallowCopy := <-peerUpdateInUnconfirmedDeletionBarrierStateCounter:
			newPeerUpdate := peers.PeerUpdateClone(newPeerUpdateShallowCopy)
			activePeersList = newPeerUpdate.Peers

			fullList = slices.Clone(activePeersList)
			fullList = append(fullList, myID)

			for key := range peersThatHaveUnconfirmedDelete {
				if !isElementInList(key, fullList) {
					delete(peersThatHaveUnconfirmedDelete, key)
				}
			}

			for _, peerID := range fullList {
				if !isKeyInMap(peerID, peersThatHaveUnconfirmedDelete) {
					peersThatHaveUnconfirmedDelete[peerID] = []string{}
				}
			}

			log.Println("UUUUUHM, DO I HAVE THE RIGHT UNCONFIRMED DELETE LIST????? ", fullList)
			// activePeersList and the peersThatHaveConfirmedRequest map keys should always have the same elements in them

		case acknowledgement := <-iAmAtUnconfirmedDeleteBarrier:
			ownerID := acknowledgement.ownerID
			ackID := acknowledgement.ackID

			// ignore acks for peers we don't know about
			if !isElementInList(ownerID, fullList) || !isElementInList(ackID, fullList) {
				break
			}

			if !isKeyInMap(ownerID, peersThatHaveUnconfirmedDelete) {
				log.Println("Barrier state unconf. del. counter does not maintain its peerlist correctly...")
				break
			}

			if !(isElementInList(ackID, peersThatHaveUnconfirmedDelete[ownerID])) {
				peersThatHaveUnconfirmedDelete[ownerID] = append(peersThatHaveUnconfirmedDelete[ownerID], ackID)
			}
		}
		// Check if everyone has reached barrier state, for each order in map
		for _, peerID := range fullList {
			if !isKeyInMap(peerID, peersThatHaveUnconfirmedDelete) {
				//peersThatHaveUnconfirmedDelete[peerID] = []string{}
				log.Println("Barrier state unconf. del. counter does not maintain its peerlist correctly...")
				continue
			}

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

func updateOrderStateInMap(incomingMap map[string]order.Order, key string, state order.SyncOrderState) {
	currentOrder := incomingMap[key]
	incomingMap[key] = order.NewOrder(key, currentOrder.OrderFloor, currentOrder.OrderType, state)
}
