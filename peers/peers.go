package peers
//TODO: Add logging
//TODO: Handle disconnecting and reconnecting
//TODO: Make singelton
//TODO: Add ACKING

import(
	"sanntidslab/peers/status"
	"sanntidslab/peers/snapshots"
	"sanntidslab/peers/broadcast"
	"sanntidslab/config"
	"time"
	"fmt"
)

type PeerManager struct{
	NewOrderCh chan struct{}
	DisconnectedPeerCh chan struct{}
	myID string
	broadcastTx chan broadcast.Msg
	broadcastRx chan broadcast.Msg
	snapshotManager *snapshots.SnapshotManager
	statusManager *status.StatusManager
	initialized bool
} 

func NewPeerManager(myID string) *PeerManager{
	pm := &PeerManager{
		NewOrderCh: make(chan struct{}), 
		DisconnectedPeerCh: make(chan struct{}),
		myID: myID,
		broadcastTx: make(chan broadcast.Msg),
		broadcastRx: make(chan broadcast.Msg),
		snapshotManager: snapshots.NewSnapshotManager(getMyID()),
		statusManager: status.NewStatusManager(config.BcastInterval, config.TimeoutInterval),
		initialized: false,
	}
	return pm
}


func (pm *PeerManager) Init(){
	pm.DisconnectedPeerCh = pm.statusManager.DisconnectedPeerCh

	go broadcast.Transmitter(config.BcastPort, pm.broadcastTx)
	go broadcast.Receiver(config.BcastPort, pm.broadcastRx)

	pm.initialized = true
}

func (pm *PeerManager) Run() error{
	if !pm.initialized {
		return fmt.Errorf("Init() must be called before Run()")
	}
	ticker := time.NewTicker(config.BcastInterval * time.Millisecond)
	defer ticker.Stop()

	go pm.statusManager.Run()

	for{
		select{
		case msg := <-pm.broadcastTx:
			pm.statusManager.UpdateStatus(msg.Sender)
			newOrderFound := pm.snapshotManager.MergeSnapshots(msg.Snapshots)
			if newOrderFound{
				pm.NewOrderCh <- struct{}{}			
			}
			
		case <-ticker.C:
			snapshots := pm.snapshotManager.GetSnapshots()
			msg := broadcast.Msg{
				Sender: pm.myID,
				Snapshots: snapshots,
			}
			pm.broadcastTx <- msg
		}
	}
}


func getMyID() string{
	return "hei"
}
