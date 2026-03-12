package snapshots
//TODO: Add proper error handling
//TODO: Add subscriber model to get snapshot changes of peers on a channel

import(
	"sanntidslab/controller"
	"sync"
)

type Snapshot struct{
	Version int
	Elevator controller.Elevator
}
type SnapshotManager struct{
	mutex sync.RWMutex
	elevators map[string]Snapshot
	myID string
}

func NewSnapshotManager(myID string, initialElevator controller.Elevator) *SnapshotManager{
	sm := &SnapshotManager{
		myID: myID,
		elevators: make(map[string]Snapshot),
	}	
	
	sm.elevators[myID] = Snapshot{
		Version: 0,
		Elevator: initialElevator,
	}

	return sm
}

//Takes incoming state, updates if necessary
func (sm *SnapshotManager) MergeSnapshots(incomingSnapshots map[string]Snapshot){
	sm.mutex.Lock()
	defer sm.mutex.Unlock()
	
	for rcvdID, rcvdSnapshot := range incomingSnapshots{
		if rcvdID == sm.myID{
			continue
		}
		
		storedSnapshot, elevatorIsStored := sm.elevators[rcvdID]
		if !elevatorIsStored ||
		rcvdSnapshot.Version > storedSnapshot.Version {
			sm.elevators[rcvdID] = rcvdSnapshot
		}
	}
}

func (sm *SnapshotManager) UpdateLocalSnapshot(localElevator controller.Elevator) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	previousSnapshot := sm.elevators[sm.myID]
	sm.elevators[sm.myID] = Snapshot{
		Version: previousSnapshot.Version + 1,
		Elevator: localElevator,
	}
}

func (sm *SnapshotManager) GetSnapshots() map[string]Snapshot{
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()
	
	output := make(map[string]Snapshot, len(sm.elevators))
	for id, storedSnapshot := range sm.elevators {
		output[id] = storedSnapshot 
	}
	return output
}
