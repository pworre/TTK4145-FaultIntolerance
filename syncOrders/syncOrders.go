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

// ! IMPORTANT !
// TODO: At the moment, the syscall restart immediately restarts the program when we get an inactivityTimeout.
// TODO: This happens so fast that the other elevators never get time to register the elevator as lost before it comes straight back again,
// TODO: which means that we never save a backup, and the new instance starts with fresh, empty orders...
// TODO: We need to find a fix for this, either storing the last state locally (but then need several copies),
// TODO: or waiting for peers to save our backup before we can restart.
// TODO: We can also solve this with a process peers backup that we continually write our state to such that it can take over,
// TODO: that is what MArius and Eskil does, but we must see what is best for us...

type Order struct {
	Placed  bool
	Version uint16
	AckList []string
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

const HALL_SYNC_BARRIER = 70

const TRANSMIT_INTERVAL = 50 * time.Millisecond

const G_BCAST_PORT = 40104
const CAB_ACK_BCAST_PORT = 40105
const CAB_RESTORE_BCAST_PORT = 40106

func SynchronizationLoop(startFloor int, cfg config.Config, localStateChange chan elevator.Elevator,
	assignEvent chan [elevator.N_FLOORS][elevator.N_BUTTONS]bool, localRequest chan elevator.ButtonEvent,
	localClearing chan elevator.ButtonEvent, peerUpdate chan peers.PeerUpdate, setLights chan [elevator.N_FLOORS][elevator.N_BUTTONS]bool,
	activePeersChan chan []string, inactivityTimeout chan bool, restart chan bool, stillActive chan bool, worldViewCh chan WorldView, syncRestoreWorldViewCh chan WorldView) {

	myID := cfg.ID
	myWorldView := WorldView{
		PeerID:        myID,
		ElevatorState: elevator.NewStartElevator(startFloor),
		OrderView:     [elevator.N_FLOORS][elevator.N_BUTTONS]Order{},
	}

	peersAtHallSyncBarrier := [elevator.N_FLOORS][elevator.N_BUTTONS - 1]map[string]bool{}
	for floor := 0; floor < elevator.N_FLOORS; floor++ {
		for button := 0; button < elevator.N_BUTTONS-1; button++ {
			peersAtHallSyncBarrier[floor][button] = make(map[string]bool)
		}
	}

	activePeersList := []string{}
	lostPeersList := []string{}
	outOfServicePeersList := []string{}
	peerStates := make(map[string]elevator.Elevator)
	peerCabOrders := make(map[string][elevator.N_FLOORS]Order)
	lostPeerBackupStates := make(map[string]WorldView)

	newConfirmedPlacements := [elevator.N_FLOORS][elevator.N_BUTTONS]bool{}
	lastConfirmedPlacements := newConfirmedPlacements
	shouldReassign := false

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
				myWorldView.OrderView[request.Floor][request.Button].AckList = []string{myID}
			} else {
				myWorldView.OrderView[request.Floor][request.Button].Placed = true
				myWorldView.OrderView[request.Floor][request.Button].AckList = []string{myID}
				myWorldView.OrderView[request.Floor][request.Button].Version += 1
			}

		case request := <-localClearing:
			if request.Button == elevator.B_Cab {
				myWorldView.OrderView[request.Floor][request.Button].Placed = false
				myWorldView.OrderView[request.Floor][request.Button].AckList = []string{myID}
			} else {
				myWorldView.OrderView[request.Floor][request.Button].Placed = false
				myWorldView.OrderView[request.Floor][request.Button].AckList = []string{myID}
				myWorldView.OrderView[request.Floor][request.Button].Version += 1
			}

		case newLocalState := <-localStateChange:
			myWorldView.ElevatorState = newLocalState
			if !slices.Contains(outOfServicePeersList, myID) {
				outOfServicePeersList = append(outOfServicePeersList, myID)
			}

			shouldReassign = true

		case <-inactivityTimeout:

			// Can only restart if we are not alone
			if len(activePeersList) > 0 {
				log.Printf("OS exit should be done here")
				log.Printf("allPeersStillActive: %v", activePeersList)
				restart <- true
			} else {
				log.Println("Dude, is this what happens???")
				log.Println("PeersStillActive:", activePeersList)
				stillActive <- true
			}

		case newPeerUpdate := <-peerUpdate:
			//inactivityTimeout <- true
			oldActivePeersList := activePeersList
			newActivePeersList := newPeerUpdate.Peers

			if len(newActivePeersList) != len(oldActivePeersList) {
				shouldReassign = true
			}

			log.Printf("Old peer list: %v", oldActivePeersList)
			log.Printf("New peer list: %v", newActivePeersList)

			for _, id := range newActivePeersList {
				// Check if new peer
				if !slices.Contains(oldActivePeersList, id) {
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
				}

				lostPeersList = append(lostPeersList, lostID)

				// Update barrier maps
				for floor := 0; floor < elevator.N_FLOORS; floor++ {
					for button := 0; button < elevator.N_BUTTONS-1; button++ {
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

				for floor := 0; floor < elevator.N_FLOORS; floor++ {
					for button := 0; button < elevator.N_BUTTONS; button++ {
						newWorldView.OrderView[floor][button].AckList = addAck(newWorldView.OrderView[floor][button].AckList, myID)
					}
				}

				myWorldView = newWorldView
			}

			for _, str := range activePeersList {
				log.Printf("Peer number: %s", str)
			}

		case restore := <-cabRestoreRx:
			if restore.TargetID != myID {
				break
			}

			localCabOrders := extractCabOrders(myWorldView.OrderView)

			for floor := 0; floor < elevator.N_FLOORS; floor++ {
				restoredCabOrder := restore.CabOrders[floor]
				localCabOrder := localCabOrders[floor]

				restoredCabOrder.Placed = (restoredCabOrder.Placed || localCabOrder.Placed)
				restoredCabOrder.AckList = []string{myID}
				myWorldView.OrderView[floor][elevator.N_BUTTONS-1] = restoredCabOrder
			}

		case restored := <-syncRestoreWorldViewCh:
			if restored.PeerID != myID {
				log.Printf("Very weird, I got a process peers restore from a different process")
				break
			}
			log.Printf("Restoring worldview from processPairs for peer %s", restored.PeerID)

			myWorldView = restored

			for floor := 0; floor < elevator.N_FLOORS; floor++ {
				myWorldView.OrderView[floor][elevator.N_BUTTONS-1].AckList = addAck(myWorldView.OrderView[floor][elevator.N_BUTTONS-1].AckList, myID)
				lastConfirmedPlacements[floor][elevator.N_BUTTONS-1] = myWorldView.OrderView[floor][elevator.N_BUTTONS-1].Placed
			}

			shouldReassign = true

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

			// Peer states for hall request assigner
			peerStates[incomingWorldView.PeerID] = incomingWorldView.ElevatorState

			isPeerOutOfService := incomingWorldView.ElevatorState.OutOfService
			if isPeerOutOfService {
				if !slices.Contains(outOfServicePeersList, incomingWorldView.PeerID) {
					outOfServicePeersList = append(outOfServicePeersList, incomingWorldView.PeerID)
					shouldReassign = true
				}
			} else {
				// Remove from outOfServicePeersList if incoming is no longer out of service
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

			newWorldView := myWorldView

			// Caborder acknowledging
			incomingCabOrders := extractCabOrders(incomingWorldView.OrderView)

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

					// Debug
					if floor == 0 && button == 0 {
						log.Println("Incoming hallorder version:", incomingHallOrder.Version)
						log.Println("Local hallorder version:", localHallOrder.Version)
					}

					if localHallOrder.Version < 30 && incomingHallOrder.Version >= HALL_SYNC_BARRIER {
						// Either we have just wrapped and are waiting for the others, or we are a new peer just joining
						// In case we are a new peer, we must take OR
						// This will in the worst case do an order twice, but never miss an order
						log.Println("I am either a new peer joining or have just wrapped waiting for others")
						newWorldView.OrderView[floor][button].Placed = (localHallOrder.Placed || incomingHallOrder.Placed)
						// Before we were in the middle of either clearing an order or adding one,
						// we have now started either a new adding or a new clearing, and must clear the acklist and increment the version
						newWorldView.OrderView[floor][button].Version += 1
						newWorldView.OrderView[floor][button].AckList = []string{myID}

						//continue
					} else if localHallOrder.Version >= HALL_SYNC_BARRIER && incomingHallOrder.Version < 30 {
						// Accept incoming orders that have just wrapped if we have not wrapped yet, and then we also wrap
						log.Println("I accept a peer that has wrapped, and now wrap myself!")
						newWorldView.OrderView[floor][button].Placed = incomingHallOrder.Placed
						newWorldView.OrderView[floor][button].Version = incomingHallOrder.Version
						newWorldView.OrderView[floor][button].AckList = mergeAckLists(incomingHallOrder.AckList, []string{myID})
						peersAtHallSyncBarrier[floor][button] = make(map[string]bool)

					} else {
						if localHallOrder.Version > incomingHallOrder.Version {
							// Ignore orders coming in that are older than you
							log.Println("I ignored someone else")
							//continue

						} else if localHallOrder.Version == incomingHallOrder.Version {
							if localHallOrder.Placed == incomingHallOrder.Placed {
								//newWorldView.OrderView[floor][button].Placed = localHallOrder.Placed
								//newWorldView.OrderView[floor][button].Version = localHallOrder.Version
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

					// Barrier wrap logic
					incomingHallOrder = incomingWorldView.OrderView[floor][button]
					localHallOrder = newWorldView.OrderView[floor][button]

					// Check if incoming has max Version
					if incomingHallOrder.Version >= HALL_SYNC_BARRIER {
						peersAtHallSyncBarrier[floor][button][incomingWorldView.PeerID] = true
					} else {
						delete(peersAtHallSyncBarrier[floor][button], incomingWorldView.PeerID)
					}
					// Check if we have max Version
					if localHallOrder.Version >= HALL_SYNC_BARRIER {
						peersAtHallSyncBarrier[floor][button][myID] = true
					} else {
						delete(peersAtHallSyncBarrier[floor][button], myID)
					}
					// Wrap Version to 0 once all known peers are at barrier
					if localHallOrder.Version >= HALL_SYNC_BARRIER {
						allAtHallSyncBarrier := true
						for _, peerID := range activePeersList {
							if !peersAtHallSyncBarrier[floor][button][peerID] {
								allAtHallSyncBarrier = false
								break
							}
						}
						if allAtHallSyncBarrier {
							log.Println("I have counted it is safe to reset")
							newWorldView.OrderView[floor][button].Version = 0
							peersAtHallSyncBarrier[floor][button] = make(map[string]bool)
						}
					}

				}
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

			if localCabOrder.Placed != cabAck.Placed {
				break
			}

			myWorldView.OrderView[cabAck.Floor][elevator.N_BUTTONS-1].AckList = addAck(myWorldView.OrderView[cabAck.Floor][elevator.N_BUTTONS-1].AckList, cabAck.AckerID)

		case <-ticker.C:
			// Send myWorldView
			networkTx <- myWorldView
			//log.Println("Hopefully sent something!")

			// needed for non-blocking
			select {
			case worldViewCh <- myWorldView:
			default:
				// Replace old version in buffer
				<-worldViewCh
				worldViewCh <- myWorldView
			}
		}

		if confirmedPlacementsChanged(myWorldView, lastConfirmedPlacements, activePeersList) || shouldReassign {

			newConfirmedPlacements := extractConfirmedPlacements(myWorldView, lastConfirmedPlacements, activePeersList)

			assignOrders(myID, myWorldView.ElevatorState, newConfirmedPlacements, peerStates, peerCabOrders, outOfServicePeersList, assignEvent)
			setLights <- newConfirmedPlacements

			lastConfirmedPlacements = newConfirmedPlacements
			shouldReassign = false
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

	// If we are in the case of being alone, but also unable to service orders,
	// we must take the assignment anyway, and just not be able to service it
	if len(allServicableElevatorIDs) == 0 {
		assignEvent <- placements
		return
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

func confirmedPlacementsChanged(newWorldView WorldView, lastConfirmedPlacements [elevator.N_FLOORS][elevator.N_BUTTONS]bool, activePeersList []string) bool {
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
