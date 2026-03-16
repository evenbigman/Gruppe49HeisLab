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
	"sanntidslab/controller"
	"log"
	"net"
	"time"
	"fmt"
)

type PeerManager struct{
	NewOrderCh chan struct{}
	DisconnectedPeerCh chan struct{}
	myID uint64
	broadcastTx chan broadcast.Msg
	broadcastRx chan broadcast.Msg
	snapshotManager *snapshots.SnapshotManager
	statusManager *status.StatusManager
	initialized bool
} 

func NewPeerManager() *PeerManager{
	myID := getMyID()
	pm := &PeerManager{
		NewOrderCh: make(chan struct{}), 
		DisconnectedPeerCh: make(chan struct{}),
		myID: myID,
		broadcastTx: make(chan broadcast.Msg),
		broadcastRx: make(chan broadcast.Msg),
		snapshotManager: snapshots.NewSnapshotManager(myID),
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
		log.Printf("ID: %d", getMyID())
		return fmt.Errorf("Init() must be called before Run()")
	}
	ticker := time.NewTicker(config.BcastInterval * time.Millisecond)
	defer ticker.Stop()

	go pm.statusManager.Run()

	for{
		select{
		case msg := <-pm.broadcastRx:
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

func (pm *PeerManager) GetMySnapshot() (controller.Elevator, error) {
	sm := pm.snapshotManager
	snapshot, err := sm.GetSnapshot(pm.myID)
	if err != nil{
		return controller.Elevator{}, err
	}
	
	return snapshot.Elevator, nil
}

func getMyID() uint64{ //Get mac address
	interfaces, err := net.Interfaces()
	if err != nil {
		panic(err)
	}
	for _, i := range interfaces{ //Fore each interface
		if i.Flags&net.FlagUp != 0 && len(i.HardwareAddr) != 0	{
			var result uint64
			for _, b := range i.HardwareAddr {
				result = (result << 8) | uint64(b)
			}
			return result
		}
	}
	return 0
}
