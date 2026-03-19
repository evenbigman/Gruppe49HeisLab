package snapshots

//TODO: Add logging
//TODO: Add proper error handling
//TODO: Add subscriber model to get snapshot changes of peers on a channel
//TODO: Prevent integer overflow on version number
//TODO: Refactor MergeSnapshots, fix abstraction layer together with peers
//TODO: GetORDERS which exist wwhile you havent joined
//TODO: Make deepcopy for functions which export snapshots

import (
	//"log"
	"sanntidslab/config"
	"sanntidslab/controller"
	"sync"
)

type Snapshot struct {
	Version  int
	Elevator controller.Elevator
}
type SnapshotManager struct {
	mutex     sync.RWMutex
	snapshots map[uint64]Snapshot
	myID      uint64
}

var (
	instance *SnapshotManager
	once sync.Once
)

func GetSnapshotManager(myID uint64) *SnapshotManager {
	sm := &SnapshotManager{
		myID:      myID,
		snapshots: make(map[uint64]Snapshot),
	}

	once.Do(func() {
		instance = sm
	})
	return instance
}

// Takes incoming state, updates if necessary. And returnsed lowest version they have of our state
func (sm *SnapshotManager) MergeSnapshots(incomingSnapshots map[uint64]Snapshot) (ackedVersion int) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	ackedVersion = 0

	for rcvdID, rcvdSnapshot := range incomingSnapshots {
		if rcvdID == sm.myID {
			_, mySnapshotExists := sm.snapshots[sm.myID]
			if mySnapshotExists {
				ackedVersion = rcvdSnapshot.Version
			} else {
				sm.snapshots[sm.myID] = rcvdSnapshot
			}
			continue
		}

		storedSnapshot, elevatorIsStored := sm.snapshots[rcvdID]
		if !elevatorIsStored ||
			rcvdSnapshot.Version > storedSnapshot.Version {
			sm.snapshots[rcvdID] = rcvdSnapshot
		}
	}
	return ackedVersion
}

func (sm *SnapshotManager) SetSnapshot(ID uint64, version int, elevator controller.Elevator) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	sm.snapshots[ID] = Snapshot{
		Version:  version,
		Elevator: elevator,
	}
}

func (sm *SnapshotManager) GetSnapshots() map[uint64]Snapshot {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	output := make(map[uint64]Snapshot, len(sm.snapshots))
	for id, storedSnapshot := range sm.snapshots {
		output[id] = storedSnapshot
	}
	return output
}

func (sm *SnapshotManager) ComputeOrders(oldSnapshots map[uint64]Snapshot, connectedIds []uint64) [config.NumFloors][2]bool {

	newSnapshots := sm.GetSnapshots()

	var orders [config.NumFloors][2]bool

	//Check for change in orders in this priority:
	// rising edge > falling edge > constant high > constant low
	// no previous order into high/low means rising/falling respectively
	// MAYBE BUG: when joining network
	for i := range orders{
		for j := range orders[i]{
			changed := false
			for _, id := range connectedIds{
				oldSnapshot, oldSnapshotExists := oldSnapshots[id]
				newSnapshot, _ := newSnapshots[id]

				newOrder := newSnapshot.Elevator.PressedHallButtons[i][j]
				if !oldSnapshotExists {
					changed = true
					orders[i][j] = newOrder
				} else {
					oldOrder := oldSnapshot.Elevator.PressedHallButtons[i][j]

					if oldOrder != newOrder{
						changed = true
						if newOrder{
							orders[i][j] = true
							break //rising edge, set order true -- move to next direction
						} else {
							orders[i][j] = false
						}
					} else if newOrder == true && changed == false{
						orders[i][j] = true	
					}
				}
			}
		}
	}
	//log.Println()
	return orders
}
