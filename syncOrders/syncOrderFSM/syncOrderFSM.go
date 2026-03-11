package syncOrderFSM

import (
	//"elevatorControl/elevator"
	"log"
	"networkDriver/peers"
	"slices"
	"syncOrders/order"
	"elevatorControl/elevator"
)

// ! Hope to change this struct or remove it

type barrierKey struct {
	ownerID string
	floor 	int
	button 	int
	state 	order.SyncOrderState
}

type acknowledgeBarrier struct {
	key 	barrierKey
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

	lastConfirmedOrderMap := make(map[string]order.Order)

	// TODO: Random channels, sort later

	iAmAtRequestBarrier := make(chan acknowledgeBarrier, 64)
	iAmAtDeleteBarrier := make(chan acknowledgeBarrier, 64)
	iAmAtUnconfirmedRequestBarrier := make(chan acknowledgeBarrier, 64)
	iAmAtUnconfirmedDeleteBarrier := make(chan acknowledgeBarrier, 64)

	allAgreeToAddOrder := make(chan order.Order)
	allAgreeToDeleteOrder := make(chan order.Order)
	allHaveUnconfirmedRequest := make(chan order.Order)
	allHaveUnconfirmedDeletion := make(chan order.Order)

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
		// in use for newOrderStateTransition to only apply when there is a change
		changed := false

		select {
		case newPeerUpdateShallowCopy := <-peerUpdateInSyncOrdersFSM:
			newPeerUpdate := peers.PeerUpdateClone(newPeerUpdateShallowCopy)
			//activePeersList = newPeerUpdate.Peers
			// Update localOrderToSyncMap
			if newPeerUpdate.New != "" {
				if !(isKeyInMap(newPeerUpdate.New, localOrderToSyncMap)) {
					localOrderToSyncMap[newPeerUpdate.New] = order.NewUnknownOrder(newPeerUpdate.New)
					changed = true
				}
			}
			for _, peerID := range newPeerUpdate.Lost {
				if isKeyInMap(peerID, localOrderToSyncMap) {
					delete(localOrderToSyncMap, peerID)
					delete(lastConfirmedOrderMap, peerID)
					changed = true
				} 
			}
			// activePeersList and the localOrderToSyncMap map keys should always have the same elements in them

		case <-networkDisconnect:
			for ID, _ := range localOrderToSyncMap {
				updateOrderStateInMap(localOrderToSyncMap, ID, order.SOS_UNKNOWN)
			}
			clear(lastConfirmedOrderMap)
			clearAllConfirmedOrders <- true
			changed = true

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
						//localOrder = incomingOrderToSync
						continue
					}

					switch incomingOrderToSync.OrderState {
					case order.SOS_NONE:

						rememberedOrder, orderExists := lastConfirmedOrderMap[key_ID]
						if !orderExists {
							break
						}

						switch localOrder.OrderState {
						case order.SOS_CONFIRMED_REQUEST:
							/*
							if rememberedOrder.OrderState != order.SOS_CONFIRMED_REQUEST {
								break
							}*/
							confirmedRequest <- rememberedOrder
							delete(lastConfirmedOrderMap, key_ID)
							updateOrderStateInMap(localOrderToSyncMap, key_ID, order.SOS_NONE)
							changed = true

						case order.SOS_CONFIRMED_DELETION:
							/*
							if rememberedOrder.OrderState != order.SOS_CONFIRMED_DELETION {
								break
							}*/
							confirmedDeletion <- localOrder
							delete(lastConfirmedOrderMap, key_ID)
							updateOrderStateInMap(localOrderToSyncMap, key_ID, order.SOS_NONE)
							changed = true

						default:
							log.Println(incomingID, " told us they have no orders, and we dont care.")
						}

					case order.SOS_UNCONFIRMED_REQUEST:

						iAmAtUnconfirmedRequestBarrier <- acknowledgeBarrier{
							key: 	makeBarrierKey(incomingOrderToSync), 
							ackID: 	incomingID,
							ord: 	incomingOrderToSync,
						}

						log.Printf("Peer %s sees UNCONFIRMED for owner %s from sender %s\n", myID, key_ID, incomingID)

						switch localOrder.OrderState {
						case order.SOS_NONE, order.SOS_UNKNOWN:
							if localOrder != incomingOrderToSync {
								localOrderToSyncMap[key_ID] = incomingOrderToSync
								localOrder = localOrderToSyncMap[key_ID]
								changed = true
							}

							// Need a second barrier, also for the unconfirmation......
							log.Printf("Peer %s sending UNCONFIRMED ACK for owner %s from sender %s\n", myID, key_ID, incomingID)
							
							iAmAtUnconfirmedRequestBarrier <- acknowledgeBarrier{
								key: 	makeBarrierKey(incomingOrderToSync), 
								ackID: 	myID,
								ord: 	incomingOrderToSync,
							}

							log.Println(incomingID, " told us they have a request, and we believe them!")

						case order.SOS_UNCONFIRMED_REQUEST:

							if localOrder != incomingOrderToSync {
								localOrderToSyncMap[key_ID] = incomingOrderToSync
								localOrder = localOrderToSyncMap[key_ID]
								changed = true
							}

						default:

						}

					case order.SOS_UNCONFIRMED_DELETION:

						iAmAtUnconfirmedDeleteBarrier <- acknowledgeBarrier{
							key: 	makeBarrierKey(incomingOrderToSync), 
							ackID: 	incomingID,
							ord: 	incomingOrderToSync,
						}

						switch localOrder.OrderState {
						case order.SOS_CONFIRMED_REQUEST, order.SOS_NONE, order.SOS_UNKNOWN:
							if localOrder != incomingOrderToSync {
								localOrderToSyncMap[key_ID] = incomingOrderToSync
								localOrder = localOrderToSyncMap[key_ID]
								changed = true
							}

							log.Printf("Peer %s sending UNCONFIRMED DELETION ACK for owner %s from sender %s\n", myID, key_ID, incomingID)
							iAmAtUnconfirmedDeleteBarrier <- acknowledgeBarrier{
								key: 	makeBarrierKey(incomingOrderToSync),
								ackID: 	myID,
								ord: 	incomingOrderToSync,
							}

						case order.SOS_UNCONFIRMED_DELETION:
							if localOrder != incomingOrderToSync {
								localOrderToSyncMap[key_ID] = incomingOrderToSync
								localOrder = localOrderToSyncMap[key_ID]
								changed = true
							}

						default:
						}

					case order.SOS_CONFIRMED_REQUEST:

						//lastConfirmedOrderMap[key_ID] = incomingOrderToSync

						iAmAtRequestBarrier <- acknowledgeBarrier{
								key: 	makeBarrierKey(incomingOrderToSync), 
								ackID: 	incomingID,
								ord: 	incomingOrderToSync,
						}

						switch localOrder.OrderState {
						case order.SOS_NONE,  order.SOS_UNKNOWN, order.SOS_UNCONFIRMED_REQUEST, order.SOS_CONFIRMED_REQUEST:
							if localOrder != incomingOrderToSync {
								localOrderToSyncMap[key_ID] = incomingOrderToSync
								localOrder = localOrderToSyncMap[key_ID]
								changed = true
							}
							iAmAtRequestBarrier <- acknowledgeBarrier{
							key: 	makeBarrierKey(incomingOrderToSync), 
							ackID: 	myID,
							ord: 	incomingOrderToSync,
							}
						}

					case order.SOS_CONFIRMED_DELETION:

						//lastConfirmedOrderMap[key_ID] = incomingOrderToSync

						iAmAtDeleteBarrier <- acknowledgeBarrier{
							key: 	makeBarrierKey(incomingOrderToSync), 
							ackID: 	incomingID,
							ord: 	incomingOrderToSync,
						}

						switch localOrder.OrderState {
						case order.SOS_UNKNOWN, order.SOS_CONFIRMED_REQUEST, order.SOS_UNCONFIRMED_DELETION, order.SOS_CONFIRMED_DELETION, order.SOS_NONE:
							if localOrder != incomingOrderToSync {
								localOrderToSyncMap[key_ID] = incomingOrderToSync
								localOrder = localOrderToSyncMap[key_ID]
								changed = true
							}

							iAmAtDeleteBarrier <- acknowledgeBarrier{
							key: 	makeBarrierKey(incomingOrderToSync), 
							ackID: 	myID,
							ord: 	incomingOrderToSync,
							}
						default:

						}
					}
				}

			case orderToAdd := <-allAgreeToAddOrder:
				log.Printf("FSM adding confirmedRequest %+v\n", orderToAdd)
				if orderToAdd.OrderState != order.SOS_CONFIRMED_REQUEST {
					log.Println("WARNING: FSM attempt to add a non_CONFIRMED_REQUEST order to confirmed request list:", orderToAdd)
					break
				}
				confirmedRequest <- orderToAdd
				lastConfirmedOrderMap[orderToAdd.PeerID] = orderToAdd
				updateOrderStateInMap(localOrderToSyncMap, orderToAdd.PeerID, order.SOS_NONE)
				changed = true
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
				lastConfirmedOrderMap[orderToDelete.PeerID] = orderToDelete
				updateOrderStateInMap(localOrderToSyncMap, orderToDelete.PeerID, order.SOS_NONE)
				changed = true

			case orderToPromote := <-allHaveUnconfirmedRequest:
				_, peerIsInMap := localOrderToSyncMap[orderToPromote.PeerID]
				if !peerIsInMap {
					break
				}

				updateOrderStateInMap(localOrderToSyncMap, orderToPromote.PeerID, order.SOS_CONFIRMED_REQUEST)
				iAmAtRequestBarrier <- acknowledgeBarrier{
					key: 	makeBarrierKey(localOrderToSyncMap[orderToPromote.PeerID]), 
					ackID: 	myID,
					ord: 	localOrderToSyncMap[orderToPromote.PeerID],
				}
				log.Println(orderToPromote.PeerID, "has an order that is unconfirmed for everyone, so we make the executive decision to move on!")
				changed = true

			case orderToPromote := <-allHaveUnconfirmedDeletion:
				_, peerIsInMap := localOrderToSyncMap[orderToPromote.PeerID]
				if !peerIsInMap {
					break
				}

				updateOrderStateInMap(localOrderToSyncMap, orderToPromote.PeerID, order.SOS_CONFIRMED_DELETION)
				iAmAtDeleteBarrier <- acknowledgeBarrier{
					key: 	makeBarrierKey(localOrderToSyncMap[orderToPromote.PeerID]), 
					ackID: 	myID,
					ord: 	localOrderToSyncMap[orderToPromote.PeerID],
				}
				changed = true
			}	

		}
		if changed {
			newOrderStateTransition <- MapCopy(localOrderToSyncMap) // Deep copy of map to be sent
		}

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
	peersThatHaveConfirmedRequest := make(map[barrierKey][]string)
	//peersThatHaveConfirmedRequest[myID] = []string{}
	log.Println("Entered the requestBarrierStateCounter!!!")

	for {
		select {
		case newPeerUpdateShallowCopy := <-peerUpdateInRequestBarrierStateCounter:
			newPeerUpdate := peers.PeerUpdateClone(newPeerUpdateShallowCopy)
			activePeersList = newPeerUpdate.Peers

			fullList = slices.Clone(activePeersList)
			fullList = append(fullList, myID)

			for key := range peersThatHaveConfirmedRequest {
				if !isElementInList(key.ownerID, fullList) {
					delete(peersThatHaveConfirmedRequest, key)
				}
			}

			log.Println("UUUUUHM, DO I HAVE THE RIGHT CONFIRMED LIST????? ", fullList)
			// activePeersList and the peersThatHaveConfirmedRequest map keys should always have the same elements in them

		case acknowledgement := <-iAmAtRequestBarrier:
			log.Println("WTF????????? SHOULD PRINT")
			key := acknowledgement.key
			ackID := acknowledgement.ackID
			ord := acknowledgement.ord

			// ignore acks for peers we don't know about
			if !isElementInList(key.ownerID, fullList) || !isElementInList(ackID, fullList) {
				break
			}

			if key != makeBarrierKey(ord) {
				log.Println("Mismatch of key/order pair for barrier ack")
				break
			}

			if !isElementInList(ackID, peersThatHaveConfirmedRequest[key]) {
				peersThatHaveConfirmedRequest[key] = append(peersThatHaveConfirmedRequest[key], ackID)
			}

		log.Println(
			"Barrier check:",
			"fullList:", fullList,
			"acks:", peersThatHaveConfirmedRequest,
		)
		// Check if everyone has reached barrier state, for each order in map
		for key, ackList := range peersThatHaveConfirmedRequest {
			if containSameElements(fullList, ackList) {
				ord := order.NewOrder(key.ownerID, key.floor, elevator.Button(key.button), key.state)
				allAgreeToAddOrder <- ord
				delete(peersThatHaveConfirmedRequest, key)
				log.Println("WOW, AN ACTUAL CONFIRMED ORDER! ", key.ownerID, " owns it.")
				}
			}
		}
	}
}


func deletionBarrierStateCounter(myID string, peerUpdateInDeletionBarrierStateCounter chan peers.PeerUpdate, iAmAtDeleteBarrier chan acknowledgeBarrier, allAgreeToDeleteOrder chan order.Order) {
	activePeersList := make([]string, 0)
	fullList := []string{}
	peersThatHaveConfirmedDelete := make(map[barrierKey][]string)
	//peersThatHaveConfirmedDelete[myID] = []string{}
	log.Println("Entered the deletionBarrierStateCounter!!!")

	for {
		select {
		case newPeerUpdateShallowCopy := <-peerUpdateInDeletionBarrierStateCounter:
			newPeerUpdate := peers.PeerUpdateClone(newPeerUpdateShallowCopy)
			activePeersList = newPeerUpdate.Peers

			fullList = slices.Clone(activePeersList)
			fullList = append(fullList, myID)

			for key := range peersThatHaveConfirmedDelete {
				if !isElementInList(key.ownerID, fullList) {
					delete(peersThatHaveConfirmedDelete, key)
				}
			}

			log.Println("UUUUUHM, DO I HAVE THE RIGHT CONFIRMED DELETE LIST????? ", fullList)
			// activePeersList and the peersThatHaveConfirmedDelete map keys should always have the same elements in them

		case acknowledgement := <-iAmAtDeleteBarrier:
			key := acknowledgement.key
			ackID := acknowledgement.ackID
			ord := acknowledgement.ord

			// ignore acks for peers we don't know about
			if !isElementInList(key.ownerID, fullList) || !isElementInList(ackID, fullList) {
				break
			}

			if key != makeBarrierKey(ord) {
				log.Println("Mismatch of key/order pair for delete barrier ack")
				break
			}

			if !isElementInList(ackID, peersThatHaveConfirmedDelete[key]) {
				peersThatHaveConfirmedDelete[key] = append(peersThatHaveConfirmedDelete[key], ackID)
			}
		}
		// Check if everyone has reached barrier state, for each order in map
		for key, ackList := range peersThatHaveConfirmedDelete {
			if containSameElements(fullList, ackList) {
				ord := order.NewOrder(key.ownerID, key.floor, elevator.Button(key.button), key.state)
				allAgreeToDeleteOrder <- ord
				delete(peersThatHaveConfirmedDelete, key)
				log.Println("WOW, AN ACTUAL CONFIRMED DELETE! ", key.ownerID, " owns it.")
			}
		}
	}
}


func unconfirmedRequestBarrierStateCounter(myID string, peerUpdateInUnconfirmedRequestBarrierStateCounter chan peers.PeerUpdate, iAmAtUnconfirmedRequestBarrier chan acknowledgeBarrier, allHaveUnconfirmedRequest chan order.Order) {
	activePeersList := make([]string, 0)
	fullList := []string{}
	peersThatHaveUnconfirmedRequest := make(map[barrierKey][]string)
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
				if !isElementInList(key.ownerID, fullList) {
					delete(peersThatHaveUnconfirmedRequest, key)
				}
			}

			log.Println("UUUUUHM, DO I HAVE THE RIGHT UNCONFIRMED REQUEST LIST????? ", fullList)
			// activePeersList and the peersThatHaveConfirmedRequest map keys should always have the same elements in them

		case acknowledgement := <-iAmAtUnconfirmedRequestBarrier:
			log.Println("WTF????????? SHOULD UNCONFIRMED PRINT")
			key := acknowledgement.key
			ackID := acknowledgement.ackID
			ord := acknowledgement.ord

			// ignore acks for peers we don't know about
			if !isElementInList(key.ownerID, fullList) || !isElementInList(ackID, fullList) {
				break
			}

			if key != makeBarrierKey(ord) {
				log.Println("Mismatch of key/order pair for unconfirmed request barrier ack")
				break
			}

			if !(isElementInList(ackID, peersThatHaveUnconfirmedRequest[key])) {
				peersThatHaveUnconfirmedRequest[key] = append(peersThatHaveUnconfirmedRequest[key], ackID)
			}
			log.Println(ackID, " acknowledged that ", key.ownerID, " has an unconfirmed order! Full acklist: ", peersThatHaveUnconfirmedRequest[key])
		}

		log.Println(
			"Barrier check:",
			"fullList:", fullList,
			"acks:", peersThatHaveUnconfirmedRequest,
		)

		// Check if everyone has reached barrier state, for each order in map
		for key, ackList := range peersThatHaveUnconfirmedRequest {
			if containSameElements(fullList, ackList) {
				ord := order.NewOrder(key.ownerID, key.floor, elevator.Button(key.button), key.state)
				allHaveUnconfirmedRequest <- ord
				delete(peersThatHaveUnconfirmedRequest, key)
				log.Println("WOW, AN ACTUAL UNCONFIRMED REQUEST! ", key.ownerID, " owns it.")
			}
		}
	}
}

func unconfirmedDeletionBarrierStateCounter(myID string, peerUpdateInUnconfirmedDeletionBarrierStateCounter chan peers.PeerUpdate, iAmAtUnconfirmedDeleteBarrier chan acknowledgeBarrier, allHaveUnconfirmedDeletion chan order.Order) {
	activePeersList := []string{}
	fullList := []string{}

	peersThatHaveUnconfirmedDelete := make(map[barrierKey][]string)
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
				if !isElementInList(key.ownerID, fullList) {
					delete(peersThatHaveUnconfirmedDelete, key)
				}
			}

			log.Println("UUUUUHM, DO I HAVE THE RIGHT UNCONFIRMED DELETE LIST????? ", fullList)
			// activePeersList and the peersThatHaveUnconfirmedDelete map keys should always have the same elements in them

		case acknowledgement := <-iAmAtUnconfirmedDeleteBarrier:
			key := acknowledgement.key
			ackID := acknowledgement.ackID
			ord := acknowledgement.ord

			// ignore acks for peers we don't know about
			if !isElementInList(key.ownerID, fullList) || !isElementInList(ackID, fullList) {
				break
			}

			if key != makeBarrierKey(ord) {
				log.Println("Mismatch of key/order pair for unconfirmed delete barrier ack")
				break
			}

			if !(isElementInList(ackID, peersThatHaveUnconfirmedDelete[key])) {
				peersThatHaveUnconfirmedDelete[key] = append(peersThatHaveUnconfirmedDelete[key], ackID)
			}

		}
		// Check if everyone has reached barrier state, for each order in map
		for key, ackList := range peersThatHaveUnconfirmedDelete {
			if containSameElements(fullList, ackList) {
				ord := order.NewOrder(key.ownerID, key.floor, elevator.Button(key.button), key.state)
				allHaveUnconfirmedDeletion <- ord
				delete(peersThatHaveUnconfirmedDelete, key)
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

func makeBarrierKey(ord order.Order) barrierKey {
	return barrierKey{
		ownerID: ord.PeerID,
		floor: ord.OrderFloor,
		button: int(ord.OrderType),
		state: ord.OrderState,
	}
}