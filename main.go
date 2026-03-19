package main

import (
	//	backup "sanntidslab/backup_handler"

	"log"
	"sanntidslab/config"
	"sanntidslab/controller"
	hallrequestassigner "sanntidslab/hall_request_assigner"
	"sanntidslab/peers"
	"sanntidslab/peers/snapshots"
)

func assignHallOrders(pm *peers.PeerManager, ec *controller.ElevatorController, state *controller.Elevator) {
	mySnapshot, _ := pm.GetMySnapshot()
	connectedSnapshots := pm.GetConnectedSnapshots()

	allSnapshots := make([]snapshots.Snapshot, 0, 1+len(connectedSnapshots))
	allSnapshots = append(allSnapshots, mySnapshot)
	allSnapshots = append(allSnapshots, connectedSnapshots...)

	log.Println("My elevator: ")
	log.Println(mySnapshot)

	log.Println("Connected elevators: ")
	log.Println(connectedSnapshots)

	elevatorsSnapshot := hallrequestassigner.ElevatorsSnapshot{
		HallCalls: state.PressedHallButtons,
		Snapshot:  allSnapshots,
	}

	hallAssignments, err := hallrequestassigner.AssignHallRequests(elevatorsSnapshot)
	if err != nil {
		log.Printf("error assigning hall requests: %v", err)
		panic("could not get hall assignments")
	}

	myOrders := hallAssignments["id_1"]

	ec.AssignHallOrders(myOrders)
}

func main() {
	//	backup.Init()

	pm := peers.GetPeerManager()

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
						log.Println(err)
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
