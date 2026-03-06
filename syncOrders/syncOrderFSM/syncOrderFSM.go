package syncOrderFSM

import (
	"syncOrders"
	"log"
)

// Finite state machine loop

func StateMachineLoop(chans chan bool) {
	localOrderToSyncMap := map[string]Order{}

	//localOrderToSync := syncOrders.Order{
	//PeerID:            myID,
	//OrderType:         elevator.B_Cab,
	//OrderFloor:        -1,
	//CurrentOrderState: COS_NONE,
	//}

// TODO: Complete implementation of the barrier state counting, spawn correct number of threads in main
// TODO: For a peerUpdate, we need to update the localOrderToSyncMap


	for {
		select {
		case <-networkDisconnect:
			for ID, _ := range localOrderToSyncMap {
				localOrderToSyncMap[ID].CurrentOrderState = COS_UNKNOWN
			}

		case incomingOrderToSyncMap := copyMap(<-orderToSyncMapMessage): // Deep Copy
			for incomingID, incomingincomingOrderToSync := range incomingOrderToSyncMap {
				switch incomingOrderToSync.CurrentOrderState {
				case COS_NONE:

					switch localOrderToSyncMap[incomingID].CurrentOrderState {
					case COS_CONFIRMED_REQUEST:
						iAgree <- myID
						// Add to agreelist

					case COS_CONFIRMED_DELETION:
						iAgree <- myID
						// Add to agreelist

					default:

					}

				
				case COS_UNCONFIRMED_REQUEST:
					
					switch localOrderToSyncMap[incomingID].CurrentOrderState {
					case COS_NONE:
						localOrderToSyncMap[incomingID].CurrentOrderState = COS_UNCONFIRMED_REQUEST

					default:

					}
				
				case COS_UNCONFIRMED_DELETION:

					switch localOrderToSyncMap[incomingID].CurrentOrderState {
					case COS_NONE:
						localOrderToSyncMap[incomingID].CurrentOrderState = COS_UNCONFIRMED_DELETION

					default:

					}
				
				case COS_CONFIRMED_REQUEST:
					incomingConfirmedRequest(incomingOrderToSync.PeerID)

					switch localOrderToSyncMap[incomingID].CurrentOrderState {
					case COS_UNCONFIRMED_REQUEST:
						localOrderToSyncMap[incomingID].CurrentOrderState = COS_CONFIRMED_REQUEST

					default:

					}
				
				case COS_CONFIRMED_DELETION:
					incomingConfirmedDeletion(incomingOrderToSync.PeerID)
					
					switch localOrderToSyncMap[incomingID].CurrentOrderState {
					case COS_UNCONFIRMED_DELETION:
						localOrderToSyncMap[incomingID].CurrentOrderState = COS_CONFIRMED_DELETION

					default:

					}

				}
		}
		
		case <-allAgreeToAddOrder:
			// Add confirmed order, turn on lights
			// ! Double-check that the order has state completed
			confirmedRequest <- localOrderToSyncMap[incomingID]
			localOrderToSyncMap[incomingID].CurrentOrderState = COS_NONE
		
		case <-allAgreeToDeleteOrder:
			// Remove completed order, turn off lights
			// ! Double-check that the order has state completed
			confirmedDeletion <- localOrderToSyncMap[incomingID]
			localOrderToSyncMap[incomingID].CurrentOrderState = COS_NONE
		}
		youCanTransmitNow <- copyMap(localOrderToSyncMap) // Deep copy of map to be sent
	}
}

func doAllElevatorsAgree(allElevatorsThatAgree []string) bool {
	if fullpeerlist == allElevatorsThatAgree {
		return true
	} else {
		return false
	}
}

func confirmedRequestOrderPeerCounter(incomingConfirmedRequest chan string, incomingConfirmedDelete chan string, allAgreeToConfirm chan bool) {
	peersThatHaveConfirmedRequest := []string //length peerlist

	for {
		select {
		case <-peerUpdate:
			peersThatHaveConfirmedRequest = []string //length peerlist

		case peerID := <-incomingConfirmedRequest:
			peersThatHaveConfirmedRequest[peerID] = true

		}
		if allElevatorAgree(peersThatHaveConfirmedRequest) {
			allAgreeToAddOrder <- true
			peersThatHaveConfirmedRequest := []string //length peerlist
		}
	}
}

func confirmedDeletionOrderPeerCounter(incomingConfirmedRequest chan string, incomingConfirmedDelete chan string, allAgreeToDelete chan bool) {
	peersThatHaveConfirmedDeletion := []string //length peerlist

	for {
		select {
		case <-peerUpdate:
			peersThatHaveConfirmedDeletion = []string //length peerlist
		
		case peerID := <-incomingConfirmedDelete:
			peersThatHaveConfirmedDeletion[peerID] = true

		}
		if allElevatorAgree(peersThatHaveConfirmedDeletion) {
			allAgreeToDeleteOrder <- true
			peersThatHaveConfirmedDeletion := []string //length peerlist
		}
	}
}

func copyMap(oldMap map[string]Order) map[string]Order {
    newMap := make(map[string]Order, len(oldMap))
    for key, value := range oldMap {
        newMap[key] = value
    }
    return newMap
}