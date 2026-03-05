package controller

import (
	"sanntidslab/elevio"
	"sync"
	"time"
)

const (
	//some constants used for the elevators to avoid magic numbers
	//TODO read from file
	numFloors    = 4
	maxFloor     = numFloors - 1
	doorOpenTime = 3
)

type ElevatorStatus int

const (
	Idle ElevatorStatus = iota
	Driving
	DoorOpen
	Obstructed
)

type Direction int

const (
	Stopped Direction = iota
	MovingUp
	MovingDown
)

type Elevator struct {
	CurrentFloor        int
	NextFloor           int
	HallOrders          [numFloors][2]bool
	CabOrders           [numFloors]bool
	Direction           Direction
	State               ElevatorStatus
	PressedHallButtons  [numFloors][2]bool
	PressedCabButtons   [numFloors]bool
	ObstructionPressent bool
}

// Local elevator
var elevator = Elevator{
	CurrentFloor: -1, //start with undefined states, maybe convert to non-magic-numbers
	NextFloor:    -1,
	Direction:    Stopped,
	State:        Idle,
}
var elevatorLock sync.Mutex
var _initialized bool = false

// Public funcitons
func CompleteWaitingOrders() {
	for moreOrders() {
		switch elevator.Direction {
		case MovingUp:
			for moreOrdersAbove() {
				if elevator.CabOrders[elevator.CurrentFloor] || elevator.HallOrders[elevator.CurrentFloor][0] || elevator.HallOrders[elevator.CurrentFloor][1] {
					stopElevatorAtCurrentFloor()
				}
				elevatorDriveUp()
			}
			stopElevator()

		case MovingDown:
			for moreOrdersBelow() {
				if elevator.CabOrders[elevator.CurrentFloor] || elevator.HallOrders[elevator.CurrentFloor][0] || elevator.HallOrders[elevator.CurrentFloor][1] {
					stopElevatorAtCurrentFloor()
				}
				elevatorDriveDown()
			}
			stopElevator()

		case Stopped:
			if moreOrdersAbove() {
				elevatorDriveUp()
			} else if moreOrdersBelow() {
				elevatorDriveDown()
			}
		}
	}
	elevatorLock.Lock()
	elevator.State = Idle
	elevator.Direction = Stopped
	elevatorLock.Unlock()
}

func InitElevator()
	elevio.Init("localhost:15657", numFloors)
	//could add functionality to ensure that the elevator knows its current floor if it is between floors when starting.
	_initialized = true
}

func SetHallOrders(confirmedHallOrders [numFloors][2]bool)
	elevatorLock.Lock()
	defer elevatorLock.Unlock()

	elevator.HallOrders = confirmedHallOrders
}

func SetCabOrders(confirmedCabOrders [numFloors]bool) {
	elevatorLock.Lock()
	defer elevatorLock.Unlock()
	elevator.CabOrders = confirmedCabOrders

}

func GetElevatorState() Elevator {
	//locking to ensure no updates happen while it reads the state of the elevator
	elevatorLock.Lock()
	defer elevatorLock.Unlock()
	v := elevator
	return v
}

// Private funcitons
func updateElevatorState() {
	if !_initialized {
		InitElevator()
	}

	floor := make(chan int)
	buttons := make(chan elevio.ButtonEvent)
	obstruction := make(chan bool)

	go elevio.PollButtons(buttons)
	go elevio.PollFloorSensor(floor)
	go elevio.PollObstructionSwitch(obstruction)
	for {
		select {
		case v := <-buttons:

			switch v.Button {
			case elevio.BT_HallDown:
				elevatorLock.Lock()
				elevator.PressedHallButtons[v.Floor][1] = true
				elevatorLock.Unlock()

			case elevio.BT_HallUp:
				elevatorLock.Lock()
				elevator.PressedHallButtons[v.Floor][0] = true
				elevatorLock.Unlock()

			case elevio.BT_Cab:
				elevatorLock.Lock()
				elevator.PressedCabButtons[v.Floor] = true
				elevatorLock.Unlock()

			}

		case v := <-floor:
			elevatorLock.Lock()
			elevator.CurrentFloor = v
			elevatorLock.Unlock()

		case v := <-obstruction:
			elevatorLock.Lock()
			elevator.ObstructionPressent = v
			elevatorLock.Unlock()

		}
	}
}

func moreOrdersAbove() bool {
	elevatorLock.Lock()
	hallOrders := elevator.HallOrders
	cabOrders := elevator.CabOrders
	currentFloor := elevator.CurrentFloor
	floorAbove := elevator.CurrentFloor + 1
	elevatorLock.Unlock()

	if currentFloor == maxFloor {
		return false
	}

	for _, orders := range hallOrders[floorAbove:] {
		for _, orderAbove := range orders {
			if orderAbove {
				return true
			}
		}
	}

	for _, orderAbove := range cabOrders[floorAbove:] {
		if orderAbove {
			return true
		}

	}

	return false
}

func moreOrdersBelow() bool {
	elevatorLock.Lock()
	hallOrders := elevator.HallOrders
	cabOrders := elevator.CabOrders
	currentFloor := elevator.CurrentFloor
	floorBelow := elevator.CurrentFloor - 1
	elevatorLock.Unlock()

	if currentFloor == 0 {
		return false
	}

	for _, orders := range hallOrders[:floorBelow] {
		for _, orderBelow := range orders {
			if orderBelow {
				return true
			}
		}
	}
	for _, orderBelow := range cabOrders {
		if orderBelow {
			return true
		}
	}
	return false
}

func moreOrders() bool {
	elevatorLock.Lock()
	hallOrders := elevator.HallOrders
	cabOrders := elevator.CabOrders
	elevatorLock.Unlock()

	orderActive := false

	for _, orders := range hallOrders {
		for _, orderActive = range orders {
			if orderActive {
				return orderActive
			}
		}
	}

	for _, orderActive := range cabOrders {
		if orderActive {
			return orderActive
		}
	}

	return false
}

func setLight(floor int, lightType elevio.ButtonType, lightState bool) {
	//TODO change this functions in terms of this module, with new constants
	elevio.SetButtonLamp(lightType, floor, lightState)
}

func openDoor() {
	elevio.SetDoorOpenLamp(true)
}

func closeDoor() {
	elevio.SetDoorOpenLamp(false)
}

func goToFloor(orderedFloor int) {
	for elevator.CurrentFloor != orderedFloor {
		if elevator.CurrentFloor < orderedFloor {
			elevio.SetMotorDirection(elevio.MD_Up)
		} else if elevator.CurrentFloor > orderedFloor {
			elevio.SetMotorDirection(elevio.MD_Down)
		}
	}
	stopElevator()
}

func stopElevatorAtCurrentFloor() {
	stopElevator()
	openDoor()
	elevator.State = DoorOpen
	time.Sleep(doorOpenTime * time.Second)
	closeDoor()
}

func stopElevator() {
	elevio.SetMotorDirection(elevio.MD_Stop)
	elevatorLock.Lock()
	elevator.Direction = Stopped
	elevatorLock.Unlock()
}

func elevatorDriveUp() {
	elevio.SetMotorDirection(elevio.MD_Up)
	elevatorLock.Lock()
	elevator.Direction = MovingUp
	elevatorLock.Unlock()
}

func elevatorDriveDown() {
	elevio.SetMotorDirection(elevio.MD_Down)
	elevatorLock.Lock()
	elevator.Direction = MovingDown
	elevatorLock.Unlock()
}
