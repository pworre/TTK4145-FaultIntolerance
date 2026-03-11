package syncOrderFSM

import (
	//"elevatorControl/elevator"
	"elevatorControl/elevator"
	"fmt"
	"log"
	"networkDriver/peers"
	"slices"
	"syncOrders/order"
)

// ! Hope to change this struct or remove it

type acknowledgeBarrier struct {
	key   order.BarrierKey
	ackID string
	ord   order.Order
}

// ! End

// Finite state machine loop

// TODO: Assert that both the fsm localOrderToSyncMap and the syncOrders orderToSyncMap have the same members!
// TODO: This could probably warrant combining them both into one long file,
// TODO: but that would make the single file incredibly long and with way too many responsibilities,
// TODO: which is probably bad code quality...
// TODO: We will see what we have to to, but it seems paramount that the maps have the same keys

func StateMachineLoop(myID string, newOrderStateTransition chan map[string]order.Order, newOrderStateReceival chan order.OrderStateMessage, confirmedRequest chan order.Order, confirmedDeletion chan order.Order, networkDisconnect chan bool, clearAllConfirmedOrders chan bool, peerUpdateInSyncOrdersFSM chan peers.PeerUpdate, peerUpdateInRequestBarrierStateCounter chan peers.PeerUpdate, peerUpdateInDeletionBarrierStateCounter chan peers.PeerUpdate, peerUpdateInUnconfirmedRequestBarrierStateCounter chan peers.PeerUpdate, peerUpdateInUnconfirmedDeletionBarrierStateCounter chan peers.PeerUpdate, resetAckListReqBar chan order.BarrierKey, resetAckListDelBar chan order.BarrierKey, resetAckListUncReqBar chan order.BarrierKey, resetAckListUncDelBar chan order.BarrierKey) {

	//activePeersList := make([]string, 0) // Most likely not needed???

	localOrderToSyncMap := make(map[string]order.Order)
	localOrderToSyncMap[myID] = order.NewEmptyOrder(myID)
	lastSentMap := MapCopy(localOrderToSyncMap)
	activePeersList := make([]string, 0)

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

	go requestBarrierStateCounter(myID, peerUpdateInRequestBarrierStateCounter, iAmAtRequestBarrier, allAgreeToAddOrder, resetAckListReqBar)
	go deletionBarrierStateCounter(myID, peerUpdateInDeletionBarrierStateCounter, iAmAtDeleteBarrier, allAgreeToDeleteOrder, resetAckListDelBar)
	go unconfirmedRequestBarrierStateCounter(myID, peerUpdateInUnconfirmedRequestBarrierStateCounter, iAmAtUnconfirmedRequestBarrier, allHaveUnconfirmedRequest, resetAckListUncReqBar)
	go unconfirmedDeletionBarrierStateCounter(myID, peerUpdateInUnconfirmedDeletionBarrierStateCounter, iAmAtUnconfirmedDeleteBarrier, allHaveUnconfirmedDeletion, resetAckListUncDelBar)

	// ! VERY IMPORTANT ! Go over and see every time that a map or slice is sent on a channel!!! In both cases they must be copied...

	// ! VERY IMPORTANT ! When new peer initializes and joins, it sets itself as none and everyone else as unknown
	// ! Is this handled by default, or must we explicitly enforce this?

	// Probably done // TODO: Complete implementation of the barrier state counting, spawn correct number of threads in syncOrders
	// Probably done // TODO: For a peerUpdate, we need to update the localOrderToSyncMap
	// Probably done // TODO: Might be necessary to periodically send the map, not just with changes... then we also need a currentMessageToSend, most likely

	for {
		// in use for newOrderStateTransition to only apply when there is a change
		//changed := false

		select {
		case newPeerUpdateShallowCopy := <-peerUpdateInSyncOrdersFSM:
			newPeerUpdate := peers.PeerUpdateClone(newPeerUpdateShallowCopy)
			activePeersList = newPeerUpdate.Peers

			fullList := append([]string{}, activePeersList...)
			fullList = append(fullList, myID)
			localOrderToSyncMap = normalizeOrderMap(order.MapClone(localOrderToSyncMap), fullList)

			// activePeersList and the localOrderToSyncMap map keys should always have the same elements in them

		case <-networkDisconnect:
			for ID, _ := range localOrderToSyncMap {
				updateOrderStateInMap(localOrderToSyncMap, ID, order.SOS_UNKNOWN)
			}
			clear(lastConfirmedOrderMap)
			clearAllConfirmedOrders <- true

			//<-waitForReconnection // Blocks and does nothing until we are reconnected or restart

			// ! For the reason stated above in the big TODO,
			// ! we need to prioritize peerUpdates and networkDisconnects to ensure that the maps have the same members

		default:
			select {
			case incomingOrderToSyncMessage := <-newOrderStateReceival:
				fullList := append([]string{}, activePeersList...)
				fullList = append(fullList, myID)
				incomingOrderToSyncMap := normalizeOrderMap(MapCopy(incomingOrderToSyncMessage.OrderToSyncMap), fullList)
				log.Println("Entering the orderSync state machine. Incoming map: ", incomingOrderToSyncMap)
				incomingID := incomingOrderToSyncMessage.TransmittedPeerID

				for key_ID, incomingOrderToSync := range incomingOrderToSyncMap {
					localOrder, localExists := localOrderToSyncMap[key_ID]

					if !localExists {
						//localOrder = incomingOrderToSync
						continue
					}

					if localOrder.OrderState == order.SOS_UNKNOWN {
						localOrderToSyncMap[key_ID] = incomingOrderToSync
					}
					log.Printf("Incoming order from sender=%s sig=%s", incomingID, orderSignature(incomingOrderToSync))

					log.Printf(
						"Before processing owner=%s incoming=%s local=%s sender=%s",
						key_ID,
						orderSignature(incomingOrderToSync),
						orderSignature(localOrderToSyncMap[key_ID]),
						incomingID,
					)

					switch incomingOrderToSync.OrderState {
					case order.SOS_NONE:
						log.Printf("Entering SOS_NONE: %s from sender=%s", orderSignature(incomingOrderToSync), incomingID)

						rememberedOrder, orderExists := lastConfirmedOrderMap[key_ID]
						if !orderExists {
							break
						}

						switch localOrder.OrderState {
						case order.SOS_CONFIRMED_REQUEST:
							if rememberedOrder.OrderState == order.SOS_CONFIRMED_REQUEST {
								confirmedRequest <- localOrder
								updateOrderStateInMap(localOrderToSyncMap, key_ID, order.SOS_NONE)
								//changed = true
							}

						case order.SOS_CONFIRMED_DELETION:
							if rememberedOrder.OrderState == order.SOS_CONFIRMED_DELETION {
								confirmedDeletion <- localOrder
								updateOrderStateInMap(localOrderToSyncMap, key_ID, order.SOS_NONE)
								//changed = true
							}

						default:
							log.Println(incomingID, " told us they have no orders, and we dont care.")
						}

					case order.SOS_UNCONFIRMED_REQUEST:
						log.Printf("Entering UNCONFIRMED_REQUEST: %s from sender=%s", orderSignature(incomingOrderToSync), incomingID)

						log.Printf("Unconfirmed request barrier ACK owner=%s ackID=%s sig=%s", key_ID, incomingID, orderSignature(incomingOrderToSync))
						iAmAtUnconfirmedRequestBarrier <- acknowledgeBarrier{
							key:   order.NewBarrierKey(incomingOrderToSync),
							ackID: incomingID,
							ord:   incomingOrderToSync,
						}

						log.Printf("Peer %s sees UNCONFIRMED for owner %s from sender %s\n", myID, key_ID, incomingID)

						switch localOrder.OrderState {
						case order.SOS_NONE, order.SOS_UNKNOWN:
							if localOrder != incomingOrderToSync {
								localOrderToSyncMap[key_ID] = incomingOrderToSync
								localOrder = localOrderToSyncMap[key_ID]
								//changed = true
							}

							// Need a second barrier, also for the unconfirmation......
							log.Printf("Peer %s sending UNCONFIRMED ACK for owner %s from sender %s\n", myID, key_ID, incomingID)

							iAmAtUnconfirmedRequestBarrier <- acknowledgeBarrier{
								key:   order.NewBarrierKey(incomingOrderToSync),
								ackID: myID,
								ord:   incomingOrderToSync,
							}

							log.Println(incomingID, " told us they have a request, and we believe them!")

						case order.SOS_UNCONFIRMED_REQUEST:

							if localOrder != incomingOrderToSync {
								localOrderToSyncMap[key_ID] = incomingOrderToSync
								localOrder = localOrderToSyncMap[key_ID]
								//changed = true
							}

						default:

						}

					case order.SOS_UNCONFIRMED_DELETION:
						log.Printf("Entering UNCONFIRMED_DELETION: %s from sender=%s", orderSignature(incomingOrderToSync), incomingID)

						log.Printf("Unconfirmed delete barrier ACK owner=%s ackID=%s sig=%s", key_ID, incomingID, orderSignature(incomingOrderToSync))
						iAmAtUnconfirmedDeleteBarrier <- acknowledgeBarrier{
							key:   order.NewBarrierKey(incomingOrderToSync),
							ackID: incomingID,
							ord:   incomingOrderToSync,
						}

						switch localOrder.OrderState {
						case order.SOS_CONFIRMED_REQUEST, order.SOS_NONE, order.SOS_UNKNOWN:
							if localOrder != incomingOrderToSync {
								localOrderToSyncMap[key_ID] = incomingOrderToSync
								localOrder = localOrderToSyncMap[key_ID]
								//changed = true
							}

							log.Printf("Peer %s sending UNCONFIRMED DELETION ACK for owner %s from sender %s\n", myID, key_ID, incomingID)
							iAmAtUnconfirmedDeleteBarrier <- acknowledgeBarrier{
								key:   order.NewBarrierKey(incomingOrderToSync),
								ackID: myID,
								ord:   incomingOrderToSync,
							}

						case order.SOS_UNCONFIRMED_DELETION:
							if localOrder != incomingOrderToSync {
								localOrderToSyncMap[key_ID] = incomingOrderToSync
								localOrder = localOrderToSyncMap[key_ID]
								//changed = true
							}

						default:
						}

					case order.SOS_CONFIRMED_REQUEST:
						log.Printf("Entering CONFIRMED_REQUEST: %s from sender=%s", orderSignature(incomingOrderToSync), incomingID)

						//incomingConfirmedRequest(incomingOrderToSync.PeerID)
						log.Printf("Confirmed request barrier ACK owner=%s ackID=%s sig=%s", key_ID, incomingID, orderSignature(incomingOrderToSync))

						//lastConfirmedOrderMap[key_ID] = incomingOrderToSync

						iAmAtRequestBarrier <- acknowledgeBarrier{
							key:   order.NewBarrierKey(incomingOrderToSync),
							ackID: incomingID,
							ord:   incomingOrderToSync,
						}

						switch localOrder.OrderState {
						case order.SOS_NONE, order.SOS_UNKNOWN, order.SOS_UNCONFIRMED_REQUEST, order.SOS_CONFIRMED_REQUEST:
							if localOrder != incomingOrderToSync {
								localOrderToSyncMap[key_ID] = incomingOrderToSync
								localOrder = localOrderToSyncMap[key_ID]
								//changed = true
							}
							iAmAtRequestBarrier <- acknowledgeBarrier{
								key:   order.NewBarrierKey(incomingOrderToSync),
								ackID: myID,
								ord:   incomingOrderToSync,
							}
						}

					case order.SOS_CONFIRMED_DELETION:
						log.Printf("Entering CONFIRMED_DELETION: %s from sender=%s", orderSignature(incomingOrderToSync), incomingID)

						//incomingConfirmedDeletion(incomingOrderToSync.PeerID)
						log.Printf("Confirmed delete barrier ACK owner=%s ackID=%s sig=%s", key_ID, incomingID, orderSignature(incomingOrderToSync))

						//lastConfirmedOrderMap[key_ID] = incomingOrderToSync

						iAmAtDeleteBarrier <- acknowledgeBarrier{
							key:   order.NewBarrierKey(incomingOrderToSync),
							ackID: incomingID,
							ord:   incomingOrderToSync,
						}

						switch localOrder.OrderState {
						case order.SOS_UNKNOWN, order.SOS_CONFIRMED_REQUEST, order.SOS_UNCONFIRMED_DELETION, order.SOS_CONFIRMED_DELETION, order.SOS_NONE:
							if localOrder != incomingOrderToSync {
								localOrderToSyncMap[key_ID] = incomingOrderToSync
								localOrder = localOrderToSyncMap[key_ID]
								//changed = true
							}

							iAmAtDeleteBarrier <- acknowledgeBarrier{
								key:   order.NewBarrierKey(incomingOrderToSync),
								ackID: myID,
								ord:   incomingOrderToSync,
							}
						default:

						}

					default:
						log.Printf("Entering default, aka UNKNOWN: %s from sender=%s", orderSignature(incomingOrderToSync), incomingID)
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
				//changed = true
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
				//changed = true

			case orderToPromote := <-allHaveUnconfirmedRequest:
				_, peerIsInMap := localOrderToSyncMap[orderToPromote.PeerID]
				if !peerIsInMap {
					break
				}

				updateOrderStateInMap(localOrderToSyncMap, orderToPromote.PeerID, order.SOS_CONFIRMED_REQUEST)
				iAmAtRequestBarrier <- acknowledgeBarrier{
					key:   order.NewBarrierKey(localOrderToSyncMap[orderToPromote.PeerID]),
					ackID: myID,
					ord:   localOrderToSyncMap[orderToPromote.PeerID],
				}
				log.Println(orderToPromote.PeerID, "has an order that is unconfirmed for everyone, so we make the executive decision to move on!")
				//changed = true

			case orderToPromote := <-allHaveUnconfirmedDeletion:
				_, peerIsInMap := localOrderToSyncMap[orderToPromote.PeerID]
				if !peerIsInMap {
					break
				}

				updateOrderStateInMap(localOrderToSyncMap, orderToPromote.PeerID, order.SOS_CONFIRMED_DELETION)
				iAmAtDeleteBarrier <- acknowledgeBarrier{
					key:   order.NewBarrierKey(localOrderToSyncMap[orderToPromote.PeerID]),
					ackID: myID,
					ord:   localOrderToSyncMap[orderToPromote.PeerID],
				}
				//changed = true
			}

		}
		if !mapsEqual(localOrderToSyncMap, lastSentMap) {
			if len(localOrderToSyncMap) != len(lastSentMap) {
				log.Println("localOrderToSyncMap changes length!! Presumably due to a peerUpdate")
			}
			for ID, newVal := range localOrderToSyncMap {
				oldVal, ok := lastSentMap[ID]
				if !ok {
					log.Printf("Peer number %s got added to the syncMap! This is the order there now: %s", ID, orderSignature(localOrderToSyncMap[ID]))
					continue
				}
				if oldVal != newVal {
					log.Printf("The order in the syncMap for peer number %s went from state %s to state %s", ID, orderSignature(lastSentMap[ID]), orderSignature(localOrderToSyncMap[ID]))
				}
			}
			for ID, _ := range lastSentMap {
				_, ok := localOrderToSyncMap[ID]
				if !ok {
					log.Printf("Peer number %s got removed from the syncMap! This was the order that got removed: %s", ID, orderSignature(lastSentMap[ID]))
					continue
				}
			}
			copyToSend := MapCopy(localOrderToSyncMap)
			newOrderStateTransition <- copyToSend
			lastSentMap = MapCopy(localOrderToSyncMap)
		}
		//newOrderStateTransition <- MapCopy(localOrderToSyncMap) // Deep copy of map to be sent

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
func requestBarrierStateCounter(myID string, peerUpdateInRequestBarrierStateCounter chan peers.PeerUpdate, iAmAtRequestBarrier chan acknowledgeBarrier, allAgreeToAddOrder chan order.Order, resetAckListReqBar chan order.BarrierKey) {
	activePeersList := make([]string, 0)
	fullList := []string{}
	peersThatHaveConfirmedRequest := make(map[order.BarrierKey][]string)
	//peersThatHaveConfirmedRequest[myID] = []string{}
	log.Println("Entered the requestBarrierStateCounter!!!")

	for {
		select {
		case peerID := <-resetAckListReqBar:
			peersThatHaveConfirmedRequest[peerID] = []string{}

		case newPeerUpdateShallowCopy := <-peerUpdateInRequestBarrierStateCounter:
			newPeerUpdate := peers.PeerUpdateClone(newPeerUpdateShallowCopy)
			activePeersList = newPeerUpdate.Peers

			fullList = slices.Clone(activePeersList)
			fullList = append(fullList, myID)

			for key := range peersThatHaveConfirmedRequest {
				if !isElementInList(key.OwnerID, fullList) {
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
			if !isElementInList(key.OwnerID, fullList) || !isElementInList(ackID, fullList) {
				break
			}

			if key != order.NewBarrierKey(ord) {
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
					ord := order.NewOrder(key.OwnerID, key.Floor, elevator.Button(key.Button), key.State)
					allAgreeToAddOrder <- ord
					delete(peersThatHaveConfirmedRequest, key)
					log.Println("WOW, AN ACTUAL CONFIRMED ORDER! ", key.OwnerID, " owns it.")
				}
			}
		}
	}
}

func deletionBarrierStateCounter(myID string, peerUpdateInDeletionBarrierStateCounter chan peers.PeerUpdate, iAmAtDeleteBarrier chan acknowledgeBarrier, allAgreeToDeleteOrder chan order.Order, resetAckListDelBar chan order.BarrierKey) {
	activePeersList := make([]string, 0)
	fullList := []string{}
	peersThatHaveConfirmedDelete := make(map[order.BarrierKey][]string)
	//peersThatHaveConfirmedDelete[myID] = []string{}
	log.Println("Entered the deletionBarrierStateCounter!!!")

	for {
		select {
		case peerID := <-resetAckListDelBar:
			peersThatHaveConfirmedDelete[peerID] = []string{}

		case newPeerUpdateShallowCopy := <-peerUpdateInDeletionBarrierStateCounter:
			newPeerUpdate := peers.PeerUpdateClone(newPeerUpdateShallowCopy)
			activePeersList = newPeerUpdate.Peers

			fullList = slices.Clone(activePeersList)
			fullList = append(fullList, myID)

			for key := range peersThatHaveConfirmedDelete {
				if !isElementInList(key.OwnerID, fullList) {
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
			if !isElementInList(key.OwnerID, fullList) || !isElementInList(ackID, fullList) {
				break
			}

			if key != order.NewBarrierKey(ord) {
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
				ord := order.NewOrder(key.OwnerID, key.Floor, elevator.Button(key.Button), key.State)
				allAgreeToDeleteOrder <- ord
				delete(peersThatHaveConfirmedDelete, key)
				log.Println("WOW, AN ACTUAL CONFIRMED DELETE! ", key.OwnerID, " owns it.")
			}
		}
	}
}

func unconfirmedRequestBarrierStateCounter(myID string, peerUpdateInUnconfirmedRequestBarrierStateCounter chan peers.PeerUpdate, iAmAtUnconfirmedRequestBarrier chan acknowledgeBarrier, allHaveUnconfirmedRequest chan order.Order, resetAckListUncReqBar chan order.BarrierKey) {
	activePeersList := make([]string, 0)
	fullList := []string{}
	peersThatHaveUnconfirmedRequest := make(map[order.BarrierKey][]string)
	//peersThatHaveUnconfirmedRequest[myID] = []string{}
	log.Println("Entered the unconfirmedRequestBarrierStateCounter!!!")

	for {
		select {
		case peerID := <-resetAckListUncReqBar:
			peersThatHaveUnconfirmedRequest[peerID] = []string{}

		case newPeerUpdateShallowCopy := <-peerUpdateInUnconfirmedRequestBarrierStateCounter:
			newPeerUpdate := peers.PeerUpdateClone(newPeerUpdateShallowCopy)
			activePeersList = newPeerUpdate.Peers

			fullList = slices.Clone(activePeersList)
			fullList = append(fullList, myID)

			for key := range peersThatHaveUnconfirmedRequest {
				if !isElementInList(key.OwnerID, fullList) {
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
			if !isElementInList(key.OwnerID, fullList) || !isElementInList(ackID, fullList) {
				break
			}

			if key != order.NewBarrierKey(ord) {
				log.Println("Mismatch of key/order pair for unconfirmed request barrier ack")
				break
			}

			if !(isElementInList(ackID, peersThatHaveUnconfirmedRequest[key])) {
				peersThatHaveUnconfirmedRequest[key] = append(peersThatHaveUnconfirmedRequest[key], ackID)
			}
			log.Println(ackID, " acknowledged that ", key.OwnerID, " has an unconfirmed order! Full acklist: ", peersThatHaveUnconfirmedRequest[key])
		}

		log.Println(
			"Barrier check:",
			"fullList:", fullList,
			"acks:", peersThatHaveUnconfirmedRequest,
		)

		// Check if everyone has reached barrier state, for each order in map
		for key, ackList := range peersThatHaveUnconfirmedRequest {
			if containSameElements(fullList, ackList) {
				ord := order.NewOrder(key.OwnerID, key.Floor, elevator.Button(key.Button), key.State)
				allHaveUnconfirmedRequest <- ord
				delete(peersThatHaveUnconfirmedRequest, key)
				log.Println("WOW, AN ACTUAL UNCONFIRMED REQUEST! ", key.OwnerID, " owns it.")
			}
		}
	}
}

func unconfirmedDeletionBarrierStateCounter(myID string, peerUpdateInUnconfirmedDeletionBarrierStateCounter chan peers.PeerUpdate, iAmAtUnconfirmedDeleteBarrier chan acknowledgeBarrier, allHaveUnconfirmedDeletion chan order.Order, resetAckListUncDelBar chan order.BarrierKey) {
	activePeersList := []string{}
	fullList := []string{}

	peersThatHaveUnconfirmedDelete := make(map[order.BarrierKey][]string)
	//peersThatHaveUnconfirmedDelete[myID] = []string{}

	log.Println("Entered the unconfirmedDeletionBarrierStateCounter!!!")

	for {
		select {
		case peerID := <-resetAckListUncDelBar:
			peersThatHaveUnconfirmedDelete[peerID] = []string{}

		case newPeerUpdateShallowCopy := <-peerUpdateInUnconfirmedDeletionBarrierStateCounter:
			newPeerUpdate := peers.PeerUpdateClone(newPeerUpdateShallowCopy)
			activePeersList = newPeerUpdate.Peers

			fullList = slices.Clone(activePeersList)
			fullList = append(fullList, myID)

			for key := range peersThatHaveUnconfirmedDelete {
				if !isElementInList(key.OwnerID, fullList) {
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
			if !isElementInList(key.OwnerID, fullList) || !isElementInList(ackID, fullList) {
				break
			}

			if key != order.NewBarrierKey(ord) {
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
				ord := order.NewOrder(key.OwnerID, key.Floor, elevator.Button(key.Button), key.State)
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
	_, exists := incomingMap[key]
	if !exists {
		log.Println("Dude wtf????????? Okay, this should never happen")
		return
	}

	switch state {
	case order.SOS_NONE:
		incomingMap[key] = order.NewEmptyOrder(key)
	case order.SOS_UNKNOWN:
		incomingMap[key] = order.NewUnknownOrder(key)
	default:
		currentOrder := incomingMap[key]
		incomingMap[key] = order.NewOrder(key, currentOrder.OrderFloor, currentOrder.OrderType, state)
	}
}

func normalizeOrderMap(orderMap map[string]order.Order, listOfIDs []string) map[string]order.Order {
	if orderMap == nil {
		orderMap = make(map[string]order.Order)
	}

	for _, id := range listOfIDs {
		_, isInMap := orderMap[id]
		if !isInMap {
			orderMap[id] = order.NewUnknownOrder(id)
		}
	}

	for id, ord := range orderMap {
		if !slices.Contains(listOfIDs, id) {
			delete(orderMap, id)
			continue
		}

		if ord.OrderState == order.SOS_NONE {
			orderMap[id] = order.NewEmptyOrder(id)
			continue
		}
		if ord.OrderState == order.SOS_UNKNOWN {
			orderMap[id] = order.NewUnknownOrder(id)
			continue
		}

		// Checking if map-key is consistent with order.PeerID
		if ord.PeerID != id {
			ord.PeerID = id
			orderMap[id] = ord
		}
	}
	return orderMap
}

func mapsEqual(firstMap map[string]order.Order, secondMap map[string]order.Order) bool {
	if len(firstMap) != len(secondMap) {
		return false
	}
	for key, firstValue := range firstMap {
		secondValue, ok := secondMap[key]
		if !ok || firstValue != secondValue {
			return false
		}
	}
	return true
}

// ! NB ! Only for debugging!!! Must be deleted at a later stage!

func orderSignature(ord order.Order) string {
	return fmt.Sprintf(
		"owner=%s floor=%d button=%d state=%d",
		ord.PeerID,
		ord.OrderFloor,
		ord.OrderType,
		ord.OrderState,
	)
}
