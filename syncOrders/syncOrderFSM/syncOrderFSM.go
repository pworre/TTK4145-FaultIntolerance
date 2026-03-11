package syncOrderFSM

import (
	//"elevatorControl/elevator"
	"log"
	"networkDriver/peers"
	"slices"
	"syncOrders/order"
	"elevatorControl/elevator"
)

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

// Finite state machine loop

/*
1 - Button press
2 - Other peers receive UNCONFIRMED_REQUEST (saves local [localOrderToSyncMap]+ sends ack [iAmAtUnconfirmedRequestBarrier])
3 - Unconfirmed Request promoted to CONFIRMED_REQUEST [allHaveUnconfirmedRequest] ----> orderToPromote := <-allHaveUnconfirmedRequest
4 - Confirmed request barrier: allAgreeToAddOrder <- ord
5 - Order added: orderToAdd := <-allAgreeToAddOrder
6 - Peers sends SOS_NONE

**** ONLY BARRIER-SUCCESS SHOULD MAKE A COMMIT ****
*/


func StateMachineLoop(myID string, newOrderStateTransition chan map[string]order.Order, newOrderStateReceival chan order.OrderStateMessage, confirmedRequest chan order.Order, confirmedDeletion chan order.Order, networkDisconnect chan bool, clearAllConfirmedOrders chan bool, peerUpdateInSyncOrdersFSM chan peers.PeerUpdate, peerUpdateInRequestBarrierStateCounter chan peers.PeerUpdate, peerUpdateInDeletionBarrierStateCounter chan peers.PeerUpdate, peerUpdateInUnconfirmedRequestBarrierStateCounter chan peers.PeerUpdate, peerUpdateInUnconfirmedDeletionBarrierStateCounter chan peers.PeerUpdate) {

	localOrderToSyncMap := make(map[string]order.Order)
	localOrderToSyncMap[myID] = order.NewEmptyOrder(myID)

	iAmAtRequestBarrier := make(chan acknowledgeBarrier, 64)
	iAmAtDeleteBarrier := make(chan acknowledgeBarrier, 64)
	iAmAtUnconfirmedRequestBarrier := make(chan acknowledgeBarrier, 64)
	iAmAtUnconfirmedDeleteBarrier := make(chan acknowledgeBarrier, 64)

	allAgreeToAddOrder := make(chan order.Order)
	allAgreeToDeleteOrder := make(chan order.Order)
	allHaveUnconfirmedRequest := make(chan order.Order)
	allHaveUnconfirmedDeletion := make(chan order.Order)

	go requestBarrierStateCounter(myID, peerUpdateInRequestBarrierStateCounter, iAmAtRequestBarrier, allAgreeToAddOrder)
	go deletionBarrierStateCounter(myID, peerUpdateInDeletionBarrierStateCounter, iAmAtDeleteBarrier, allAgreeToDeleteOrder)
	go unconfirmedRequestBarrierStateCounter(myID, peerUpdateInUnconfirmedRequestBarrierStateCounter, iAmAtUnconfirmedRequestBarrier, allHaveUnconfirmedRequest)
	go unconfirmedDeletionBarrierStateCounter(myID, peerUpdateInUnconfirmedDeletionBarrierStateCounter, iAmAtUnconfirmedDeleteBarrier, allHaveUnconfirmedDeletion)

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
					changed = true
				} 
			}

		case <-networkDisconnect:
			for ID, _ := range localOrderToSyncMap {
				updateOrderStateInMap(localOrderToSyncMap, ID, order.SOS_UNKNOWN)
			}
			clearAllConfirmedOrders <- true
			changed = true

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

						continue

						/*
						switch localOrder.OrderState {
						case order.SOS_NONE, 
							order.SOS_UNCONFIRMED_REQUEST,
							order.SOS_UNCONFIRMED_DELETION,
							order.SOS_CONFIRMED_REQUEST,
							order.SOS_CONFIRMED_DELETION:

							log.Println(incomingID, " told us they have no orders, and we dont care.")

						default:
						}
						*/

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

						iAmAtRequestBarrier <- acknowledgeBarrier{
								key: 	makeBarrierKey(incomingOrderToSync), 
								ackID: 	incomingID,
								ord: 	incomingOrderToSync,
						}

						switch localOrder.OrderState {
						case order.SOS_NONE, order.SOS_UNKNOWN, order.SOS_UNCONFIRMED_REQUEST, order.SOS_CONFIRMED_REQUEST:
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

						iAmAtDeleteBarrier <- acknowledgeBarrier{
							key: 	makeBarrierKey(incomingOrderToSync), 
							ackID: 	incomingID,
							ord: 	incomingOrderToSync,
						}

						switch localOrder.OrderState {
						case order.SOS_UNKNOWN, order.SOS_CONFIRMED_REQUEST, order.SOS_UNCONFIRMED_DELETION, order.SOS_CONFIRMED_DELETION:
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
				localOrder, orderExists := localOrderToSyncMap[orderToAdd.PeerID]
				if !orderExists {
					break
				}
				if localOrder.OrderState != order.SOS_CONFIRMED_REQUEST {
					break
				}

				confirmedRequest <- orderToAdd
				updateOrderStateInMap(localOrderToSyncMap, orderToAdd.PeerID, order.SOS_NONE)
				changed = true

			case orderToDelete := <-allAgreeToDeleteOrder:
				log.Printf("FSM adding confirmedDeletion %+v\n", orderToDelete)

				if orderToDelete.OrderState != order.SOS_CONFIRMED_DELETION {
					log.Printf("WARNING: refusing a non-confirmed-to-delete order to be deleted")
					break
				}

				localOrder, orderExists := localOrderToSyncMap[orderToDelete.PeerID]
				if !orderExists {
					break
				}
				if localOrder.OrderState != order.SOS_CONFIRMED_DELETION {
					break
				}

				confirmedDeletion <- orderToDelete
				updateOrderStateInMap(localOrderToSyncMap, orderToDelete.PeerID, order.SOS_NONE)
				changed = true

			case orderToPromote := <-allHaveUnconfirmedRequest:
				localOrder, peerIsInMap := localOrderToSyncMap[orderToPromote.PeerID]
				if !peerIsInMap {
					break
				}

				if localOrder.OrderState != order.SOS_UNCONFIRMED_REQUEST {
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
				localOrder, peerIsInMap := localOrderToSyncMap[orderToPromote.PeerID]
				if !peerIsInMap {
					break
				}

				if localOrder.OrderState != order.SOS_UNCONFIRMED_DELETION {
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
	}
}


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

			for key, ackList := range peersThatHaveConfirmedRequest {
				if !isElementInList(key.ownerID, fullList) {
					delete(peersThatHaveConfirmedRequest, key)
					continue
				}
				peersThatHaveConfirmedRequest[key] = maskAcklistWithFullist(ackList, fullList)
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
					continue
				}
				peersThatHaveConfirmedDelete[key] = maskAcklistWithFullist(peersThatHaveConfirmedDelete[key], fullList)
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
					continue
				}
				peersThatHaveUnconfirmedRequest[key] = maskAcklistWithFullist(peersThatHaveUnconfirmedRequest[key], fullList)
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
					continue
				}
				peersThatHaveUnconfirmedDelete[key] = maskAcklistWithFullist(peersThatHaveUnconfirmedDelete[key], fullList)
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

func maskAcklistWithFullist(ackList []string, fullList []string) []string {
	maskedList := make([]string, 0, len(ackList))
	for _, ackID := range ackList {
		if isElementInList(ackID, fullList) {
			maskedList = append(maskedList, ackID)
		}
	}
	return maskedList
}