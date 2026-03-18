package status
//TODO: Add proper error handling
//TODO: Add logging
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
	DisconnectedPeerCh chan struct{}
	peers map[uint64]status
	timeout time.Duration
	interval time.Duration
	mutex sync.RWMutex
}

func NewStatusManager(timeout time.Duration, interval time.Duration) *StatusManager{
	sm := &StatusManager{
		DisconnectedPeerCh: make(chan struct{}),
		timeout:  timeout,
		interval: interval,
		peers: make(map[uint64]status),
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

func (sm *StatusManager) UpdateStatus(peerID uint64){
	sm.mutex.Lock()	
	defer sm.mutex.Unlock()
	
	peer, peerIsStored := sm.peers[peerID]
	if !peerIsStored{
		peer = status{
			LastSeen: time.Now(),
			Connected: true,
		}
	} else {
		peer.LastSeen = time.Now()
		peer.Connected = true
	}
}

func (sm *StatusManager) GetStatuses() map[uint64]status{
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()
	
	output := make(map[uint64]status, len(sm.peers))
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
			sm.DisconnectedPeerCh <- struct{}{}
		}	
	}
}
