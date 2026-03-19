package syncOrders

// - - - - - - Overview - - - - - - - - -

// This module cointains an order syncronization algorithm between peers in a distributed network elevator system.
// This module synchronizes the requests between local elevator instances on the network,
// only extracting the orders that have been acknowledged by all peers on the network for
// doing side-effects like lighting buttons or assigning orders. If a peer is disconnected or put out of service for any reason,
// the other peers on the network can then serve the orders or give them back to the peer when it becomes available again,
// and thus we maintain the guarantee that a light turning on means an elevator will serve the order.

import (
	"elevatorControl/elevator"
	"elevatorControl/hallRequestAssigner"
	"log"
	"networkDriver/bcast"
	"networkDriver/peers"
	"slices"
	"time"
)

type Order struct {
	Placed  bool     // Whether or not there is a request
	Version uint16   // Non-monotonic cyclic counter
	AckList []string // List of peers that have acknowledged the order
}

type WorldView struct {
	PeerID        string                                       `json:"peerID"`
	ElevatorState elevator.Elevator                            `json:"elevatorState"`
	OrderView     [elevator.N_FLOORS][elevator.N_BUTTONS]Order `json:"orders"`
}

type CabAck struct {
	OwnerID string `json:"ownerID"`
	Floor   int    `json:"floor"`
	Placed  bool   `json:"placed"`
	AckerID string `json:"ackerID"`
}

type CabRestore struct {
	TargetID  string
	SenderID  string
	CabOrders [elevator.N_FLOORS]Order
}

// Max version number
const HALL_SYNC_BARRIER = 60000

const TRANSMIT_INTERVAL = 50 * time.Millisecond

const G_BCAST_PORT = 40104
const CAB_ACK_BCAST_PORT = 40105
const CAB_RESTORE_BCAST_PORT = 40106

func SynchronizationLoop(myID string, startFloor int,
	localRequest chan elevator.ButtonEvent, localClearing chan elevator.ButtonEvent,
	localStateChange chan elevator.Elevator, peerUpdate chan peers.PeerUpdate,
	restartWorldView chan WorldView, stillActive chan bool, backupStore chan WorldView,
	assignEvent chan [elevator.N_FLOORS][elevator.N_BUTTONS]bool,
	setLights chan [elevator.N_FLOORS][elevator.N_BUTTONS]bool) {

	// Worldview for broadcasting
	myWorldView := WorldView{
		PeerID:        myID,
		ElevatorState: elevator.NewStartElevator(startFloor),
		OrderView:     [elevator.N_FLOORS][elevator.N_BUTTONS]Order{},
	}

	// Barrier counter for all hall orders, for resetting the version number
	peersAtHallSyncBarrier := [elevator.N_FLOORS][elevator.N_BUTTONS - 1]map[string]bool{}
	for floor := 0; floor < elevator.N_FLOORS; floor++ {
		for button := 0; button < (elevator.N_BUTTONS - 1); button++ {
			peersAtHallSyncBarrier[floor][button] = make(map[string]bool)
		}
	}

	// Peer lists and states
	activePeersList := []string{}
	lostPeersList := []string{}
	outOfServicePeersList := []string{}

	peerStates := make(map[string]elevator.Elevator)
	peerCabOrders := make(map[string][elevator.N_FLOORS]Order)
	lostPeerBackupStates := make(map[string]WorldView)

	// Reassignment variables
	newConfirmedPlacements := [elevator.N_FLOORS][elevator.N_BUTTONS]bool{}
	lastConfirmedPlacements := newConfirmedPlacements

	shouldReassign := false

	// Channels and routines for broadcasting and receiving worldviews between peers
	networkRx := make(chan WorldView, 1024)
	networkTx := make(chan WorldView, 1024)
	go bcast.Transmitter(G_BCAST_PORT, networkTx)
	go bcast.Receiver(G_BCAST_PORT, networkRx)

	// Channels and routines for acknowledging cab orders between peers
	cabAckRx := make(chan CabAck, 1024)
	cabAckTx := make(chan CabAck, 1024)
	go bcast.Transmitter(CAB_ACK_BCAST_PORT, cabAckTx)
	go bcast.Receiver(CAB_ACK_BCAST_PORT, cabAckRx)

	// Channels and routines for restoring cab orders upon a reconnect
	cabRestoreRx := make(chan CabRestore, 1024)
	cabRestoreTx := make(chan CabRestore, 1024)
	go bcast.Transmitter(CAB_RESTORE_BCAST_PORT, cabRestoreTx)
	go bcast.Receiver(CAB_RESTORE_BCAST_PORT, cabRestoreRx)

	// Timer for periodic broadcasting of worldview
	ticker := time.NewTicker(TRANSMIT_INTERVAL)
	defer ticker.Stop()

	for {
		select {
		case request := <-localRequest:
			myWorldView.OrderView[request.Floor][request.Button].Placed = true
			myWorldView.OrderView[request.Floor][request.Button].AckList = []string{myID}
			// Only hall orders need version, for cab orders we synchronize by trusting the peer owning it
			if request.Button != elevator.B_Cab {
				myWorldView.OrderView[request.Floor][request.Button].Version += 1
			}

		case request := <-localClearing:
			myWorldView.OrderView[request.Floor][request.Button].Placed = false
			myWorldView.OrderView[request.Floor][request.Button].AckList = []string{myID}
			// Only hall orders need version, for cab orders we synchronize by trusting the peer owning it
			if request.Button != elevator.B_Cab {
				myWorldView.OrderView[request.Floor][request.Button].Version += 1
			}

		case newLocalState := <-localStateChange:
			myWorldView.ElevatorState = newLocalState
			shouldReassign = true

		case newPeerUpdate := <-peerUpdate:
			oldActivePeersList := activePeersList
			newActivePeersList := newPeerUpdate.Peers

			if len(newActivePeersList) != len(oldActivePeersList) {
				shouldReassign = true
			}

			for _, id := range newActivePeersList {
				// Check if new peer
				if !slices.Contains(oldActivePeersList, id) {
					// Check if we have never heard from this peer before
					if !slices.Contains(lostPeersList, id) {
						log.Printf("Peer %s joined the network", id)

					} else {
						// We have a rejoined peer
						backup, exists := lostPeerBackupStates[id]

						if exists {
							// Retrieve backup and send to the rejoined peer
							peerStates[id] = backup.ElevatorState

							cabRestoreTx <- CabRestore{
								TargetID:  id,
								SenderID:  myID,
								CabOrders: extractCabOrders(backup.OrderView),
							}

							delete(lostPeerBackupStates, id)

							// Remove peer from lostPeerList
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
			activePeersList = newActivePeersList

			// Save backup for lost peers
			log.Printf("Lost peers: %v", newPeerUpdate.Lost)
			for _, lostID := range newPeerUpdate.Lost {

				if slices.Contains(lostPeersList, lostID) {
					log.Printf("Lost peer %s, but it has already been lost", lostID)
					continue
				}

				lostPeersList = append(lostPeersList, lostID)

				// Update barrier counter maps
				for floor := 0; floor < elevator.N_FLOORS; floor++ {
					for button := 0; button < (elevator.N_BUTTONS - 1); button++ {
						delete(peersAtHallSyncBarrier[floor][button], lostID)
					}
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

				delete(peerStates, lostID)
				delete(peerCabOrders, lostID)
			}

			if len(activePeersList) == 0 {
				newWorldView := myWorldView

				// If we are alone, we make sure that all orders are acknowledged by us and therefore confirmed
				for floor := 0; floor < elevator.N_FLOORS; floor++ {
					for button := 0; button < elevator.N_BUTTONS; button++ {
						newWorldView.OrderView[floor][button].AckList = addAck(newWorldView.OrderView[floor][button].AckList, myID)
					}
				}

				myWorldView = newWorldView
			}

		case restore := <-cabRestoreRx:
			if restore.TargetID != myID {
				break
			}
			// If we got a cab restore to our ID, we must have rejoined the network

			localCabOrders := extractCabOrders(myWorldView.OrderView)

			for floor := 0; floor < elevator.N_FLOORS; floor++ {
				restoredCabOrder := restore.CabOrders[floor]
				localCabOrder := localCabOrders[floor]

				// Must serve both restore orders and orders made while elevator was disconnected
				// This will in the worst case do an order twice, but never miss an order
				restoredCabOrder.Placed = (restoredCabOrder.Placed || localCabOrder.Placed)
				restoredCabOrder.AckList = []string{myID}
				myWorldView.OrderView[floor][elevator.N_BUTTONS-1] = restoredCabOrder
			}

		case myWorldView = <-restartWorldView:
			// Acknowledge our own cab orders upon restart
			for floor := 0; floor < elevator.N_FLOORS; floor++ {
				myWorldView.OrderView[floor][elevator.N_BUTTONS-1].AckList = addAck(myWorldView.OrderView[floor][elevator.N_BUTTONS-1].AckList, myID)
			}
			shouldReassign = true

		case incomingWorldView := <-networkRx:
			// Ignore my own rebroadcasts
			if incomingWorldView.PeerID == myID {
				break
			}

			// Ignore messages if we have not yet gotten the peerUpdate
			if !slices.Contains(activePeersList, incomingWorldView.PeerID) {
				break
			}

			// Peer states for hall request assigner
			peerStates[incomingWorldView.PeerID] = incomingWorldView.ElevatorState

			// Update outOfServicePeersList
			isPeerOutOfService := incomingWorldView.ElevatorState.OutOfService

			if isPeerOutOfService {
				if !slices.Contains(outOfServicePeersList, incomingWorldView.PeerID) {
					outOfServicePeersList = append(outOfServicePeersList, incomingWorldView.PeerID)
					shouldReassign = true
				}
			} else {
				filteredList := []string{}
				for _, id := range outOfServicePeersList {
					if id != incomingWorldView.PeerID {
						filteredList = append(filteredList, id)
					}
				}
				if len(outOfServicePeersList) != len(filteredList) {
					shouldReassign = true
				}
				outOfServicePeersList = filteredList
			}

			// Order synchronization
			newWorldView := myWorldView

			incomingCabOrders := extractCabOrders(incomingWorldView.OrderView)

			// Caborders are synchronized by trusting the peer who owns them, since they are the only ones who can detect changes
			// However, we must still acknowledge them, since only orders with full ackList get confirmed
			peerCabOrders[incomingWorldView.PeerID] = incomingCabOrders

			for floor := 0; floor < elevator.N_FLOORS; floor++ {
				incomingCabOrder := incomingCabOrders[floor]

				cabAckTx <- CabAck{
					OwnerID: incomingWorldView.PeerID,
					Floor:   floor,
					Placed:  incomingCabOrder.Placed,
					AckerID: myID,
				}
			}

			// Hallorder synchronization
			for floor := 0; floor < elevator.N_FLOORS; floor++ {
				for button := 0; button < (elevator.N_BUTTONS - 1); button++ {

					incomingHallOrder := incomingWorldView.OrderView[floor][button]
					localHallOrder := newWorldView.OrderView[floor][button]

					if localHallOrder.Version < 30 && incomingHallOrder.Version >= HALL_SYNC_BARRIER {
						// Either we have just reset the version number and are waiting for the others, or we are a new peer just joining
						// In case we are a new peer, we must take OR
						// This will in the worst case do an order twice, but never miss an order
						newWorldView.OrderView[floor][button].Placed = (localHallOrder.Placed || incomingHallOrder.Placed)
						newWorldView.OrderView[floor][button].Version += 1
						newWorldView.OrderView[floor][button].AckList = []string{myID}

					} else if localHallOrder.Version >= HALL_SYNC_BARRIER && incomingHallOrder.Version < 30 {
						// Accept incoming orders that have just reset the version number if we have not reset yet, and then we also reset
						newWorldView.OrderView[floor][button].Placed = incomingHallOrder.Placed
						newWorldView.OrderView[floor][button].Version = incomingHallOrder.Version
						newWorldView.OrderView[floor][button].AckList = mergeAckLists(incomingHallOrder.AckList, []string{myID})
						peersAtHallSyncBarrier[floor][button] = make(map[string]bool)

					} else {
						if localHallOrder.Version > incomingHallOrder.Version {
							// Ignore orders coming in that are older than us

						} else if localHallOrder.Version == incomingHallOrder.Version {
							// Merge orders with same version as us
							if localHallOrder.Placed == incomingHallOrder.Placed {
								newWorldView.OrderView[floor][button].AckList = mergeAckLists(localHallOrder.AckList, incomingHallOrder.AckList)
								newWorldView.OrderView[floor][button].AckList = addAck(newWorldView.OrderView[floor][button].AckList, myID)
							} else {
								// This will in the worst case do an order twice, but never miss an order
								newWorldView.OrderView[floor][button].Placed = (localHallOrder.Placed || incomingHallOrder.Placed)
								newWorldView.OrderView[floor][button].Version += 1
								newWorldView.OrderView[floor][button].AckList = []string{myID}
							}

						} else if localHallOrder.Version < incomingHallOrder.Version {
							// Accept orders that are newer than us
							newWorldView.OrderView[floor][button].Placed = incomingHallOrder.Placed
							newWorldView.OrderView[floor][button].Version = incomingHallOrder.Version
							newWorldView.OrderView[floor][button].AckList = mergeAckLists(incomingHallOrder.AckList, []string{myID})
						}
					}

					// Barrier reset logic
					incomingHallOrder = incomingWorldView.OrderView[floor][button]
					localHallOrder = newWorldView.OrderView[floor][button]

					// Check if incoming has max version
					if incomingHallOrder.Version >= HALL_SYNC_BARRIER {
						peersAtHallSyncBarrier[floor][button][incomingWorldView.PeerID] = true
					} else {
						delete(peersAtHallSyncBarrier[floor][button], incomingWorldView.PeerID)
					}
					// Check if we have max version
					if localHallOrder.Version >= HALL_SYNC_BARRIER {
						peersAtHallSyncBarrier[floor][button][myID] = true
					} else {
						delete(peersAtHallSyncBarrier[floor][button], myID)
					}
					// Reset version to 0 once all known peers are at barrier
					if localHallOrder.Version >= HALL_SYNC_BARRIER {
						allAtHallSyncBarrier := true
						for _, peerID := range activePeersList {
							if !peersAtHallSyncBarrier[floor][button][peerID] {
								allAtHallSyncBarrier = false
								break
							}
						}
						if allAtHallSyncBarrier {
							newWorldView.OrderView[floor][button].Version = 0
							peersAtHallSyncBarrier[floor][button] = make(map[string]bool)
						}
					}

				}
			}
			myWorldView = newWorldView

		case cabAck := <-cabAckRx:
			if cabAck.OwnerID != myID {
				break
			}

			if cabAck.Floor < 0 || cabAck.Floor >= elevator.N_FLOORS {
				break
			}

			localCabOrder := myWorldView.OrderView[cabAck.Floor][elevator.N_BUTTONS-1]

			if localCabOrder.Placed != cabAck.Placed {
				break
			}

			// Another peer acknowledges one of our cab orders
			myWorldView.OrderView[cabAck.Floor][elevator.N_BUTTONS-1].AckList = addAck(myWorldView.OrderView[cabAck.Floor][elevator.N_BUTTONS-1].AckList, cabAck.AckerID)

		case <-ticker.C:
			networkTx <- myWorldView

			// Non-blocking select
			select {
			case backupStore <- myWorldView:
			default:
				// Replace old backup with the latest version
				<-backupStore
				backupStore <- myWorldView
			}
		}

		if confirmedPlacementsChanged(myWorldView, lastConfirmedPlacements, activePeersList) || shouldReassign {

			newConfirmedPlacements := extractConfirmedPlacements(myWorldView, lastConfirmedPlacements, activePeersList)

			// Only confirmed placements are used for side effects
			assignOrders(myID, myWorldView.ElevatorState, newConfirmedPlacements, peerStates, peerCabOrders, outOfServicePeersList, assignEvent)
			setLights <- newConfirmedPlacements

			lastConfirmedPlacements = newConfirmedPlacements
			shouldReassign = false
		}
	}
}

func assignOrders(myID string, myState elevator.Elevator, placements [elevator.N_FLOORS][elevator.N_BUTTONS]bool, peerStates map[string]elevator.Elevator, peerCabOrders map[string][elevator.N_FLOORS]Order, outOfServiceList []string, assignEvent chan<- [elevator.N_FLOORS][elevator.N_BUTTONS]bool) {

	hraInput := hallRequestAssigner.HRAInput{
		HallRequests: make([][elevator.N_BUTTONS - 1]bool, elevator.N_FLOORS),
		States:       make(map[string]hallRequestAssigner.HRAElevState),
	}

	input := [][elevator.N_BUTTONS - 1]bool{}

	for floor := 0; floor < elevator.N_FLOORS; floor++ {
		temp := [elevator.N_BUTTONS - 1]bool{}
		for button := 0; button < (elevator.N_BUTTONS - 1); button++ {
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

	// If we are in the case of being alone, but also unable to service orders,
	// we must take the assignment anyway, and just not be able to service it
	if len(allServicableElevatorIDs) == 0 {
		assignEvent <- placements
		return
	}

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
		// Convert elevator.Behaviour [int] to hra.behaviour [string]
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

func addAck(ackList []string, id string) []string {
	if !slices.Contains(ackList, id) {
		return append(ackList, id)
	}
	return ackList
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

func confirmedPlacementsChanged(newWorldView WorldView, lastConfirmedPlacements [elevator.N_FLOORS][elevator.N_BUTTONS]bool, activePeersList []string) bool {
	return extractConfirmedPlacements(newWorldView, lastConfirmedPlacements, activePeersList) != lastConfirmedPlacements

}

// This function sets the placements for all the orders that have a full ackList,
// and uses the lastConfirmedPlacements for all the rest (That dont have a full ackList)
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
