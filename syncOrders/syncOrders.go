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

type WorldView struct {
	PeerID        string                                       `json:"peerID"`
	ElevatorState elevator.Elevator                            `json:"elevatorState"`
	CabOrders     map[string][elevator.N_FLOORS]elevator.Order `json:"cabOrders"`
}

// !!! IMPORTANT INFO !!!

// TODO: This will probably work, but we never explicitly check a barrier before transitioning to tuning on the lights,
// TODO: like we said we would in the progress report... The barrier here is only to avoid the super annoying resetting all the time,
// TODO: not for confirming that everyone has confirmed an order. However, I think that this solution is much better to work with,
// TODO: and the next step will probably be deciding a barrier state for confirmation,
// TODO: and then sending around an ackList in the WorldView for checking if everyone agrees,
// TODO: and NOTTTTTT!!!!!!! using the seperate counting threads!!!! Those were a nightmare...
// TODO: With this implementation, I actually think its not too much work to add an acklist for every single order in the map (its only 12 orders),
// TODO: and then checking if we are at barrier, confirming, and resetting the barrier will be a piece of cake!!!
// TODO: Those are the next steps, I think...

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
		CabOrders:     make(map[string][elevator.N_FLOORS]elevator.Order),
	}

	activePeersList := []string{}
	peerStates := make(map[string]elevator.Elevator)

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
			if request.Button != elevator.B_Cab {
				myWorldView.ElevatorState.Requests[request.Floor][request.Button].Version += 1
			}
			assignOrders(myWorldView, peerStates, assignEvent)
			setLights <- elevator.ExtractOrderPlacementTable(myWorldView.ElevatorState.Requests)

		case request := <-localClearing:
			myWorldView.ElevatorState.Requests[request.Floor][request.Button].Placed = false
			myWorldView.ElevatorState.Requests[request.Floor][request.Button].Unknown = false
			if !(request.Button == elevator.B_Cab) {
				myWorldView.ElevatorState.Requests[request.Floor][request.Button].Version += 1
			}
			assignOrders(myWorldView, peerStates, assignEvent)
			setLights <- elevator.ExtractOrderPlacementTable(myWorldView.ElevatorState.Requests)

		case newLocalState := <-localStateChange:
			myWorldView.ElevatorState.Floor = newLocalState.Floor
			myWorldView.ElevatorState.Direction = newLocalState.Direction
			myWorldView.ElevatorState.Behaviour = newLocalState.Behaviour
			myWorldView.CabOrders[myID] = elevator.ExtractCabOrders(newLocalState.Requests)

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

			assignOrders(myWorldView, peerStates, assignEvent)

		case incomingWorldView := <-networkRx:
			log.Printf("Decoded worldview before filtering: %+v", incomingWorldView)

			// Ignore messages if we have not yet gotten the peerUpdate
			if !slices.Contains(activePeersList, incomingWorldView.PeerID) {
				log.Printf("Discarded worldview from %q because activePeersList=%v",
					incomingWorldView.PeerID, activePeersList)
				break
			}
			// We throw away messages from ourselves
			if myID == incomingWorldView.PeerID {
				break
			}
			fmt.Printf("I am receiving a worldview from someone else: %+v", incomingWorldView)

			// Be aware! We need this for the hallrequestassigner, but we should not trust their requests as our own,
			// they are only meant to be used as inputs for the HRA
			peerStates[incomingWorldView.PeerID] = incomingWorldView.ElevatorState

			newWorldView := myWorldView

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
				newWorldView.CabOrders[incomingWorldView.PeerID] = elevator.ExtractCabOrders(incomingWorldView.ElevatorState.Requests)
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
						newWorldView.ElevatorState.Requests[floor][button].Unknown = false
					} else {
						if incomingHallOrder.Version >= BARRIER {
							newWorldView.ElevatorState.Requests[floor][button].Placed = incomingHallOrder.Placed
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

						} else if localHallOrder.Version < incomingHallOrder.Version {
							// Accept orders that are newer than us
							log.Println("I got convinced by someone else")
							newWorldView.ElevatorState.Requests[floor][button].Placed = incomingHallOrder.Placed
							newWorldView.ElevatorState.Requests[floor][button].Version = incomingHallOrder.Version
						}

					}

				}
			}
			// Counter will be different here, so must compare orders and possibly elevatorstates, not counters
			if needToAssignAgain(newWorldView, myWorldView) {

				assignOrders(newWorldView, peerStates, assignEvent)
				setLights <- elevator.ExtractOrderPlacementTable(newWorldView.ElevatorState.Requests)
			}

			// Even if no new assignment was needed, the version number might still be updated, so we must update state
			myWorldView = newWorldView

		case <-ticker.C:
			// Send myWorldView
			networkTx <- myWorldView
			log.Println("Hopefully sent something!")
		}
	}
}

func assignOrders(myWorldView WorldView, peerStates map[string]elevator.Elevator, assignEvent chan<- [elevator.N_FLOORS][elevator.N_BUTTONS]elevator.Order) {

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
		placements := elevator.ExtractCabOrderPlacements(allElevatorStates[ID].Requests)
		for floor := 0; floor < elevator.N_FLOORS; floor++ {
			input = append(input, placements[floor])
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

func needToAssignAgain(newWorldView WorldView, oldWorldView WorldView) bool {
	if elevator.ExtractCabOrderPlacements(newWorldView.ElevatorState.Requests) != elevator.ExtractCabOrderPlacements(oldWorldView.ElevatorState.Requests) {
		return true
	}
	if elevator.ExtractHallOrderPlacements(newWorldView.ElevatorState.Requests) != elevator.ExtractHallOrderPlacements(oldWorldView.ElevatorState.Requests) {
		return true
	}
	return false
}

/*
func networkEncode(input WorldView) []byte {

	jsonBytes, err := json.Marshal(input)
	if err != nil {
		log.Println("json.Marshal error: ", err)
	}
	return jsonBytes
}

func networkDecode(out []byte) WorldView {

	var incomingMsg WorldView
	err := json.Unmarshal(out, &incomingMsg)
	if err != nil {
		log.Println("json.Unmarshal error: ", err)
	}
	return incomingMsg
}
*/
