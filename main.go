package main

import (
	backup "sanntidslab/backup_handler"
	"sanntidslab/controller"
	"sanntidslab/peers"
)

func main() {
	backup.Init()

	ElevatorController := controller.GetController()
	ElevatorController.InitElevator()
	ElevatorController.Start()
	elevator := ElevatorController.GetElevatorState()

	peerManager := peers.NewPeerManager()
	peerManager.Init()
	peerManager.Run()

	for {
		select {
		case order := <-peerManager.NewOrderCh:
			// Something
		case disconnectedPeers := <-peerManager.DisconnectedPeerCh:
			// Something
		}
	}
}
