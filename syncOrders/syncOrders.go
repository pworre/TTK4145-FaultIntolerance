package syncOrders

import (
	"elevatorControl/elevator"
	"elevatorControl/hallRequestAssigner"
	"elevator_project/config"
	"fmt"
	"log"
	"networkDriver/bcast"
	"networkDriver/peers"
	"slices"
	"time"
)

// TODO: Consider moving the order struct out into syncOrders, and not have it be part of the elevator.
// TODO: Almost certain this is better, it shouldnt take too long, and it would massively simplify assignOrders

type WorldView struct {
	PeerID        string                            `json:"peerID"`
	ElevatorState elevator.Elevator                 `json:"elevatorState"`
	CabOrders     [elevator.N_FLOORS]elevator.Order `json:"cabOrders"`
}

// TODO: Some form of counter or AckList to make sure that we transistion from the barrier correctly.
// TODO: Should only come into play after very many iterations, though

const BARRIER = 60000

const TRANSMIT_INTERVAL = 500 * time.Millisecond

const G_BCAST_PORT = 40104

func SynchronizationLoop(startFloor int, cfg config.Config, localStateChange chan elevator.Elevator, assignEvent chan [elevator.N_FLOORS][elevator.N_BUTTONS]elevator.Order, localRequest chan elevator.ButtonEvent, localClearing chan elevator.ButtonEvent, peerUpdate chan peers.PeerUpdate, setLights chan [elevator.N_FLOORS][elevator.N_BUTTONS]bool) {

	myID := cfg.ID
	myWorldView := WorldView{
		PeerID:        myID,
		ElevatorState: elevator.NewStartElevator(startFloor),
		CabOrders:     [elevator.N_FLOORS]elevator.Order{},
	}
	lastWorldView := myWorldView

	activePeersList := []string{}
	peerStates := make(map[string]elevator.Elevator)
	peerCabOrders := make(map[string][elevator.N_FLOORS]elevator.Order)
	newConfirmedPlacements := [elevator.N_FLOORS][elevator.N_BUTTONS]bool{}
	lastConfirmedPlacements := newConfirmedPlacements

	// Channels, routines and timer for periodically broadcasting and receiving WorldViews between peers
	networkRx := make(chan WorldView, 1024)
	networkTx := make(chan WorldView, 1024)

	go bcast.Transmitter(G_BCAST_PORT, networkTx)
	go bcast.Receiver(G_BCAST_PORT, networkRx)

	ticker := time.NewTicker(TRANSMIT_INTERVAL)
	defer ticker.Stop()

	for {
		select {
		case request := <-localRequest:
			myWorldView.ElevatorState.Requests[request.Floor][request.Button].Placed = true
			myWorldView.ElevatorState.Requests[request.Floor][request.Button].Unknown = false
			myWorldView.ElevatorState.Requests[request.Floor][request.Button].AckList = []string{myID}
			if request.Button == elevator.B_Cab {
				myWorldView.ElevatorState.Requests[request.Floor][request.Button].Version = BARRIER
			} else {
				myWorldView.ElevatorState.Requests[request.Floor][request.Button].Version += 1
			}

		case request := <-localClearing:
			myWorldView.ElevatorState.Requests[request.Floor][request.Button].Placed = false
			myWorldView.ElevatorState.Requests[request.Floor][request.Button].Unknown = false
			myWorldView.ElevatorState.Requests[request.Floor][request.Button].AckList = []string{myID}
			if request.Button == elevator.B_Cab {
				myWorldView.ElevatorState.Requests[request.Floor][request.Button].Version = BARRIER
			} else {
				myWorldView.ElevatorState.Requests[request.Floor][request.Button].Version += 1
			}

		case newLocalState := <-localStateChange:
			myWorldView.ElevatorState.Floor = newLocalState.Floor
			myWorldView.ElevatorState.Direction = newLocalState.Direction
			myWorldView.ElevatorState.Behaviour = newLocalState.Behaviour
			myWorldView.CabOrders = elevator.ExtractCabOrders(newLocalState.Requests)

			/*
				// Needed to maintain the global worldview of orders and not just the ones we were assigned and then cleared
				for floor := 0; floor < elevator.N_FLOORS; floor++ {
					for button := 0; button < elevator.N_BUTTONS; button++ {
						myWorldView.ElevatorState.Requests[floor][button].Placed = oldWorldView.ElevatorState.Requests[floor][button].Placed
					}
				}

				myWorldView.CabOrders[myID] = elevator.ExtractCabOrders(newLocalState.Requests)
				log.Println("Local state change reached syncOrders!")

				// ? HMMM, is this correct? Should probably send it to the other elevators first!!!
				if needToAssignAgain(myWorldView, oldWorldView) {
					assignOrders(myWorldView, peerStates, assignEvent)
					setLights <- elevator.ExtractOrderPlacementTable(myWorldView.ElevatorState.Requests)
				}
			*/

		case newPeerUpdate := <-peerUpdate:
			activePeersList = newPeerUpdate.Peers
			for _, lostID := range newPeerUpdate.Lost {
				delete(peerStates, lostID)
			}

			if len(activePeersList) == 0 {
				//networkDisconnect <- true // ? Maybe this is not needed at all...?
				for floor := 0; floor < elevator.N_FLOORS; floor++ {
					for button := 0; button < elevator.N_BUTTONS; button++ {
						myWorldView.ElevatorState.Requests[floor][button].Unknown = true
					}
				}
			}

			for _, str := range activePeersList {
				log.Printf("Peer number: %s", str)
			}

			if needToAssignAgain(myWorldView, lastWorldView, lastConfirmedPlacements, activePeersList) {

				// This function sets the placements for all the orders that have a full AckList,
				// and uses the lastConfirmedPlacements for all the rest (That dont have a full AckList)
				newConfirmedPlacements = extractConfirmedPlacements(myWorldView, lastConfirmedPlacements, activePeersList)

				// TODO: This is ugly, and can be fixed when we mode the order struct out from the elevator struct and into syncOrders
				confirmedOrders := [elevator.N_FLOORS][elevator.N_BUTTONS]elevator.Order{}
				for floor := 0; floor < elevator.N_FLOORS; floor++ {
					for button := 0; button < elevator.N_BUTTONS; button++ {
						confirmedOrders[floor][button].Placed = true
					}
				}

				confirmedElevatorState := myWorldView.ElevatorState
				confirmedElevatorState.Requests = confirmedOrders

				confirmedWorldView := WorldView{
					PeerID:        myID,
					ElevatorState: confirmedElevatorState,
					CabOrders:     myWorldView.CabOrders,
				}

				assignOrders(confirmedWorldView, peerStates, peerCabOrders, assignEvent)
				setLights <- newConfirmedPlacements

				lastConfirmedPlacements = newConfirmedPlacements
			}

		case incomingWorldView := <-networkRx:
			log.Printf("Decoded worldview before filtering: %+v", incomingWorldView)

			// Ignore my own rebroadcasts
			if incomingWorldView.PeerID == myID {
				break
			}

			// Ignore messages if we have not yet gotten the peerUpdate
			if !slices.Contains(activePeersList, incomingWorldView.PeerID) {
				log.Printf("Discarded worldview from %q because activePeersList=%v",
					incomingWorldView.PeerID, activePeersList)
				break
			}

			fmt.Printf("I am receiving a worldview from someone else: %+v", incomingWorldView)

			// Be aware! We need this for the hallrequestassigner, but we should not trust their requests as our own,
			// they are only meant to be used as inputs for the HRA
			peerStates[incomingWorldView.PeerID] = incomingWorldView.ElevatorState

			newWorldView := myWorldView

			// TODO: CabOrder Acklist updating logic
			// Caborder updating
			unknownCount := 0
			for floor := 0; floor < elevator.N_FLOORS; floor++ {
				incomingCabOrder := incomingWorldView.ElevatorState.Requests[floor][elevator.N_BUTTONS-1]
				if incomingCabOrder.Unknown {
					unknownCount++
					continue
				} else {
					if newWorldView.ElevatorState.Requests[floor][elevator.N_BUTTONS-1].Unknown {
						newWorldView.ElevatorState.Requests[floor][elevator.N_BUTTONS-1] = incomingCabOrder
					}
				}
			}
			// Slightly ugly, but necessary maintaining of the CabOrders map in myWorldView
			if unknownCount > 0 && unknownCount != elevator.N_FLOORS {
				log.Println("Something weird has happened, either all or no orders should be unknown at the same time")
			}
			if unknownCount == 0 {
				peerCabOrders[incomingWorldView.PeerID] = elevator.ExtractCabOrders(incomingWorldView.ElevatorState.Requests)
			}

			// Hallorder synchronization
			for floor := 0; floor < elevator.N_FLOORS; floor++ {
				for button := 0; button < (elevator.N_BUTTONS - 1); button++ {
					incomingHallOrder := incomingWorldView.ElevatorState.Requests[floor][button]
					if incomingHallOrder.Unknown {
						continue
					}

					localHallOrder := newWorldView.ElevatorState.Requests[floor][button]

					if localHallOrder.Unknown {
						// Trust incomingOrder, and set your own order identical
						newWorldView.ElevatorState.Requests[floor][button] = incomingHallOrder
						// These two are included in the one above
						//newWorldView.ElevatorState.Requests[floor][button].AckList = incomingHallOrder.AckList
						//newWorldView.ElevatorState.Requests[floor][button].Unknown = false
					} else {
						if incomingHallOrder.Version >= BARRIER {
							newWorldView.ElevatorState.Requests[floor][button].Placed = incomingHallOrder.Placed
							newWorldView.ElevatorState.Requests[floor][button].AckList = incomingHallOrder.AckList
							newWorldView.ElevatorState.Requests[floor][button].Version = BARRIER
							// TODO: Some kind of barrier counting logic, to get all peers to the barrier
						} else if localHallOrder.Version > incomingHallOrder.Version {
							// Ignore orders coming in that are older than you
							log.Println("I ignored someone else")
							continue

						} else if localHallOrder.Version == incomingHallOrder.Version {
							// This will in the worst case do an order twice, but never miss an order
							log.Println("I made a compromize with someone else")
							newWorldView.ElevatorState.Requests[floor][button].Placed = (localHallOrder.Placed || incomingHallOrder.Placed)
							// Before we were in the middle of either clearing an order or adding one,
							// we have now started either a new adding or a new clearing, and must clear the acklist and increment the version
							newWorldView.ElevatorState.Requests[floor][button].Version += 1
							newWorldView.ElevatorState.Requests[floor][button].AckList = []string{myID}

						} else if localHallOrder.Version < incomingHallOrder.Version {
							// Accept orders that are newer than us
							log.Println("I got convinced by someone else")
							newWorldView.ElevatorState.Requests[floor][button].Placed = incomingHallOrder.Placed
							newWorldView.ElevatorState.Requests[floor][button].Version = incomingHallOrder.Version
							newWorldView.ElevatorState.Requests[floor][button].AckList = elevator.MergeAckLists(incomingHallOrder.AckList, []string{myID})
							newWorldView.ElevatorState.Requests[floor][button].Version += 1 // ! Needed because we have the merged AckList now
						}

					}

				}
			}
			// Counter will be different here, so must compare orders and possibly elevatorstates, not counters
			if needToAssignAgain(newWorldView, myWorldView, lastConfirmedPlacements, activePeersList) {

				// This function sets the placements for all the orders that have a full AckList,
				// and uses the lastConfirmedPlacements for all the rest (That dont have a full AckList)
				newConfirmedPlacements = extractConfirmedPlacements(newWorldView, lastConfirmedPlacements, activePeersList)

				// TODO: This is ugly, and can be fixed when we mode the order struct out from the elevator struct and into syncOrders
				confirmedOrders := [elevator.N_FLOORS][elevator.N_BUTTONS]elevator.Order{}
				for floor := 0; floor < elevator.N_FLOORS; floor++ {
					for button := 0; button < elevator.N_BUTTONS; button++ {
						confirmedOrders[floor][button].Placed = true
					}
				}

				confirmedElevatorState := newWorldView.ElevatorState
				confirmedElevatorState.Requests = confirmedOrders

				confirmedWorldView := WorldView{
					PeerID:        myID,
					ElevatorState: confirmedElevatorState,
					CabOrders:     newWorldView.CabOrders,
				}

				assignOrders(confirmedWorldView, peerStates, peerCabOrders, assignEvent)
				setLights <- newConfirmedPlacements

				lastConfirmedPlacements = newConfirmedPlacements
			}

			// Even if no new assignment was needed, the version number might still be updated, so we must update state
			myWorldView = newWorldView
			lastWorldView = myWorldView

		case <-ticker.C:
			// Send myWorldView
			networkTx <- myWorldView
			log.Println("Hopefully sent something!")
		}
	}
}

func assignOrders(myWorldView WorldView, peerStates map[string]elevator.Elevator, peerCabOrders map[string][elevator.N_FLOORS]elevator.Order, assignEvent chan<- [elevator.N_FLOORS][elevator.N_BUTTONS]elevator.Order) {

	log.Println("Entering assignOrders")
	myID := myWorldView.PeerID

	hraInput := hallRequestAssigner.HRAInput{
		HallRequests: make([][elevator.N_BUTTONS - 1]bool, elevator.N_FLOORS),
		States:       make(map[string]hallRequestAssigner.HRAElevState),
	}

	placements := elevator.ExtractHallOrderPlacements(myWorldView.ElevatorState.Requests)
	input := [][elevator.N_BUTTONS - 1]bool{}

	for floor := 0; floor < elevator.N_FLOORS; floor++ {
		temp := [elevator.N_BUTTONS - 1]bool{}
		for button := 0; button < elevator.N_BUTTONS-1; button++ {
			temp[button] = placements[floor][button]
		}
		input = append(input, temp)
	}
	hraInput.HallRequests = input

	// Active peerIDs pluss myself as input for hallRequestAssigner
	allElevatorIDs := []string{myID}
	for peerID, _ := range peerStates {
		allElevatorIDs = append(allElevatorIDs, peerID)
	}

	allElevatorStates := make(map[string]elevator.Elevator)
	for id, state := range peerStates {
		allElevatorStates[id] = state
	}
	allElevatorStates[myID] = myWorldView.ElevatorState

	allCabOrders := make(map[string][elevator.N_FLOORS]elevator.Order)
	for id, cabOrderList := range peerCabOrders {
		allCabOrders[id] = cabOrderList
	}
	allCabOrders[myID] = elevator.ExtractCabOrders(myWorldView.ElevatorState.Requests)

	for _, ID := range allElevatorIDs {
		// convert elevator.behaviour [int] to hra.behaviour [string]
		var elevBehaviour_hra string
		switch allElevatorStates[ID].Behaviour {
		case elevator.EB_Idle:
			elevBehaviour_hra = "idle"
		case elevator.EB_Moving:
			elevBehaviour_hra = "moving"
		case elevator.EB_DoorOpen:
			elevBehaviour_hra = "doorOpen"
		}

		var elevDirection_hra string
		switch allElevatorStates[ID].Direction {
		case elevator.D_Up:
			elevDirection_hra = "up"
		case elevator.D_Stop:
			elevDirection_hra = "stop"
		case elevator.D_Down:
			elevDirection_hra = "down"
		}

		input := []bool{}
		//placements := elevator.ExtractCabOrderPlacements(allElevatorStates[ID].Requests)

		for floor := 0; floor < elevator.N_FLOORS; floor++ {
			input = append(input, allCabOrders[ID][floor].Placed)
		}
		cabRequests_hra := input

		hraInput.States[ID] = hallRequestAssigner.HRAElevState{
			Behavior:    elevBehaviour_hra,
			Floor:       allElevatorStates[ID].Floor,
			Direction:   elevDirection_hra,
			CabRequests: cabRequests_hra,
		}

	}

	newAssignmentMap := hallRequestAssigner.Decode(hallRequestAssigner.AssignOrders(hallRequestAssigner.Encode(hraInput)))

	newAssignmentPlacements := newAssignmentMap[myID]
	newAssignment := [elevator.N_FLOORS][elevator.N_BUTTONS]elevator.Order{}

	for floor := 0; floor < elevator.N_FLOORS; floor++ {
		for button := 0; button < elevator.N_BUTTONS; button++ {
			newAssignment[floor][button].Placed = newAssignmentPlacements[floor][button]
			newAssignment[floor][button].Version = myWorldView.ElevatorState.Requests[floor][button].Version
			newAssignment[floor][button].Unknown = false
		}
	}

	log.Printf("assignOrders: worldview placements = %+v", elevator.ExtractOrderPlacementTable(myWorldView.ElevatorState.Requests))
	log.Printf("assignOrders: assignment for %s = %+v", myID, newAssignment)

	assignEvent <- newAssignment
}

func needToAssignAgain(newWorldView WorldView, oldWorldView WorldView, lastConfirmedPlacements [elevator.N_FLOORS][elevator.N_BUTTONS]bool, activePeersList []string) bool {
	return extractConfirmedPlacements(newWorldView, lastConfirmedPlacements, activePeersList) != extractConfirmedPlacements(oldWorldView, lastConfirmedPlacements, activePeersList)

}

// This function sets the placements for all the orders that have a full AckList,
// and uses the lastConfirmedPlacements for all the rest (That dont have a full AckList)
func extractConfirmedPlacements(newWorldView WorldView, lastConfirmedPlacements [elevator.N_FLOORS][elevator.N_BUTTONS]bool, activePeersList []string) [elevator.N_FLOORS][elevator.N_BUTTONS]bool {
	newConfirmedPlacements := [elevator.N_FLOORS][elevator.N_BUTTONS]bool{}

	requiredAcks := append([]string{}, activePeersList...)
	requiredAcks = append(requiredAcks, newWorldView.PeerID)

	for floor := 0; floor < elevator.N_FLOORS; floor++ {
		for button := 0; button < elevator.N_BUTTONS; button++ {
			order := newWorldView.ElevatorState.Requests[floor][button]

			if elevator.ContainSameElements(order.AckList, requiredAcks) {
				newConfirmedPlacements[floor][button] = order.Placed
			} else {
				newConfirmedPlacements[floor][button] = lastConfirmedPlacements[floor][button]
			}
		}
	}
	return newConfirmedPlacements
}
