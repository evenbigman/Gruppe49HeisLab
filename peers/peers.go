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
	ConfirmedOrderChangeCh      chan struct{}
	UnconfirmedOrderChangeCh      chan struct{}
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
	orderMutex     sync.RWMutex
	confirmedOrders         [config.NumFloors][2]bool
	unconfirmedOrders         [config.NumFloors][2]bool
}

var (
	instance *PeerManager
	once     sync.Once
)

func GetPeerManager() *PeerManager {
	myID := GetMyID()
	pm := &PeerManager{
		ConfirmedOrderChangeCh:      make(chan struct{}),
		UnconfirmedOrderChangeCh:      make(chan struct{}),
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

				connectedPeers := 0
				var mutualUnconfirmedOrders [config.NumFloors][2]int
				for id, snapshot := range msg.Snapshots{
					s, err := pm.GetStatus(id)
					if err != nil && id != pm.myID{
						continue
					}
					if s.Connected || id == pm.myID{
						connectedPeers++
						if UnconfirmedOrderExists(snapshot) {
							newOrders := snapshot.Elevator.PressedHallButtons
							if !orderMatrixEqual(newOrders, pm.GetUnconfirmedOrders()) {
								pm.SetUnconfirmedOrders(newOrders)
								pm.UnconfirmedOrderChangeCh <- struct{}{}
							}
						}
						for i := range mutualUnconfirmedOrders{
							for j := range mutualUnconfirmedOrders[i]{
								if snapshot.Elevator.PressedHallButtons[i][j]{
									mutualUnconfirmedOrders[i][j]++	
								} else {
									mutualUnconfirmedOrders[i][j]--
								}
							}
						}
					}
				}

				var confirmedOrders [config.NumFloors][2]bool
				savedConfirmedOrders := pm.GetConfirmedOrders()

				for i := range confirmedOrders{
					for j := range confirmedOrders[i]{
						log.Printf("Connected peers: %d:" ,connectedPeers)
						switch mutualUnconfirmedOrders[i][j] {
						case connectedPeers:
							confirmedOrders[i][j] = true
						case -connectedPeers:
							confirmedOrders[i][j] = false
						default:
							confirmedOrders[i][j] = savedConfirmedOrders[i][j]
						}
					}
				}

				if !orderMatrixEqual(confirmedOrders, savedConfirmedOrders){
					pm.SetConfirmedOrders(confirmedOrders)
					pm.ConfirmedOrderChangeCh <- struct{}{}
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

func (pm *PeerManager) GetConfirmedOrders() [config.NumFloors][2]bool {
	pm.orderMutex.RLock()
	defer pm.orderMutex.RUnlock()

	return pm.confirmedOrders
}

func (pm *PeerManager) SetConfirmedOrders(orders [config.NumFloors][2]bool) {
	pm.orderMutex.Lock()
	defer pm.orderMutex.Unlock()

	pm.confirmedOrders = orders
}

func (pm *PeerManager) GetUnconfirmedOrders() [config.NumFloors][2]bool {
	pm.orderMutex.RLock()
	defer pm.orderMutex.RUnlock()

	return pm.unconfirmedOrders
}

func (pm *PeerManager) SetUnconfirmedOrders(orders [config.NumFloors][2]bool) {
	pm.orderMutex.Lock()
	defer pm.orderMutex.Unlock()

	pm.unconfirmedOrders = orders
}

func (pm *PeerManager) GetStatus(ID uint64) (status.Status, error){
	statuses := pm.statusManager.GetStatuses()
	s, statusFound := statuses[ID]
	if !statusFound {
		err := fmt.Errorf("Status for %d not found", ID)
		return status.Status{}, err
	}
	
	return s, nil
}

func (pm *PeerManager) ImOnline() bool {
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

func UnconfirmedOrderExists(snapshot snapshots.Snapshot) bool{
	confirmedOrders := snapshot.Elevator.ConfirmedHallOrders
	hallButtons := snapshot.Elevator.PressedHallButtons

	if !orderMatrixEqual(hallButtons, confirmedOrders){
		return true
	} else {
		return false
	}
}

func orderMatrixEqual(a, b [config.NumFloors][2]bool) bool {
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
