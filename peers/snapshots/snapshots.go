package snapshots

import(
	"sanntidslab/controller"
	"time"
	"log"
	"sync"
)


/* Snapshot MANAGER
Type contains: map of all elevator snapshots, mutex for it, its own ID
Functions:
1. Init
2. Merge changes received from internet
3. Merge own changes received from controller
4. Read elevator states 
5. Allow for subscribing to changes
*/

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
func (sm *SnapshotManager) UpdatePeerSnapshots(incomingSnapshots map[string]Snapshot){
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
