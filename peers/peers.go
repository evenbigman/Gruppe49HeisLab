package peers

import(
	"sanntidslab/controller"
	"time"
	"log"
	"sync"
)

type PeerInfo struct{
	LastSeen time.Time
	Connected bool
}

type ElevatorState struct{
	Version int
	Elevator controller.Elevator
}


/*PeerMonitor
PeerMonitor struct: maop[struct]peerInfo, timeout
Functions:
1. Init
2. UpdatePeer
*/
type PeerMonitor struct{
	peers map[string]PeerInfo
	timeout time.Duration
	interval time.Duration
	mutex sync.Mutex
}

func NewPeerMonitor(timeout time.Duration, interval time.Duration) *PeerMonitor{
	pm := &PeerMonitor{
		timeout:  timeout,
		interval: interval,
		peers: make(map[string]PeerInfo),
	}
	return pm
}

func (pm *PeerMonitor) Run(){
	ticker := time.NewTicker(pm.interval)
	defer ticker.Stop()

	for range ticker.C {
		pm.update()
	}
}

func (pm *PeerMonitor) UpdateHeartbeat(peerID string){
	pm.mutex.Lock()	
	defer pm.mutex.Unlock()
	
	_, peerIsStored := pm.peers[peerID]
	if !peerIsStored{
		pm.peers[peerID] = PeerInfo{
			LastSeen: time.Now(),
			Connected: true,
		}
	}
}

func (pm *PeerMonitor) update(){
	pm.mutex.Lock()	
	defer pm.mutex.Unlock()

	for _, peer := range pm.peers{
		if time.Since(peer.LastSeen) >= pm.timeout {
			peer.Connected = false
		}	
	}
}

/* STATE MANAGER
Type contains: map of all elevator states, mutex for it, its own ID
Functions:
1. Init
2. Merge changes received from internet
3. Merge own changes received from controller
4. Read elevator states 
*/
type StateManager struct{
	mutex sync.RWMutex
	elevators map[string]ElevatorState
	myID string
}

func NewStateManager(myID string, initialLocalElevator controller.Elevator) *StateManager{
	sm := &StateManager{
		myID: myID,
		elevators: make(map[string]ElevatorState),
	}	
	
	sm.elevators[myID] = ElevatorState{
		Version: 0,
		Elevator: initialLocalElevator,
	}

	return sm
}

//Takes incoming state, updates if necessary
func (sm *StateManager) UpdateGlobalState(incomingElevatorStates map[string]ElevatorState){
	sm.mutex.Lock()
	defer sm.mutex.Unlock()
	
	for rcvdID, rcvdElevatorState := range incomingElevatorStates{
		if rcvdID == sm.myID{
			continue
		}
		
		storedElevatorState, elevatorIsStored := sm.elevators[rcvdID]
		if !elevatorIsStored ||
		rcvdElevatorState.Version > storedElevatorState.Version {
			sm.elevators[rcvdID] = rcvdElevatorState
		}
	}
}

func (sm *StateManager) UpdateLocalState(localElevator controller.Elevator) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	previousState := sm.elevators[sm.myID]
	sm.elevators[sm.myID] = ElevatorState{
		Version: previousState.Version + 1,
		Elevator: localElevator,
	}
}

func (sm *StateManager) Snapshot() map[string]ElevatorState{
	sm.mutex.Lock()
	defer sm.mutex.Unlock()
	
	outputGlobalState := make(map[string]ElevatorState, len(sm.elevators))
	for id, elevatorState := range sm.elevators {
		outputGlobalState[id] = elevatorState 
	}
	return outputGlobalState
}
