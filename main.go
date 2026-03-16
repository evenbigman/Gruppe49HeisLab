package main

import (
	backup "sanntidslab/backup_handler"
	"sanntidslab/config"
	"sanntidslab/controller"
	hallrequestassigner "sanntidslab/hall_request_assigner"
	"sanntidslab/peers"
	"time"
)

func main() {
	backup.Init()

	peerManager := peers.NewPeerManager()
	peerManager.Init()
	peerManager.Run()

	pm := peers.NewPeerManager()

	pm.Init()
	go pm.Run()

	time.Sleep(config.InitDelay)
	ec := controller.GetController()
	startSnapshot, err := pm.GetMySnapshot()

	if err == nil {
		ec.InitElevatorWithStates(startSnapshot.Elevator)
	} else {
		ec.InitElevator()
	}

	buttonCh := ec.SubscribeButtons()
	stateCh := ec.SubscribeState()

	ec.Start()

	for {
		select {
		case <-peerManager.NewOrderCh:
			// assign orders from others
		case <-peerManager.DisconnectedPeerCh:
			// Redistribute orders
		case <-buttonCh:
			stateToAck := ec.GetElevatorState()
			go func() {
				err := pm.WaitForAck(stateToAck, config.TimeoutAck)
				if err != nil {
				} else {
					ec.SetGlobalHallOrders(stateToAck.PressedHallButtons)
					// TODO: Add hallrequest assigner
					ec.SetCabOrders(stateToAck.PressedCabButtons)
				}
			}()
		case <-stateCh:
			state := ec.GetElevatorState()
			pm.SetMySnapshot(state)
		}
	}

}
