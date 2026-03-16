package syncOrders

import (
	"elevatorControl/elevator"
	"elevatorControl/hallRequestAssigner"
	"elevator_project/config"
	"log"
	"networkDriver/bcast"
	"networkDriver/peers"
	"slices"
	"time"
)

// TODO: Create a barrier reset, for the big barrier
// TODO: Do an actual OS exit

// PEW:
// TODO: Make a merging sequence for orderCompletedWhileDisconnected and ordersTobeRestored

type Order struct {
	Placed  bool
	Version uint16
	Unknown bool
	AckList []string
}

type WorldView struct {
	PeerID        string                                       `json:"peerID"`
	ElevatorState elevator.Elevator                            `json:"elevatorState"`
	OrderView     [elevator.N_FLOORS][elevator.N_BUTTONS]Order `json:"orders"`
	//CabOrders     [elevator.N_FLOORS]elevator.Order `json:"cabOrders"`
}

type CabAck struct {
	OwnerID string `json:"ownerID"`
	Floor   int    `json:"floor"`
	Placed  bool   `json:"placed"`
	Version int    `json:"version"`
	AckerID string `json:"ackerID"`
}

type CabRestore struct {
	TargetID  string
	SenderID  string
	CabOrders [elevator.N_FLOORS]Order
}

const BARRIER = 60000

const TRANSMIT_INTERVAL = 200 * time.Millisecond

const G_BCAST_PORT = 40104
const CAB_ACK_BCAST_PORT = 40105
const CAB_RESTORE_BCAST_PORT = 40106

func SynchronizationLoop(startFloor int, cfg config.Config, localStateChange chan elevator.Elevator, assignEvent chan [elevator.N_FLOORS][elevator.N_BUTTONS]bool, localRequest chan elevator.ButtonEvent, localClearing chan elevator.ButtonEvent, peerUpdate chan peers.PeerUpdate, setLights chan [elevator.N_FLOORS][elevator.N_BUTTONS]bool, activePeersChan chan []string) {

	myID := cfg.ID
	myWorldView := WorldView{
		PeerID:        myID,
		ElevatorState: elevator.NewStartElevator(startFloor),
		OrderView:     [elevator.N_FLOORS][elevator.N_BUTTONS]Order{},
	}

	activePeersList := []string{}
	lostPeersList := []string{}
	outOfServicePeersList := []string{}
	peerStates := make(map[string]elevator.Elevator)
	peerCabOrders := make(map[string][elevator.N_FLOORS]Order)
	lostPeerBackupStates := make(map[string]WorldView)

	newConfirmedPlacements := [elevator.N_FLOORS][elevator.N_BUTTONS]bool{}
	lastConfirmedPlacements := newConfirmedPlacements
	newPeerOutOfService := false

	// Channels, routines and timer for periodically broadcasting and receiving WorldViews between peers
	networkRx := make(chan WorldView, 1024)
	networkTx := make(chan WorldView, 1024)
	go bcast.Transmitter(G_BCAST_PORT, networkTx)
	go bcast.Receiver(G_BCAST_PORT, networkRx)

	cabAckRx := make(chan CabAck, 1024)
	cabAckTx := make(chan CabAck, 1024)
	go bcast.Transmitter(CAB_ACK_BCAST_PORT, cabAckTx)
	go bcast.Receiver(CAB_ACK_BCAST_PORT, cabAckRx)

	cabRestoreRx := make(chan CabRestore, 1024)
	cabRestoreTx := make(chan CabRestore, 1024)
	go bcast.Transmitter(CAB_RESTORE_BCAST_PORT, cabRestoreTx)
	go bcast.Receiver(CAB_RESTORE_BCAST_PORT, cabRestoreRx)

	ticker := time.NewTicker(TRANSMIT_INTERVAL)
	defer ticker.Stop()

	for {
		select {
		case request := <-localRequest:
			if request.Button == elevator.B_Cab {
				myWorldView.OrderView[request.Floor][request.Button].Placed = true
				myWorldView.OrderView[request.Floor][request.Button].Unknown = false
				if myWorldView.OrderView[request.Floor][request.Button].Version < BARRIER {
					myWorldView.OrderView[request.Floor][request.Button].Version = BARRIER
				} else {
					myWorldView.OrderView[request.Floor][request.Button].Version += 1
				}
				myWorldView.OrderView[request.Floor][request.Button].AckList = []string{myID}
			} else {
				myWorldView.OrderView[request.Floor][request.Button].Placed = true
				myWorldView.OrderView[request.Floor][request.Button].Unknown = false
				myWorldView.OrderView[request.Floor][request.Button].AckList = []string{myID}
				myWorldView.OrderView[request.Floor][request.Button].Version += 1
			}

		case request := <-localClearing:
			if request.Button == elevator.B_Cab {
				myWorldView.OrderView[request.Floor][request.Button].Placed = false
				myWorldView.OrderView[request.Floor][request.Button].Unknown = false
				if myWorldView.OrderView[request.Floor][request.Button].Version < BARRIER {
					myWorldView.OrderView[request.Floor][request.Button].Version = BARRIER
				} else {
					myWorldView.OrderView[request.Floor][request.Button].Version += 1
				}
				myWorldView.OrderView[request.Floor][request.Button].AckList = []string{myID}
			} else {
				myWorldView.OrderView[request.Floor][request.Button].Placed = false
				myWorldView.OrderView[request.Floor][request.Button].Unknown = false
				myWorldView.OrderView[request.Floor][request.Button].AckList = []string{myID}
				myWorldView.OrderView[request.Floor][request.Button].Version += 1
			}

		case newLocalState := <-localStateChange:
			myWorldView.ElevatorState = newLocalState
			if !slices.Contains(outOfServicePeersList, myID) {
				outOfServicePeersList = append(outOfServicePeersList, myID)
			}

			// Reassign orders
			newConfirmedPlacements := extractConfirmedPlacements(myWorldView, lastConfirmedPlacements, activePeersList)
			assignOrders(myID, myWorldView.ElevatorState, newConfirmedPlacements, peerStates, peerCabOrders, outOfServicePeersList, assignEvent)
			setLights <- newConfirmedPlacements
			lastConfirmedPlacements = newConfirmedPlacements

		case newPeerUpdate := <-peerUpdate:
			oldActivePeerList := activePeersList
			newActivePeerList := newPeerUpdate.Peers

			log.Printf("Old peer list: %v", oldActivePeerList)
			log.Printf("New peer list: %v", newActivePeerList)

			for _, id := range newActivePeerList {
				// Check if new peer
				if !slices.Contains(oldActivePeerList, id) {
					if !slices.Contains(lostPeersList, id) {
						log.Printf("Peer %s joined the network", id)
					} else {
						if backup, exists := lostPeerBackupStates[id]; exists {
							log.Printf("Retrieving backup for peer %s", id)

							// Retrieve backup
							peerStates[id] = backup.ElevatorState
							//peerCabOrders[id] = backup.CabOrders

							// ! OBS ! We SHOULD have an check before assigning peerStates and peerCabOrders
							cabRestoreTx <- CabRestore{
								TargetID:  id,
								SenderID:  myID,
								CabOrders: extractCabOrders(backup.OrderView),
								//CabOrders: backup.CabOrders,
							}

							delete(lostPeerBackupStates, id)

							// remove peer from lostPeerList
							for index, lostID := range lostPeersList {
								if lostID == id {
									lostPeersList = append(lostPeersList[:index], lostPeersList[index+1:]...)
									break
								}
							}
						}
					}
				}

			}
			activePeersList = newPeerUpdate.Peers
			// Save backup for lost peers
			log.Printf("Lost peers: %v", newPeerUpdate.Lost)
			for _, lostID := range newPeerUpdate.Lost {

				if slices.Contains(lostPeersList, lostID) {
					log.Printf("Lost peer %s, but it has already been lost", lostID)
					continue
				} else {
					lostPeersList = append(lostPeersList, lostID)
				}

				// Have to save a full OrderView, but only care about backuping the cabOrders
				orderBackup := [elevator.N_FLOORS][elevator.N_BUTTONS]Order{}
				for floor := 0; floor < elevator.N_FLOORS; floor++ {
					orderBackup[floor][elevator.N_BUTTONS-1] = peerCabOrders[lostID][floor]
				}

				lostPeerBackupStates[lostID] = WorldView{
					PeerID:        lostID,
					ElevatorState: peerStates[lostID],
					OrderView:     orderBackup,
				}

				// ? Blir lostID aldri overskrepet: så vi mister aldri listen?
				// TODO: Legg til at vi fjerner fra mapet hvis reconnect
				delete(peerStates, lostID)
				delete(peerCabOrders, lostID)

			}

			// !!! Maybe the unknown field is totally obsolete...???
			/*
				if len(activePeersList) == 0 {
					for floor := 0; floor < elevator.N_FLOORS; floor++ {
						for button := 0; button < elevator.N_BUTTONS; button++ {
							myWorldView.OrderView[floor][button].Unknown = true
						}
					}
				}
			*/

			for _, str := range activePeersList {
				log.Printf("Peer number: %s", str)
			}

			if needToAssignAgain(myWorldView, lastConfirmedPlacements, activePeersList) {

				newConfirmedPlacements := extractConfirmedPlacements(myWorldView, lastConfirmedPlacements, activePeersList)

				assignOrders(myID, myWorldView.ElevatorState, newConfirmedPlacements, peerStates, peerCabOrders, outOfServicePeersList, assignEvent)
				setLights <- newConfirmedPlacements

				lastConfirmedPlacements = newConfirmedPlacements
			}

		case restore := <-cabRestoreRx:
			if restore.TargetID != myID {
				break
			}

			localCabOrders := extractCabOrders(myWorldView.OrderView)

			for floor := 0; floor < elevator.N_FLOORS; floor++ {
				incomingCabOrder := restore.CabOrders[floor]
				localCabOrder := localCabOrders[floor]

				if incomingCabOrder.Unknown {
					continue
				}

				restoredCabOrder := incomingCabOrder
				restoredCabOrder.Unknown = false
				restoredCabOrder.AckList = []string{myID}

				// merge
				if restoredCabOrder.Version > localCabOrder.Version {
					myWorldView.OrderView[floor][elevator.N_BUTTONS-1] = restoredCabOrder
				} else if restoredCabOrder.Version == localCabOrder.Version {
					if restoredCabOrder.Placed || localCabOrder.Placed {
						merged := localCabOrder
						merged.Placed = true
						merged.Unknown = false
						merged.AckList = []string{myID}
						if merged.Version < BARRIER {
							merged.Version = BARRIER
						}
						myWorldView.OrderView[floor][elevator.N_BUTTONS-1] = merged
					}
				}
			}

			if needToAssignAgain(myWorldView, lastConfirmedPlacements, activePeersList) {

				// This function sets the placements for all the orders that have a full AckList,
				// and uses the lastConfirmedPlacements for all the rest (That dont have a full AckList)
				newConfirmedPlacements := extractConfirmedPlacements(myWorldView, lastConfirmedPlacements, activePeersList)

				assignOrders(myID, myWorldView.ElevatorState, newConfirmedPlacements, peerStates, peerCabOrders, outOfServicePeersList, assignEvent)
				setLights <- newConfirmedPlacements

				lastConfirmedPlacements = newConfirmedPlacements
			}

		case incomingWorldView := <-networkRx:
			//log.Printf("Decoded worldview before filtering: %+v", incomingWorldView)

			// Ignore my own rebroadcasts
			if incomingWorldView.PeerID == myID {
				break
			}

			// Ignore messages if we have not yet gotten the peerUpdate
			if !slices.Contains(activePeersList, incomingWorldView.PeerID) {
				log.Printf("Discarded worldview from %s because activePeersList=%v",
					incomingWorldView.PeerID, activePeersList)
				break
			}

			//fmt.Printf("I am receiving a worldview from someone else: %+v", incomingWorldView)

			// Be aware! We need this for the hallrequestassigner, but we should not trust their requests as our own,
			// they are only meant to be used as inputs for the HRA
			peerStates[incomingWorldView.PeerID] = incomingWorldView.ElevatorState

			// TEST FOR OUTOFSERVICE ! ! ! ! ! ! ! ! !
			//outOfServicePeersList = append(outOfServicePeersList, "2")

			isPeerOutOfService := incomingWorldView.ElevatorState.OutOfService
			if isPeerOutOfService {
				if !slices.Contains(outOfServicePeersList, incomingWorldView.PeerID) {
					outOfServicePeersList = append(outOfServicePeersList, incomingWorldView.PeerID)
					newPeerOutOfService = true
				}
			} else {
				// Remove from outOfServicePeersList if incoming is no longer out of service
				filteredList := []string{}
				//filteredList := outOfServicePeersList[:0]
				for _, id := range outOfServicePeersList {
					if id != incomingWorldView.PeerID {
						filteredList = append(filteredList, id)
					}
				}
				outOfServicePeersList = filteredList
			}

			newWorldView := myWorldView

			// TODO: CabOrder Acklist updating logic
			// Caborder updating
			unknownCount := 0
			incomingCabOrders := extractCabOrders(incomingWorldView.OrderView)

			peerCabOrders[incomingWorldView.PeerID] = incomingCabOrders

			for floor := 0; floor < elevator.N_FLOORS; floor++ {
				incomingCabOrder := incomingCabOrders[floor]

				if incomingCabOrder.Unknown {
					unknownCount++
					continue
				}

				// We only care about cab-orders in barrier-state
				if incomingCabOrder.Version < BARRIER {
					continue
				}

				cabAckTx <- CabAck{
					OwnerID: incomingWorldView.PeerID,
					Floor:   floor,
					Placed:  incomingCabOrder.Placed,
					Version: int(incomingCabOrder.Version),
					AckerID: myID,
				}
			}

			// Slightly ugly, but necessary maintaining of the CabOrders map in myWorldView
			if unknownCount > 0 && unknownCount != elevator.N_FLOORS {
				log.Println("Something weird has happened, either all or no orders should be unknown at the same time")
			}

			//log.Printf("CabOrders received from peer %s: %+v", incomingWorldView.PeerID, peerCabOrders[incomingWorldView.PeerID])

			// Hallorder synchronization
			for floor := 0; floor < elevator.N_FLOORS; floor++ {
				for button := 0; button < (elevator.N_BUTTONS - 1); button++ {
					incomingHallOrder := incomingWorldView.OrderView[floor][button]
					if incomingHallOrder.Unknown {
						continue
					}

					localHallOrder := newWorldView.OrderView[floor][button]

					if localHallOrder.Unknown {
						// Trust incomingOrder, and set your own order identical
						newWorldView.OrderView[floor][button] = incomingHallOrder
						// These two are included in the one above
						//newWorldView.ElevatorState.Requests[floor][button].AckList = incomingHallOrder.AckList
						//newWorldView.ElevatorState.Requests[floor][button].Unknown = false
					} else {

						if localHallOrder.Version > incomingHallOrder.Version {
							// Ignore orders coming in that are older than you
							log.Println("I ignored someone else")
							continue

						} else if localHallOrder.Version == incomingHallOrder.Version {
							if localHallOrder.Placed == incomingHallOrder.Placed {
								newWorldView.OrderView[floor][button].Placed = localHallOrder.Placed
								newWorldView.OrderView[floor][button].Version = localHallOrder.Version
								newWorldView.OrderView[floor][button].AckList = mergeAckLists(localHallOrder.AckList, incomingHallOrder.AckList)
								newWorldView.OrderView[floor][button].AckList = addAck(newWorldView.OrderView[floor][button].AckList, myID)
							} else {
								// This will in the worst case do an order twice, but never miss an order
								log.Println("I made a compromize with someone else")
								newWorldView.OrderView[floor][button].Placed = (localHallOrder.Placed || incomingHallOrder.Placed)
								// Before we were in the middle of either clearing an order or adding one,
								// we have now started either a new adding or a new clearing, and must clear the acklist and increment the version
								newWorldView.OrderView[floor][button].Version += 1
								newWorldView.OrderView[floor][button].AckList = []string{myID}
							}

						} else if localHallOrder.Version < incomingHallOrder.Version {
							// Accept orders that are newer than us
							//log.Println("I got convinced by someone else")
							newWorldView.OrderView[floor][button].Placed = incomingHallOrder.Placed
							newWorldView.OrderView[floor][button].Version = incomingHallOrder.Version
							newWorldView.OrderView[floor][button].AckList = mergeAckLists(incomingHallOrder.AckList, []string{myID})
							//newWorldView.ElevatorState.Requests[floor][button].Version += 1 // ! Needed because we have the merged AckList now
						}

					}

				}
			}

			if needToAssignAgain(newWorldView, lastConfirmedPlacements, activePeersList) || newPeerOutOfService {

				// This function sets the placements for all the orders that have a full AckList,
				// and uses the lastConfirmedPlacements for all the rest (That dont have a full AckList)
				newConfirmedPlacements := extractConfirmedPlacements(newWorldView, lastConfirmedPlacements, activePeersList)

				assignOrders(myID, newWorldView.ElevatorState, newConfirmedPlacements, peerStates, peerCabOrders, outOfServicePeersList, assignEvent)
				setLights <- newConfirmedPlacements

				lastConfirmedPlacements = newConfirmedPlacements
				newPeerOutOfService = false
			}

			// Even if no new assignment was needed, the version number might still be updated, so we must update state
			myWorldView = newWorldView

		case cabAck := <-cabAckRx:
			if cabAck.OwnerID != myID {
				break
			}

			if cabAck.Floor < 0 || cabAck.Floor >= elevator.N_FLOORS {
				break
			}

			localCabOrder := myWorldView.OrderView[cabAck.Floor][elevator.N_BUTTONS-1]

			if localCabOrder.Version < BARRIER {
				break
			}
			if localCabOrder.Version != uint16(cabAck.Version) {
				break
			}
			if localCabOrder.Placed != cabAck.Placed {
				break
			}

			myWorldView.OrderView[cabAck.Floor][elevator.N_BUTTONS-1].AckList = addAck(myWorldView.OrderView[cabAck.Floor][elevator.N_BUTTONS-1].AckList, cabAck.AckerID)

			if needToAssignAgain(myWorldView, lastConfirmedPlacements, activePeersList) {

				// This function sets the placements for all the orders that have a full AckList,
				// and uses the lastConfirmedPlacements for all the rest (That dont have a full AckList)
				newConfirmedPlacements := extractConfirmedPlacements(myWorldView, lastConfirmedPlacements, activePeersList)

				assignOrders(myID, myWorldView.ElevatorState, newConfirmedPlacements, peerStates, peerCabOrders, outOfServicePeersList, assignEvent)
				setLights <- newConfirmedPlacements

				lastConfirmedPlacements = newConfirmedPlacements
			}

		case <-ticker.C:
			// Send myWorldView
			networkTx <- myWorldView
			//log.Println("Hopefully sent something!")
		}

		allElevatorIDs := append([]string{}, activePeersList...)
		if !slices.Contains(allElevatorIDs, myID) {
			allElevatorIDs = append(allElevatorIDs, myID)
		}

		if len(allElevatorIDs) == 1 {
			//oldWorldView := lastWorldView
			newWorldView := myWorldView

			for floor := 0; floor < elevator.N_FLOORS; floor++ {
				if newWorldView.OrderView[floor][elevator.N_BUTTONS-1].Version >= BARRIER {
					newWorldView.OrderView[floor][elevator.N_BUTTONS-1].AckList = addAck(newWorldView.OrderView[floor][elevator.N_BUTTONS-1].AckList, myID)
				}
				for button := 0; button < elevator.N_BUTTONS-1; button++ {
					newWorldView.OrderView[floor][button].AckList = addAck(newWorldView.OrderView[floor][button].AckList, myID)
				}
			}

			if needToAssignAgain(newWorldView, lastConfirmedPlacements, activePeersList) {

				// This function sets the placements for all the orders that have a full AckList,
				// and uses the lastConfirmedPlacements for all the rest (That dont have a full AckList)
				newConfirmedPlacements := extractConfirmedPlacements(newWorldView, lastConfirmedPlacements, activePeersList)

				assignOrders(myID, newWorldView.ElevatorState, newConfirmedPlacements, peerStates, peerCabOrders, outOfServicePeersList, assignEvent)
				setLights <- newConfirmedPlacements

				lastConfirmedPlacements = newConfirmedPlacements
			}

			myWorldView = newWorldView
		}
	}
}

func assignOrders(myID string, myState elevator.Elevator, placements [elevator.N_FLOORS][elevator.N_BUTTONS]bool, peerStates map[string]elevator.Elevator, peerCabOrders map[string][elevator.N_FLOORS]Order, outOfServiceList []string, assignEvent chan<- [elevator.N_FLOORS][elevator.N_BUTTONS]bool) {

	log.Println("Assigning orders...")

	hraInput := hallRequestAssigner.HRAInput{
		HallRequests: make([][elevator.N_BUTTONS - 1]bool, elevator.N_FLOORS),
		States:       make(map[string]hallRequestAssigner.HRAElevState),
	}

	input := [][elevator.N_BUTTONS - 1]bool{}

	for floor := 0; floor < elevator.N_FLOORS; floor++ {
		temp := [elevator.N_BUTTONS - 1]bool{}
		for button := 0; button < elevator.N_BUTTONS-1; button++ {
			temp[button] = placements[floor][button]
		}
		input = append(input, temp)
	}
	hraInput.HallRequests = input

	// Only assign orders to elevators not out of service
	allServicableElevatorIDs := []string{}
	if !myState.OutOfService {
		allServicableElevatorIDs = append(allServicableElevatorIDs, myID)
	}
	for peerID := range peerStates {
		if !slices.Contains(outOfServiceList, peerID) {
			allServicableElevatorIDs = append(allServicableElevatorIDs, peerID)
		}
	}
	log.Printf("All servicable elevators: %v", allServicableElevatorIDs)

	allServicableElevatorStates := make(map[string]elevator.Elevator)
	allServicableCabPlacements := make(map[string][]bool)
	for _, id := range allServicableElevatorIDs {
		if id == myID {
			allServicableElevatorStates[id] = myState
			allServicableCabPlacements[myID] = extractCabOrderPlacements(placements)
		} else {
			allServicableElevatorStates[id] = peerStates[id]
			servicableCabOrderPlacements := []bool{}
			for floor := 0; floor < elevator.N_FLOORS; floor++ {
				servicableCabOrderPlacements = append(servicableCabOrderPlacements, peerCabOrders[id][floor].Placed)
			}
			allServicableCabPlacements[id] = []bool(servicableCabOrderPlacements)
		}
	}

	for _, id := range allServicableElevatorIDs {
		// convert elevator.behaviour [int] to hra.behaviour [string]
		var elevBehaviour_hra string
		switch allServicableElevatorStates[id].Behaviour {
		case elevator.EB_Idle:
			elevBehaviour_hra = "idle"
		case elevator.EB_Moving:
			elevBehaviour_hra = "moving"
		case elevator.EB_DoorOpen:
			elevBehaviour_hra = "doorOpen"
		}

		var elevDirection_hra string
		switch allServicableElevatorStates[id].Direction {
		case elevator.D_Up:
			elevDirection_hra = "up"
		case elevator.D_Stop:
			elevDirection_hra = "stop"
		case elevator.D_Down:
			elevDirection_hra = "down"
		}

		cabRequests_hra := allServicableCabPlacements[id]

		hraInput.States[id] = hallRequestAssigner.HRAElevState{
			Behavior:    elevBehaviour_hra,
			Floor:       allServicableElevatorStates[id].Floor,
			Direction:   elevDirection_hra,
			CabRequests: cabRequests_hra,
		}

	}

	newAssignmentMap := hallRequestAssigner.Decode(hallRequestAssigner.AssignOrders(hallRequestAssigner.Encode(hraInput)))
	newAssignment := newAssignmentMap[myID]

	assignEvent <- newAssignment
}

func needToAssignAgain(newWorldView WorldView, lastConfirmedPlacements [elevator.N_FLOORS][elevator.N_BUTTONS]bool, activePeersList []string) bool {
	return extractConfirmedPlacements(newWorldView, lastConfirmedPlacements, activePeersList) != lastConfirmedPlacements

}

// This function sets the placements for all the orders that have a full AckList,
// and uses the lastConfirmedPlacements for all the rest (That dont have a full AckList)
func extractConfirmedPlacements(newWorldView WorldView, lastConfirmedPlacements [elevator.N_FLOORS][elevator.N_BUTTONS]bool, activePeersList []string) [elevator.N_FLOORS][elevator.N_BUTTONS]bool {
	newConfirmedPlacements := [elevator.N_FLOORS][elevator.N_BUTTONS]bool{}

	requiredAcks := []string{}
	for _, id := range activePeersList {
		if !slices.Contains(requiredAcks, id) {
			requiredAcks = append(requiredAcks, id)
		}
	}
	if !slices.Contains(requiredAcks, newWorldView.PeerID) {
		requiredAcks = append(requiredAcks, newWorldView.PeerID)
	}

	/*
		requiredAcks := append([]string{}, activePeersList...)
		requiredAcks = append(requiredAcks, newWorldView.PeerID)
	*/

	for floor := 0; floor < elevator.N_FLOORS; floor++ {
		for button := 0; button < elevator.N_BUTTONS; button++ {
			order := newWorldView.OrderView[floor][button]

			if containSameElements(order.AckList, requiredAcks) {
				newConfirmedPlacements[floor][button] = order.Placed
			} else {
				newConfirmedPlacements[floor][button] = lastConfirmedPlacements[floor][button]
			}
		}
	}
	return newConfirmedPlacements
}

func addAck(ackList []string, id string) []string {
	if !slices.Contains(ackList, id) {
		return append(ackList, id)
	}
	return ackList
}

func extractCabOrders(requests [elevator.N_FLOORS][elevator.N_BUTTONS]Order) [elevator.N_FLOORS]Order {
	var cabOrders [elevator.N_FLOORS]Order
	for floor := 0; floor < elevator.N_FLOORS; floor++ {
		cabOrders[floor] = requests[floor][elevator.N_BUTTONS-1]
	}
	return cabOrders
}

func extractCabOrderPlacements(requests [elevator.N_FLOORS][elevator.N_BUTTONS]bool) []bool {
	cabOrderPlacements := []bool{}
	for floor := 0; floor < elevator.N_FLOORS; floor++ {
		cabOrderPlacements = append(cabOrderPlacements, requests[floor][elevator.N_BUTTONS-1])
	}
	return cabOrderPlacements
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

func mergeAckLists(firstAckList []string, secondAckList []string) []string {
	mergeList := []string{}
	for _, ID := range firstAckList {
		mergeList = append(mergeList, ID)
	}
	for _, ID := range secondAckList {
		if !isElementInList(ID, firstAckList) {
			mergeList = append(mergeList, ID)
		}
	}
	return mergeList
}
