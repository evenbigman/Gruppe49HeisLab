package main

//BUG: Hall orders get assigned even though offline
//BUG: Goes through roof
//BUG: Hall orders assigned to where you are standing from others, will not get removed when taken (on the others side)

import (
	//	backup "sanntidslab/backup_handler"

	"fmt"
	"log"
	"maps"
	"sanntidslab/config"
	"sanntidslab/controller"
	hallrequestassigner "sanntidslab/hall_request_assigner"
	"sanntidslab/peers"
	"sanntidslab/peers/snapshots"
)

func mustAssignHallOrders(pm *peers.PeerManager, ec *controller.ElevatorController, state controller.Elevator) {
	if err := assignHallOrders(pm, ec, state); err != nil {
		panic(fmt.Errorf("assignHallOrders failed: %w", err))
	}
}

func assignHallOrders(pm *peers.PeerManager, ec *controller.ElevatorController, state controller.Elevator) error {
	mySnapshot, err := pm.GetMySnapshot()
	if err != nil {
		return fmt.Errorf("could not get my snapshot for hall assignment: %w", err)
	}

	if mySnapshot.Elevator.State == controller.Obstructed {
		return nil
	}

	myID := peers.GetMyID()
	allSnapshotsByID := snapshotsForAssignment(pm, myID, mySnapshot)

	myOrders, err := hallrequestassigner.GetAssignedHallRequests(state.ConfirmedHallOrders, allSnapshotsByID, myID)
	if err != nil {
		return fmt.Errorf("could not get hall assignments: %w", err)
	}

	convertedOrders := toHallOrders(myOrders)
	filteredOrders := andHallOrders(convertedOrders, state.PressedHallButtons)
	ec.AssignHallOrders(filteredOrders)
	return nil
}

func snapshotsForAssignment(pm *peers.PeerManager, myID uint64, mySnapshot snapshots.Snapshot) map[uint64]snapshots.Snapshot {
	allSnapshotsByID := maps.Clone(pm.GetConnectedSnapshots())
	if allSnapshotsByID == nil {
		allSnapshotsByID = make(map[uint64]snapshots.Snapshot, 1)
	}
	allSnapshotsByID[myID] = mySnapshot
	return allSnapshotsByID
}

func toHallOrders(orders hallrequestassigner.HallAssignment_t) [config.NumFloors][2]bool {
	var convertedOrders [config.NumFloors][2]bool
	for floor, floorOrders := range orders {
		convertedOrders[floor] = floorOrders
	}
	return convertedOrders
}

func andHallOrders(a, b [config.NumFloors][2]bool) [config.NumFloors][2]bool {
	var result [config.NumFloors][2]bool
	for floor := 0; floor < config.NumFloors; floor++ {
		for dir := 0; dir < 2; dir++ {
			result[floor][dir] = a[floor][dir] && b[floor][dir]
		}
	}
	return result
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
			mustAssignHallOrders(pm, ec, state)

		case <-pm.ConfirmedOrderChangeCh:
			orders := pm.GetConfirmedOrders()
			ec.SetGlobalHallOrders(orders)
			state := ec.GetElevatorState()
			mustAssignHallOrders(pm, ec, state)

		case <-pm.DisconnectedPeerCh:
			state := ec.GetElevatorState()
			mustAssignHallOrders(pm, ec, state)

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
			mustAssignHallOrders(pm, ec, state)
		}
	}
}
