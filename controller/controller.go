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
	doorOpenTime = 3 * time.Second
	undefined    = -1
	defaultPort  = "15657"
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
	CurrentFloor       int
	NextFloor          int
	HallOrders         [numFloors][2]bool
	CabOrders          [numFloors]bool
	Direction          Direction
	State              ElevatorStatus
	PressedHallButtons [numFloors][2]bool
	PressedCabButtons  [numFloors]bool
	ObstructionPresent bool
}

type ElevatorController struct {
	elevator               Elevator
	lock                   sync.Mutex
	initOnce               sync.Once
	stateChangeSubscribers []chan struct{}
	floorSubscribers       []chan struct{}
	buttonSubscribers      []chan struct{}
	subsLock               sync.Mutex
}

var (
	instance *ElevatorController
	once     sync.Once
)

// Public funcitons
func GetController() *ElevatorController {
	once.Do(func() {
		instance = &ElevatorController{}
	})
	return instance
}

func (ec *ElevatorController) CompleteWaitingOrders() {
	for ec.moreOrders() {

		ec.lock.Lock()
		direction := ec.elevator.Direction
		currentFloor := ec.elevator.CurrentFloor
		cabOrders := ec.elevator.CabOrders
		hallOrders := ec.elevator.HallOrders
		ec.lock.Unlock()

		switch direction {
		case MovingUp:
			for ec.moreOrdersAbove() {
				if cabOrders[currentFloor] || hallOrders[currentFloor][0] || hallOrders[currentFloor][1] {
					ec.stopElevatorAtCurrentFloor()
				}
				ec.elevatorDriveUp()
			}
			ec.stopElevator()

		case MovingDown:
			for ec.moreOrdersBelow() {
				if cabOrders[currentFloor] || hallOrders[currentFloor][0] || hallOrders[currentFloor][1] {
					ec.stopElevatorAtCurrentFloor()
				}
				ec.elevatorDriveDown()
			}
			ec.stopElevator()

		case Stopped:
			if ec.moreOrdersAbove() {
				ec.elevatorDriveUp()
			} else if ec.moreOrdersBelow() {
				ec.elevatorDriveDown()
			}
		}
	}
	ec.lock.Lock()
	ec.elevator.State = Idle
	ec.elevator.Direction = Stopped
	ec.lock.Unlock()
}

func (ec *ElevatorController) InitElevator(port ...string) {
	ec.initOnce.Do(func() {
		p := defaultPort
		if len(port) > 0 {
			p = port[0]
		}
		address := "localhost:" + p
		elevio.Init(address, numFloors)
		//could add functionality to ensure that the elevator knows its current floor if it is between floors when starting.
	})
}

func (ec *ElevatorController) SetHallOrders(confirmedHallOrders [numFloors][2]bool) {
	ec.lock.Lock()
	defer ec.lock.Unlock()

	ec.elevator.HallOrders = confirmedHallOrders
}

func (ec *ElevatorController) SetCabOrders(confirmedCabOrders [numFloors]bool) {
	ec.lock.Lock()
	defer ec.lock.Unlock()
	ec.elevator.CabOrders = confirmedCabOrders

}

func (ec *ElevatorController) GetElevatorState() Elevator {
	//locking to ensure no updates happen while it reads the state of the elevator
	ec.lock.Lock()
	defer ec.lock.Unlock()
	v := ec.elevator
	return v
}

func (ec *ElevatorController) SubscribeState() <-chan struct{} {
	return ec.addSubscriber(&ec.stateChangeSubscribers)
}

func (ec *ElevatorController) SubscribeFloor() <-chan struct{} {
	return ec.addSubscriber(&ec.floorSubscribers)
}

func (ec *ElevatorController) SubscribeButtons() <-chan struct{} {
	return ec.addSubscriber(&ec.buttonSubscribers)
}

// Private funcitons
func (ec *ElevatorController) addSubscriber(subs *[]chan struct{}) <-chan struct{} {
	ch := make(chan struct{}, 1)
	ec.subsLock.Lock()
	*subs = append(*subs, ch)
	ec.subsLock.Unlock()
	return ch
}

func (ec *ElevatorController) notify(subscribers *[]chan struct{}) {
	ec.subsLock.Lock()
	defer ec.subsLock.Unlock()
	for _, ch := range *subscribers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (ec *ElevatorController) notifyFloor() {
	ec.notify(&ec.floorSubscribers)
}

func (ec *ElevatorController) notifyState() {
	ec.notify(&ec.stateChangeSubscribers)
}

func (ec *ElevatorController) notfiyButton() {
	ec.notify(&ec.buttonSubscribers)
}

func (ec *ElevatorController) updateElevatorState() {
	ec.InitElevator()

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
				ec.lock.Lock()
				ec.elevator.PressedHallButtons[v.Floor][1] = true
				ec.lock.Unlock()

			case elevio.BT_HallUp:
				ec.lock.Lock()
				ec.elevator.PressedHallButtons[v.Floor][0] = true
				ec.lock.Unlock()

			case elevio.BT_Cab:
				ec.lock.Lock()
				ec.elevator.PressedCabButtons[v.Floor] = true
				ec.lock.Unlock()

			}

		case v := <-floor:
			ec.lock.Lock()
			ec.elevator.CurrentFloor = v
			ec.lock.Unlock()

		case v := <-obstruction:
			ec.lock.Lock()
			ec.elevator.ObstructionPresent = v
			ec.lock.Unlock()

		}
	}
}

func (ec *ElevatorController) moreOrdersAbove() bool {
	ec.lock.Lock()
	hallOrders := ec.elevator.HallOrders
	cabOrders := ec.elevator.CabOrders
	currentFloor := ec.elevator.CurrentFloor
	floorAbove := ec.elevator.CurrentFloor + 1
	ec.lock.Unlock()

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

func (ec *ElevatorController) moreOrdersBelow() bool {
	ec.lock.Lock()
	hallOrders := ec.elevator.HallOrders
	cabOrders := ec.elevator.CabOrders
	currentFloor := ec.elevator.CurrentFloor
	floorBelow := ec.elevator.CurrentFloor - 1
	ec.lock.Unlock()

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
	for _, orderBelow := range cabOrders[:floorBelow] {
		if orderBelow {
			return true
		}
	}
	return false
}

func (ec *ElevatorController) moreOrders() bool {
	ec.lock.Lock()
	hallOrders := ec.elevator.HallOrders
	cabOrders := ec.elevator.CabOrders
	ec.lock.Unlock()

	for _, orders := range hallOrders {
		for _, orderActive := range orders {
			if orderActive {
				return true
			}
		}
	}

	for _, orderActive := range cabOrders {
		if orderActive {
			return true
		}
	}

	return false
}

func (ec *ElevatorController) setLight(floor int, lightType elevio.ButtonType, lightState bool) {
	//TODO change this functions in terms of this module, with new constants
	elevio.SetButtonLamp(lightType, floor, lightState)
}

func (ec *ElevatorController) openDoor() {
	elevio.SetDoorOpenLamp(true)
}

func (ec *ElevatorController) closeDoor() {
	elevio.SetDoorOpenLamp(false)
}

func (ec *ElevatorController) stopElevatorAtCurrentFloor() {
	ec.stopElevator()
	ec.openDoor()
	time.Sleep(doorOpenTime)
	ec.closeDoor()
}

func (ec *ElevatorController) stopElevator() {
	elevio.SetMotorDirection(elevio.MD_Stop)
	ec.lock.Lock()
	ec.elevator.Direction = Stopped
	ec.lock.Unlock()
}

func (ec *ElevatorController) elevatorDriveUp() {
	elevio.SetMotorDirection(elevio.MD_Up)
	ec.lock.Lock()
	ec.elevator.Direction = MovingUp
	ec.lock.Unlock()
}

func (ec *ElevatorController) elevatorDriveDown() {
	elevio.SetMotorDirection(elevio.MD_Down)
	ec.lock.Lock()
	ec.elevator.Direction = MovingDown
	ec.lock.Unlock()
}

//TODO Add functionality to consider obstructions
//TODO Add functionlity for anouncing direction upon getting to a floor
//TODO Add functionality to consider latest cab button pressed when someone enters an elevator if it should change direction
//TODO Add functionlity to clear orders once at a floor
//TODO Add funcitonality to get configs from a file
