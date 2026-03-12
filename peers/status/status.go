package status
//TODO: Add proper error handling
//TODO: Add subscriber model to get status changes of peers on a channel
//TODO: Make singelton

import(
	"time"
	"sync"
)

type status struct{
	LastSeen time.Time
	Connected bool
}

type StatusManager struct{
	peers map[string]status
	timeout time.Duration
	interval time.Duration
	mutex sync.RWMutex
}

func NewStatusManager(timeout time.Duration, interval time.Duration) *StatusManager{
	sm := &StatusManager{
		timeout:  timeout,
		interval: interval,
		peers: make(map[string]status),
	}
	return sm
}

func (sm *StatusManager) Run(){
	ticker := time.NewTicker(sm.interval)
	defer ticker.Stop()

	for range ticker.C {
		sm.update()
	}
}

func (sm *StatusManager) UpdateStatus(peerID string){
	sm.mutex.Lock()	
	defer sm.mutex.Unlock()
	
	_, peerIsStored := sm.peers[peerID]
	if !peerIsStored{
		sm.peers[peerID] = status{
			LastSeen: time.Now(),
			Connected: true,
		}
	}
}

func (sm *StatusManager) GetStatus() map[string]status{
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()
	
	output := make(map[string]status, len(sm.peers))
	for id, storedStatus := range sm.peers {
		output[id] = storedStatus 
	}
	return output
}

func (sm *StatusManager) update(){
	sm.mutex.Lock()	
	defer sm.mutex.Unlock()

	for id, peer := range sm.peers{
		if time.Since(peer.LastSeen) >= sm.timeout {
			peer.Connected = false
			sm.peers[id] = peer
		}	
	}
}
