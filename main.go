package main

import (
//	backup "sanntidslab/backup_handler"
	"sanntidslab/peers"
	"log"
)

func main() {
//	backup.Init()

	pm := peers.NewPeerManager()

	pm.Init()
	go pm.Run()

	log.Println("Started peermanager")
//
//	time.Sleep(config.InitDelay)
//
//	ec := controller.GetController()
//	startSnapshot, err := pm.GetMySnapshot()
//	if err == nil {
//		ec.InitElevatorWithStates(startSnapshot.Elevator)
//	} else {
//		log.Println("Started fresh elevator")
//		ec.InitElevator()
//	}
//
//	buttonCh := ec.SubscribeButtons()
//	stateCh := ec.SubscribeState()
//
//	ec.Start()
//	log.Println("Started elevator controller")
//
	for {
		select {
		case <-pm.NewOrderCh:
		case <-pm.DisconnectedPeerCh:
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
