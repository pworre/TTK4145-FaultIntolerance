package syncOrders

import (
	"elevatorControl/elevator"
	"testing"
	"time"
)

// Helper functions
func createEmptyWorldView(id string, startFloor int) WorldView {
	return WorldView{
		PeerID:        id,
		ElevatorState: elevator.NewStartElevator(startFloor),
		CabOrders:     [elevator.N_FLOORS]elevator.Order{},
	}
}

func TestAssignOrders_TimoutTest(t *testing.T) {
	worldView := createEmptyWorldView("testPeer", 0)
	var nilPeerStates map[string]elevator.Elevator
	var nilPeerCabOrders map[string][elevator.N_FLOORS]elevator.Order

	assignEvent := make(chan [elevator.N_FLOORS][elevator.N_BUTTONS]elevator.Order, 1)

	done := make(chan struct{})

	go func() {
		assignOrders(worldView, nilPeerStates, nilPeerCabOrders, assignEvent)
		close(done)
	}()

	select {
	case <-done:
		//Test went ok
	case <-time.After(2 * time.Second):
		t.Fatal("timeOut")
	}
}
