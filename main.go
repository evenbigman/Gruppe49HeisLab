package main
import (
	backup "sanntidslab/backup_handler"
	"fmt"
	"log"
	"maps"
	"sanntidslab/config"
	"sanntidslab/controller"
	hallrequestassigner "sanntidslab/hall_request_assigner"
	"sanntidslab/peers"
	"sanntidslab/peers/snapshots"
)
func main() {
	backup.Init()

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
	myElevatorState := ec.GetElevatorValues()
	pm.SetMySnapshot(myElevatorState)

	cabButtonCh := ec.SubscribeCabButtons()
	stateCh := ec.SubscribeElevator()

	ec.Start()
	log.Println("Started elevator controller")
	
	// Take cab orders that you previously had before crashing
	if err == nil {
		elevator := ec.GetElevatorValues()
		ec.SetCabOrders(elevator.PressedCabButtons)
		elevator.CabOrders = elevator.PressedCabButtons
		pm.SetMySnapshot(elevator)
	}
	
	for {
		select {
		case <-pm.UnconfirmedOrderChangeCh: //someone presses a hall button
			orders := pm.GetUnconfirmedOrders()
			ec.SetPressedHallButtons(orders)
			state := ec.GetElevatorValues()
			mustAssignHallOrders(pm, ec, state)

		case <-pm.ConfirmedOrderChangeCh: //hall button is seen by every peer
			orders := pm.GetConfirmedOrders()
			ec.SetConfirmedHallOrders(orders)
			state := ec.GetElevatorValues()
			mustAssignHallOrders(pm, ec, state)

		case <-pm.DisconnectedPeerCh:
			state := ec.GetElevatorValues()
			mustAssignHallOrders(pm, ec, state)

		case <-cabButtonCh:
			log.Println("Cab press")
			if pm.ImOnline() {
				go func() { //Wait for ack
					stateToAck := ec.GetElevatorValues()
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
				state := ec.GetElevatorValues()
				ec.SetCabOrders(state.PressedCabButtons)
				state.CabOrders = state.PressedCabButtons
				pm.SetMySnapshot(state)
			}
		case <-stateCh:
			state := ec.GetElevatorValues()
			pm.SetMySnapshot(state)
			mustAssignHallOrders(pm, ec, state)
		}
	}
}

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

	filteredOrders := andHallOrders(myOrders, state.PressedHallButtons)
	ec.AssignHallOrders(filteredOrders)
	return nil
}

func snapshotsForAssignment(pm *peers.PeerManager, myID uint64, mySnapshot snapshots.Snapshot_t) map[uint64]snapshots.Snapshot_t {
	allSnapshotsByID := maps.Clone(pm.GetConnectedSnapshots())
	if allSnapshotsByID == nil {
		allSnapshotsByID = make(map[uint64]snapshots.Snapshot_t, 1)
	}
	allSnapshotsByID[myID] = mySnapshot
	return allSnapshotsByID
}

func andHallOrders(a, b controller.HallOrders_t) hallrequestassigner.HallAssignment_t {
	var result hallrequestassigner.HallAssignment_t
	for floor := 0; floor < config.NumFloors; floor++ {
		for dir := 0; dir < 2; dir++ {
			result[floor][dir] = a[floor][dir] && b[floor][dir]
		}
	}
	return result
}
