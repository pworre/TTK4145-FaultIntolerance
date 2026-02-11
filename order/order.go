package order

import (
	"elevatorControl/elevator"
	"elevatorDriver/elevio"
	"networkDriver/bcast"
	"fmt"
	"log"
	"order"
	"elevator_project/config"
)

// ? Peer routing table [1, 2, 3, 4, ..., n] - Makes order of who transmits to who

type orderType int 
const (
	HALL = 0
	CAB = 1
)

type currentOrderState int
const (
	COS_UNKNOWN = -1
	COS_NONE = 0
	COS_UNCONFIRMED_REQUEST = 1
	COS_CONFIRMED_REQUEST = 2
	COS_UNCONFIRMED_DELETION = 3 
	COS_CONFIRMED_DELETION = 4
)

type Order struct {
	ID					string
	OrderType 			orderType
	OrderFloor			int
	CurrentOrderState 	currentOrderState
}

type OrderNetworkMsg struct {
	PeerID				string
	OrderToSyncMap		map[string]Order
	OrdersConfirmed		[]Order
	StateCounter		uint64
}

const bcast_PORT = 25532

func orderSync(ordersConfirmed chan []Order, orderSyncBuffer chan Order, buttonEvent <-chan elevio.ButtonEvent, reachFloorEvent <-chan int) {
	cfg := config.ParseFlag()
	myID := cfg.ID
	
	networkRx := make(chan OrderNetworkMsg, 1024)
	networkTx := make(chan OrderNetworkMsg, 1024)
	
	go bcast.Transmitter(bcast_PORT, networkTx)
	go bcast.Receiver(bcast_PORT, networkRx)
	
	orderToSync := Order{
		ID:					myID,
		OrderType: 			HALL,
		OrderFloor: 		-1,
		CurrentOrderState: 	COS_NONE,
	}

	// ! TEMPORARY VARIABLES FOR CODING ONLY
	orderList := make([]Order, 0)
	currentOrder := Order{}
	receivedOrdersToSync := make(map[string][]Order) // This is a map with key as PeerID with Order
	nextFloor := 4

	orderToSyncMap := make(map[string]Order)
	orderToSyncMap[myID] = orderToSync

	orderNetworkMsg := OrderNetworkMsg{
		PeerID: 			myID, 
		OrderToSyncMap:		orderToSyncMap,
		OrdersConfirmed: 	nil,
		StateCounter: 		0,
	}


	for {
		select {
		case buttonPressed := <-buttonEvent:
			orderToAdd := Order{
				OrderType: 			orderType(buttonPressed.Button),
				OrderFloor: 		buttonPressed.Floor,
				CurrentOrderState: 	COS_UNCONFIRMED_REQUEST,
			}
			orderSyncBuffer <-orderToAdd

		case floorToRemove := <-reachFloorEvent:
			if floorToRemove == nextFloor {
				orderToRemove := Order{
					OrderType: 			currentOrder.OrderType,
					OrderFloor: 		floorToRemove,
					CurrentOrderState: 	COS_UNCONFIRMED_DELETION,
				}
				orderSyncBuffer <-orderToRemove
			}

		case orderToHandle := <-orderSyncBuffer:
			if orderToHandle.CurrentOrderState == COS_UNCONFIRMED_REQUEST {
				// TODO: Send orderToSync on network (NOT CONFIRMED_LIST!)

			}
			if orderToHandle.CurrentOrderState == COS_UNCONFIRMED_DELETION {
				// TODO: Send orderToSync on network
				orderNetworkMsg.OrderToSync = orderToHandle
				orderNetworkMsg.StateCounter += 1
				networkTx <-orderNetworkMsg
			}

		case networkOrders := <-ordersConfirmed:
			// TODO: Add 'hallassigner' to choose next to do

		case msgReceived := <-networkRx:
			// TODO: Save to a map
			receivedOrdersToSync[msgReceived.PeerID] = msgReceived.OrderToSyncMap
			// ! PROBLEMET NÅ ER AT VI PRØVER Å ASSIGNE MAP TIL ET MAP. HER MÅ NOE FIKSES OG HA OVERSIKT OVER HVEM SOM HAR NYEST AV HVA!!!
		}
	}
}












type globalOrders struct {
	// STRUCT OF MAP:  [floor : Direction]
	hallOrders map[int]elevator.MotorDirection

	// STRUCT OF MAP: 	[ID of responsible elev : floor]
	cabOrders map[string]int
}

type msgState struct {
	GlobalID 		int
	TimeStamp 		uint64
	ElevState 		elevator.Elevator
	GlobalOrders	globalOrders
}

func sync_orders() {
	
}
