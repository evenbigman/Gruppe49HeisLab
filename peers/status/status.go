package status
//TODO: Add proper error handling
//TODO: Add logging
//TODO: Make singelton

import(
	"time"
	"sync"
)

type status struct{
	FirstSeen time.Time
	LastSeen time.Time
	Connected bool
}

type StatusManager struct{
	DisconnectedPeerCh chan struct{}
	peers map[uint64]status
	connectionTimeThreshold time.Duration
	timeout time.Duration
	interval time.Duration
	mutex sync.RWMutex
}

func NewStatusManager(connectionTimeThreshold time.Duration, timeout time.Duration, interval time.Duration) *StatusManager{
	sm := &StatusManager{
		DisconnectedPeerCh: make(chan struct{}),
		connectionTimeThreshold: connectionTimeThreshold,
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
			FirstSeen: time.Now(),
			LastSeen: time.Now(),
			Connected: false,
		}
		sm.peers[peerID] = peer
	} else {
		peer.LastSeen = time.Now()
		sm.peers[peerID] = peer
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
		if !peer.Connected && time.Since(peer.FirstSeen) >= sm.connectionTimeThreshold{
			peer.Connected = true
			sm.peers[id] = peer
		}
		if peer.Connected && time.Since(peer.LastSeen) >= sm.timeout {
			delete(sm.peers, id)
			sm.DisconnectedPeerCh <- struct{}{}
		}	
	}
}
