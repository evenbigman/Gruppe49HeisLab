package main

import (
	//	backup "sanntidslab/backup_handler"
	"sanntidslab/config"
	"sanntidslab/controller"
	hallrequestassigner "sanntidslab/hall_request_assigner"
	"sanntidslab/peers"
	"sanntidslab/peers/snapshots"
)

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
		ec.InitElevator()
	}

	buttonCh := ec.SubscribeButtons()
	stateCh := ec.SubscribeState()

	ec.Start()

	for {
		select {
		case <-pm.NewOrderCh:
		case <-pm.DisconnectedPeerCh:
		case <-buttonCh:
			stateToAck := ec.GetElevatorState()
			go func() {
				err := pm.WaitForAck(stateToAck, config.TimeoutAck)
				if err != nil {
				} else {
					ec.SetGlobalHallOrders(stateToAck.PressedHallButtons)
					// TODO: Add hallrequest assigner

					// START OF HALL REQUEST ASSIGNER STUFF
					mySnapshot, _ := pm.GetMySnapshot()
					connectedSnapshots := pm.GetConnectedSnapshots()

					allSnapshots := make([]snapshots.Snapshot, 0, 1+len(connectedSnapshots))
					allSnapshots = append(allSnapshots, mySnapshot)            // first element
					allSnapshots = append(allSnapshots, connectedSnapshots...) // rest

					elevatorsSnapshot := hallrequestassigner.ElevatorsSnapshot{
						HallCalls: stateToAck.ConfirmedHallOrders,
						Snapshot:  allSnapshots,
					}

					hallAssignments, _ := hallrequestassigner.AssignHallRequests(elevatorsSnapshot)

					myOrders := hallAssignments["id_1"]

					ec.AssignHallOrders(myOrders)
					// END OF HALL REQUEST ASSIGNER STUFF

					ec.SetCabOrders(stateToAck.PressedCabButtons)
				}
			}()
		case <-stateCh:
			state := ec.GetElevatorState()
			pm.SetMySnapshot(state)
		}
	}
}
