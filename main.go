package main

//BUG: Hall orders get assigned even though offline
//BUG: Goes through roof
//BUG: Hall orders assigned to where you are standing from others, will not get removed when taken (on the others side)

import (
	//	backup "sanntidslab/backup_handler"

	"fmt"
	"log"
	"sanntidslab/config"
	"sanntidslab/controller"
	hallrequestassigner "sanntidslab/hall_request_assigner"
	"sanntidslab/peers"
	"sanntidslab/peers/snapshots"
	"sort"
)

func assignHallOrders(pm *peers.PeerManager, ec *controller.ElevatorController, state *controller.Elevator) {
	mySnapshot, _ := pm.GetMySnapshot()
	if mySnapshot.Elevator.State == controller.Obstructed {
		return
	}
	connectedSnapshots := pm.GetConnectedSnapshots()
	myID := peers.GetMyID()

	snapshotByID := make(map[uint64]snapshots.Snapshot, len(connectedSnapshots)+1)
	snapshotByID[myID] = mySnapshot
	for id, snapshot := range connectedSnapshots {
		if snapshot.Elevator.State != controller.Obstructed {
			snapshotByID[id] = snapshot
		}
	}

	ids := make([]uint64, 0, len(snapshotByID))
	for id := range snapshotByID {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	myIndex := -1
	for i, id := range ids {
		if id == myID {
			myIndex = i
			break
		}
	}
	if myIndex == -1 {
		log.Printf("could not find my id %d in sorted ids", myID)
		return
	}

	allSnapshots := make([]snapshots.Snapshot, 0, len(ids))
	for _, id := range ids {
		allSnapshots = append(allSnapshots, snapshotByID[id])
	}

	log.Println("My elevator: ")
	log.Println(mySnapshot)

	log.Println("Connected elevators: ")
	log.Println(connectedSnapshots)

	elevatorsSnapshot := hallrequestassigner.ElevatorsSnapshot{
		HallCalls: state.ConfirmedHallOrders,
		Snapshot:  allSnapshots,
	}

	hallAssignments, err := hallrequestassigner.AssignHallRequests(elevatorsSnapshot)
	if err != nil {
		log.Printf("error assigning hall requests: %v", err)
		panic("could not get hall assignments")
	}

	assignmentKey := fmt.Sprintf("id_%d", myIndex+1)
	myOrders, ok := hallAssignments[assignmentKey]
	if !ok {
		log.Printf("missing hall assignment for %s", assignmentKey)
		return
	}

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

	cabButtonCh := ec.SubscribeCabButtons()
	stateCh := ec.SubscribeState()

	ec.Start()
	log.Println("Started elevator controller")

	for {
		//snapshot, _ := pm.GetMySnapshot()
		//log.Println("My snappshot:", snapshot.Elevator.ConfirmedHallOrders)
		select {
		case <-pm.UnconfirmedOrderChangeCh:
			orders := pm.GetUnconfirmedOrders()
			ec.SetPressedHallButtons(orders)
			state := ec.GetElevatorState()
			assignHallOrders(pm, ec, &state)

		case <-pm.ConfirmedOrderChangeCh:
			orders := pm.GetConfirmedOrders()
			ec.SetGlobalHallOrders(orders)
			state := ec.GetElevatorState()
			assignHallOrders(pm, ec, &state)

		case <-pm.DisconnectedPeerCh:
			state := ec.GetElevatorState()
			assignHallOrders(pm, ec, &state)

		case <-cabButtonCh:
			log.Println("Cab press")
			if pm.ImOnline() {
				go func() { //Wait for ack
					stateToAck := ec.GetElevatorState()
					err := pm.WaitForAck(stateToAck, config.TimeoutAck)
					if err != nil {
						log.Println(err)
					} else {
						ec.SetCabOrders(stateToAck.PressedCabButtons)
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
			//Blir knapper satt her?
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
