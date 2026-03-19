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
	once     sync.Once
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

// Takes incoming state, updates if necessary. Also checks if new order has come :O And returnsed lowest version they have of our state
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

func (sm *SnapshotManager) ComputeHallOrders() [config.NumFloors][2]bool {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	var orders [config.NumFloors][2]bool

	for _, snapshot := range sm.snapshots {
		for i := range orders {
			for j := range orders[i] {
				if // snapshot.Elevator.ConfirmedHallOrders[i][j] ||
				snapshot.Elevator.PressedHallButtons[i][j] {
					orders[i][j] = true
				}
			}
		}
	}
	//log.Println()
	return orders
}
