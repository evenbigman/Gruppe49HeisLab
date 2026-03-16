package peers
//TODO: Add logging
//TODO: Handle disconnecting and reconnecting
//TODO: Make singelton
//TODO: Add ACKING
//TODO: Make id type

import(
	"sanntidslab/peers/status"
	"sanntidslab/peers/snapshots"
	"sanntidslab/peers/broadcast"
	"sanntidslab/config"
	"sanntidslab/controller"
	"sync"
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
	lastAckedVersion int
	ackMutex sync.Mutex
	ackNotifyCh chan struct{}
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
		lastAckedVersion: 0,
		ackNotifyCh: make(chan struct{}, 1),
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
			if msg.Sender != pm.myID{
				pm.statusManager.UpdateStatus(msg.Sender)
				newOrderFound, ackedVersion := pm.snapshotManager.MergeSnapshots(msg.Snapshots)

				pm.lastAckedVersion = ackedVersion
				pm.ackNotifyCh <- struct{}{}

				if newOrderFound{
					pm.NewOrderCh <- struct{}{}			
				}
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

func (pm *PeerManager) WaitForAck(elevator controller.Elevator, timeout time.Duration) error{
	//TODO: Make semaphor to limit how many of this function can run at a time
	pm.SetMySnapshot(elevator)

	snapshot, err := pm.GetMySnapshot()
	if err != nil{
		return err
	}

	targetVersion := snapshot.Version
	
	deadline := time.After(timeout)
	for{
		select{
		case <-pm.ackNotifyCh:
			pm.ackMutex.Lock()
			ackedVersion := pm.lastAckedVersion
			pm.ackMutex.Unlock()
			if ackedVersion >= targetVersion{
				return nil
			}
		case <-deadline:
			err := fmt.Errorf("Ack timed out")
			return err
		}
	}
} 

func (pm *PeerManager) GetMySnapshot() (snapshots.Snapshot, error) {
	snapshot, err := pm.getSnapshot(pm.myID)
	return snapshot, err
}

func (pm *PeerManager) GetConnectedSnapshots() []snapshots.Snapshot {
	statuses := pm.statusManager.GetStatuses()
	snaps := pm.snapshotManager.GetSnapshots()

	output := make([]snapshots.Snapshot, 0, len(statuses))
	for id, status := range statuses{
		if status.Connected{
			output = append(output, snaps[id])
		}
	}
	return output
}

func (pm *PeerManager) SetMySnapshot(elevator controller.Elevator) error{
	mySnapshot, err := pm.getSnapshot(pm.myID)
	if err != nil{
		return err
	}

	oldVersion := mySnapshot.Version

	sm := pm.snapshotManager
	sm.SetSnapshot(pm.myID, oldVersion + 1, elevator) 

	return nil
}

func (pm *PeerManager) getSnapshot(ID uint64) (snapshots.Snapshot, error) {
	sm := pm.snapshotManager
	snaps := sm.GetSnapshots()

	snapshot, ok := snaps[ID]
	if !ok {
		err := fmt.Errorf("Snapshot for ID: %d not found", ID)
		return snapshots.Snapshot{}, err
	}
	
	return snapshot, nil
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
