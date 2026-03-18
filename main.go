package main

import (
	//	backup "sanntidslab/backup_handler"
	"encoding/json"
	"log"
	"sanntidslab/config"
	"sanntidslab/controller"
	"sanntidslab/peers"
	"time"
)

func main() {
	//	backup.Init()

	pm := peers.NewPeerManager()

	pm.Init()
	go pm.Run()

	log.Println("Started peermanager")

	time.Sleep(config.InitDelay)

	ec := controller.GetController()
	startSnapshot, err := pm.GetMySnapshot()
	if err == nil {
		ec.InitElevatorWithStates(startSnapshot.Elevator)
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
		case <-pm.DisconnectedPeerCh:
		case <-buttonCh:
			go func() {
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
		case <-stateCh:
			state := ec.GetElevatorState()
			pm.SetMySnapshot(state)
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
