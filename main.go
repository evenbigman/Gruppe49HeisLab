package main

import (
	//	backup "sanntidslab/backup_handler"
	"encoding/json"
	"log"
	"sanntidslab/config"
	"sanntidslab/controller"
	"sanntidslab/peers"
)

func assignHallOrders(pm *peers.PeerManager, ec *controller.ElevatorController, state *controller.Elevator) {

	var myOrders [config.NumFloors][2]bool
	for i := range myOrders {
		for j := range myOrders[i] {
			myOrders[i][j] = state.ConfirmedHallOrders[i][j] || state.PressedHallButtons[i][j]
		}
	}

	peerOrders := pm.GetOrders()

	combinedOrders := [config.NumFloors][2]bool{}
	for i := range combinedOrders {
		for j := range combinedOrders[i] {
			combinedOrders[i][j] = myOrders[i][j] || peerOrders[i][j]
		}
	}

	ec.SetGlobalHallOrders(combinedOrders)

	b, _ := json.Marshal(combinedOrders)
	log.Printf("Assigned hall orders: %s", string(b))

	ec.AssignHallOrders(myOrders)
}

func main() {
	//	backup.Init()

	pm := peers.NewPeerManager()

	pm.Init()
	go pm.Run()

	ec := controller.GetController()
	startState, err := pm.WaitForStartSnapshot()

	if err == nil {
		ec.InitElevatorWithStates(startState)
	} else {
		log.Println("Started fresh elevator")
		ec.InitElevator()
	}
	myElevatorState := ec.GetElevatorState()
	pm.SetMySnapshot(myElevatorState)

	buttonCh := ec.SubscribeButtons()
	stateCh := ec.SubscribeState()

	ec.Start()
	log.Println("Started elevator controller")

	for {
		//snapshot, _ := pm.GetMySnapshot()
		//log.Println("My snappshot:", snapshot.Elevator.ConfirmedHallOrders)
		select {
		case <-pm.OrderChangeCh:
			orders := pm.GetOrders()
			b, _ := json.Marshal(orders)
			log.Printf("Orders: %s", string(b))
			ec.SetGlobalHallOrders(orders)
			state := ec.GetElevatorState()
			assignHallOrders(pm, ec, &state)

		case <-pm.DisconnectedPeerCh:
			state := ec.GetElevatorState()
			assignHallOrders(pm, ec, &state)

		case <-buttonCh:
			if pm.ImOnline() {
				go func() { //Wait for ack
					stateToAck := ec.GetElevatorState()
					err := pm.WaitForAck(stateToAck, config.TimeoutAck)
					if err != nil {
					} else {
						ec.SetGlobalHallOrders(stateToAck.PressedHallButtons)
						ec.SetCabOrders(stateToAck.PressedCabButtons)
						stateToAck.ConfirmedHallOrders = stateToAck.PressedHallButtons
						stateToAck.CabOrders = stateToAck.PressedCabButtons
						pm.SetMySnapshot(stateToAck)
					}
				}()
			} else { //Go solo
				state := ec.GetElevatorState()
				ec.SetCabOrders(state.PressedCabButtons)
				state.CabOrders = state.PressedCabButtons
				pm.SetMySnapshot(state)
			}
		case <-stateCh:
			state := ec.GetElevatorState()
			pm.SetMySnapshot(state)
			assignHallOrders(pm, ec, &state)
		}
	}
	//	case <-buttonCh:
	//		stateToAck := ec.GetElevatorState()
	//		go func() {
	//			err := pm.WaitForAck(stateToAck, config.TimeoutAck)
	//			if err != nil {
	//			} else {
	//				ec.SetGlobalHallOrders(stateToAck.PressedHallButtons)
	//				// TODO: Add hallrequest assigner

	//				// START OF HALL REQUEST ASSIGNER STUFF
	//				mySnapshot, _ := pm.GetMySnapshot()
	//				connectedSnapshots := pm.GetConnectedSnapshots()

	//				allSnapshots := make([]snapshots.Snapshot, 0, 1+len(connectedSnapshots))
	//				allSnapshots = append(allSnapshots, mySnapshot)            // first element
	//				allSnapshots = append(allSnapshots, connectedSnapshots...) // rest

	//				elevatorsSnapshot := hallrequestassigner.ElevatorsSnapshot{
	//					HallCalls: stateToAck.ConfirmedHallOrders,
	//					Snapshot:  allSnapshots,
	//				}

	//				hallAssignments, _ := hallrequestassigner.AssignHallRequests(elevatorsSnapshot)

	//				myOrders := hallAssignments["id_1"]

	//				ec.AssignHallOrders(myOrders)
	//				// END OF HALL REQUEST ASSIGNER STUFF

	//				ec.SetCabOrders(stateToAck.PressedCabButtons)
	//			}
	//		}()
	//	case <-stateCh:
	//		state := ec.GetElevatorState()
	//		pm.SetMySnapshot(state)
	//	}
	//}
}
