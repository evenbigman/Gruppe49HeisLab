package peers

//TODO: Handle disconnecting and reconnecting
//TODO: Make id type
//TODO: Make snapshot maps and status maps own types
//TODO: Only update states based on connected peers (ignore newly connected)
//TODO: define bcastinterval when initin pm

import (
	"fmt"
	"log"
	"net"
	"sanntidslab/config"
	"sanntidslab/controller"
	"sanntidslab/peers/broadcast"
	"sanntidslab/peers/snapshots"
	"sanntidslab/peers/status"
	"sync"
	"time"
)

type PeerManager struct {
	OrderChangeCh      chan struct{}
	DisconnectedPeerCh chan struct{}
	myID               uint64
	broadcastTx        chan broadcast.Msg
	broadcastRx        chan broadcast.Msg
	snapshotManager    *snapshots.SnapshotManager
	statusManager      *status.StatusManager
	initialized        bool
	lastAckedVersion   int
	ackMutex           sync.RWMutex
	ackNotifyCh        chan struct{}
	hallOrderMutex     sync.RWMutex
	hallOrders         [config.NumFloors][2]bool
}

var (
	instance *PeerManager
	once sync.Once
)

func GetPeerManager() *PeerManager {
	myID := GetMyID()
	pm := &PeerManager{
		OrderChangeCh:      make(chan struct{}),
		DisconnectedPeerCh: make(chan struct{}),
		myID:               myID,
		broadcastTx:        make(chan broadcast.Msg),
		broadcastRx:        make(chan broadcast.Msg),
		snapshotManager:    snapshots.GetSnapshotManager(myID),
		statusManager:      status.GetStatusManager(config.ConnectionTimeThreshold, config.TimeoutInterval, config.BcastInterval),
		initialized:        false,
		lastAckedVersion:   0,
		ackNotifyCh:        make(chan struct{}, 1),
	}

	once.Do(func() {
		instance = pm
	})
	return instance
}

func (pm *PeerManager) Init() {
	pm.DisconnectedPeerCh = pm.statusManager.DisconnectedPeerCh

	go broadcast.Transmitter(config.BcastPort, pm.broadcastTx)
	go broadcast.Receiver(config.BcastPort, pm.broadcastRx)

	pm.initialized = true
}

func (pm *PeerManager) Run() error {
	if !pm.initialized {
		log.Printf("ID: %d", GetMyID())
		return fmt.Errorf("Init() must be called before Run()")
	}
	ticker := time.NewTicker(config.BcastInterval)
	defer ticker.Stop()

	go pm.statusManager.Run()

	for {
		select {
		case msg := <-pm.broadcastRx:
			if msg.Sender != pm.myID {
				pm.statusManager.UpdateStatus(msg.Sender)

				oldSnapshots := pm.snapshotManager.GetSnapshots()
				statuses := pm.statusManager.GetStatuses()

				ackedVersion := pm.snapshotManager.MergeSnapshots(msg.Snapshots)

				pm.ackMutex.Lock()
				if pm.lastAckedVersion < ackedVersion {
					pm.lastAckedVersion = ackedVersion
					pm.ackMutex.Unlock()
					select {
					case <-pm.ackNotifyCh:
					default:
					}
					pm.ackNotifyCh <- struct{}{}
				} else {
					pm.ackMutex.Unlock()
				}

				pm.hallOrderMutex.Lock()
				
				var connectedIds []uint64
				for id, status := range statuses{
					if status.Connected{
						connectedIds = append(connectedIds, id)
					}
				}
				connectedIds = append(connectedIds, pm.myID)

				oldOrders := pm.GetOrders()
				newOrders := pm.snapshotManager.ComputeOrders(oldSnapshots, connectedIds)
				if !hallOrdersEqual(oldOrders, newOrders) {
					pm.hallOrders = newOrders
					pm.hallOrderMutex.Unlock()
					pm.OrderChangeCh <- struct{}{}
				} else {
					pm.hallOrderMutex.Unlock()
				}
			}

		case <-ticker.C:
			snapshots := pm.snapshotManager.GetSnapshots()
			msg := broadcast.Msg{
				Sender:    pm.myID,
				Snapshots: snapshots,
			}
			pm.broadcastTx <- msg
		}
	}
}

func (pm *PeerManager) WaitForStartSnapshot() (controller.Elevator, error) {
	startuptTimer := time.NewTimer(config.InitDelay)

	for {
		select {
		case <-startuptTimer.C:
			snapshot, err := pm.GetMySnapshot()
			return snapshot.Elevator, err
		default:
			snapshot, err := pm.GetMySnapshot()
			if err == nil {
				return snapshot.Elevator, nil
			}
		}
	}
}

func (pm *PeerManager) WaitForAck(elevator controller.Elevator, timeout time.Duration) error {
	//TODO: Make semaphor to limit how many of this function can run at a time
	pm.SetMySnapshot(elevator)

	snapshot, err := pm.GetMySnapshot()
	if err != nil {
		return err
	}

	targetVersion := snapshot.Version

	deadline := time.After(timeout)
	for {
		select {
		case <-pm.ackNotifyCh:
			pm.ackMutex.RLock()
			ackedVersion := pm.lastAckedVersion
			pm.ackMutex.RUnlock()
			if ackedVersion == targetVersion {
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

func (pm *PeerManager) GetConnectedSnapshots() map[uint64]snapshots.Snapshot {
	statuses := pm.statusManager.GetStatuses()
	snaps := pm.snapshotManager.GetSnapshots()

	output := make(map[uint64]snapshots.Snapshot)
	for id, status := range statuses {
		if status.Connected {
			output[id] = snaps[id]
		}
	}
	return output
}

func (pm *PeerManager) SetMySnapshot(elevator controller.Elevator) error {
	oldVersion := 0

	mySnapshot, err := pm.getSnapshot(pm.myID)
	if err == nil {
		oldVersion = mySnapshot.Version
	}

	sm := pm.snapshotManager
	sm.SetSnapshot(pm.myID, oldVersion+1, elevator)

	return nil
}

func (pm *PeerManager) GetOrders() [config.NumFloors][2]bool {
	pm.hallOrderMutex.RLock()
	defer pm.hallOrderMutex.RUnlock()

	return pm.hallOrders
}

func (pm *PeerManager) ImOnline() bool{
	connectedSnapshots := pm.GetConnectedSnapshots()
	if len(connectedSnapshots) != 0 {
		return true
	} else {
		return false
	}
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

func hallOrdersEqual(a, b [config.NumFloors][2]bool) bool{
	for i := range a {
		for j := range a[i] {
			if a[i][j] != b[i][j] {
				return false
			}
		}	
	}
	return true
}

func GetMyID() uint64 { //Get mac address
	interfaces, err := net.Interfaces()
	if err != nil {
		panic(err)
	}
	for _, i := range interfaces { //Fore each interface
		if i.Flags&net.FlagUp != 0 && len(i.HardwareAddr) != 0 {
			var result uint64
			for _, b := range i.HardwareAddr {
				result = (result << 8) | uint64(b)
			}
			return result
		}
	}
	return 0
}
