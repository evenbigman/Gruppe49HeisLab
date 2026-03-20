package main

//BUG: Hall orders get assigned even though offline
//BUG: Goes through roof
//BUG: Hall orders assigned to where you are standing from others, will not get removed when taken (on the others side)

import (
	//	backup "sanntidslab/backup_handler"

	"log"
	"maps"
	"sanntidslab/config"
	"sanntidslab/controller"
	hallrequestassigner "sanntidslab/hall_request_assigner"
	"sanntidslab/peers"
	"sanntidslab/peers/snapshots"
)

func assignHallOrders(pm *peers.PeerManager, ec *controller.ElevatorController, state *controller.Elevator) {
	ms, _ := pm.GetMySnapshot()
	if ms.Elevator.State == controller.Obstructed {
		return
	}
	myID := peers.GetMyID()

	msById := make(map[uint64]snapshots.Snapshot)
	msById[myID] = ms

	csByID := pm.GetConnectedSnapshots()

	allSnapshotsByID := make(map[uint64]snapshots.Snapshot, len(csByID)+1)
	maps.Copy(allSnapshotsByID, msById)
	maps.Copy(allSnapshotsByID, csByID)

	myOrders, err := hallrequestassigner.GetAssignedHallRequests(state.ConfirmedHallOrders, allSnapshotsByID, myID)
	if err != nil {
		log.Printf("error assigning hall requests: %v", err)
		panic("could not get hall assignments")
	}

	// Convert HallAssignment to controller's HallOrders type by copying values
	var convertedOrders [config.NumFloors][2]bool
	for i, order := range myOrders {
		convertedOrders[i] = order
	}

	var filteredOrders [config.NumFloors][2]bool
	for floor := 0; floor < config.NumFloors; floor++ {
		for dir := 0; dir < 2; dir++ {
			filteredOrders[floor][dir] = convertedOrders[floor][dir] && state.PressedHallButtons[floor][dir]
		}
	}

	ec.AssignHallOrders(filteredOrders)
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
}
