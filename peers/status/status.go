package status

import(
	"sanntidslab/controller"
	"time"
	"log"
	"sync"
)

/*statusManager
statusManager struct: maop[struct]peerInfo, timeout
Functions:
1. Init
2. UpdateHeartbeat of peer
3. Get Status
*/

type status struct{
	LastSeen time.Time
	Connected bool
}

type statusManager struct{
	peers map[string]status
	timeout time.Duration
	interval time.Duration
	mutex sync.Mutex
}

func NewStatusManager(timeout time.Duration, interval time.Duration) *statusManager{
	pm := &statusManager{
		timeout:  timeout,
		interval: interval,
		peers: make(map[string]status),
	}
	return pm
}

func (pm *statusManager) Run(){
	ticker := time.NewTicker(pm.interval)
	defer ticker.Stop()

	for range ticker.C {
		pm.update()
	}
}

func (pm *statusManager) UpdateHeartbeat(peerID string){
	pm.mutex.Lock()	
	defer pm.mutex.Unlock()
	
	_, peerIsStored := pm.peers[peerID]
	if !peerIsStored{
		pm.peers[peerID] = status{
			LastSeen: time.Now(),
			Connected: true,
		}
	}
}

func (pm *statusManager) update(){
	pm.mutex.Lock()	
	defer pm.mutex.Unlock()

	for id, peer := range pm.peers{
		if time.Since(peer.LastSeen) >= pm.timeout {
			peer.Connected = false
			pm.peers[id] = peer
		}	
	}
}
