package snapshots
//TODO: Add logging
//TODO: Add proper error handling
//TODO: Add subscriber model to get snapshot changes of peers on a channel
//TODO: MAke singellton
//TODO: Prevent integer overflow on version number
//TODO: Refactor MergeSnapshots, fix abstraction layer together with peers
//TODO: GetORDERS which exist wwhile you havent joined

import(
	"sanntidslab/controller"
	"sanntidslab/config"
	"sync"
)

type Snapshot struct{
	Version int
	Elevator controller.Elevator
}
type SnapshotManager struct{
	mutex sync.RWMutex
	snapshots map[uint64]Snapshot
	myID uint64
}

func NewSnapshotManager(myID uint64) *SnapshotManager{
	sm := &SnapshotManager{
		myID: myID,
		snapshots: make(map[uint64]Snapshot),

	}	

	return sm
}

//Takes incoming state, updates if necessary. Also checks if new order has come :O And returnsed lowest version they have of our state
func (sm *SnapshotManager) MergeSnapshots(incomingSnapshots map[uint64]Snapshot) (ackedVersion int){
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	ackedVersion = 0
	
	for rcvdID, rcvdSnapshot := range incomingSnapshots{
		if rcvdID == sm.myID{
			_, mySnapshotExists := sm.snapshots[sm.myID]
			if mySnapshotExists{
				ackedVersion = rcvdSnapshot.Version
			} else{
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
		Version: version,
		Elevator: elevator,
	}
}

func (sm *SnapshotManager) GetSnapshots() map[uint64]Snapshot{
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
          for i, floor := range snapshot.Elevator.ConfirmedHallOrders {                                                                                            
              for j, order := range floor {                                                                                                                   
                  if order {                                                                                                                                  
                      orders[i][j] = true                                                                                                                     
                  }                                                                                                                                           
              }   
          }
      }
      return orders
  }
