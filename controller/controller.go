package controller

import (
	"sanntidslab/elevio"
	"sync"
	"time"
)

const (
	//some constants used for the elevators to avoid magic numbers
	// TODO: read from file
	numFloors    = 4
	maxFloor     = numFloors - 1
	doorOpenTime = 3 * time.Second
	undefined    = -1
	defaultPort  = "15657"
)

type directions int

const (
	up directions = iota
	down
)

type ElevatorState int

const (
	Idle ElevatorState = iota
	MovingUp
	MovingDown
	DoorOpenHeadingUp
	DoorOpenHeadingDown
	Obstructed
)

type Elevator struct {
	CurrentFloor       int
	HallOrders         [numFloors][2]bool
	CabOrders          [numFloors]bool
	State              ElevatorState
	PressedHallButtons [numFloors][2]bool
	PressedCabButtons  [numFloors]bool
}

type ElevatorController struct {
	elevator               Elevator
	stateLock              sync.Mutex
	initOnce               sync.Once
	stateChangeSubscribers []chan struct{}
	floorSubscribers       []chan struct{}
	buttonSubscribers      []chan struct{}
	hallOrderSubscriber    []chan struct{}
	cabOrderSubscriber     []chan struct{}
	obstructionSubscriber  chan struct{}
	subsLock               sync.Mutex
}

var (
	instance     *ElevatorController
	instanceOnce sync.Once
)

// Public funcitons
func GetController() *ElevatorController {
	instanceOnce.Do(func() {
		instance = &ElevatorController{}
	})
	return instance
}

func (ec *ElevatorController) Start() {
	go ec.pollElevatorState()
	go ec.completeWaitingOrders()
	go ec.handleLights()
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
	ec.stateLock.Lock()
	defer ec.stateLock.Unlock()
	ec.elevator.HallOrders = confirmedHallOrders
	ec.notfiyHallOrders()
}

func (ec *ElevatorController) SetCabOrders(confirmedCabOrders [numFloors]bool) {
	ec.stateLock.Lock()
	defer ec.stateLock.Unlock()
	ec.elevator.CabOrders = confirmedCabOrders
	ec.notfiyCabOrders()
}

func (ec *ElevatorController) GetElevatorState() Elevator {
	//locking to ensure no updates happen while it reads the state of the elevator
	ec.stateLock.Lock()
	defer ec.stateLock.Unlock()
	return ec.elevator
}

func (ec *ElevatorController) SubscribeState() <-chan struct{} { // NOTE: might be better with a subscirbe elevator as name, as it is inteded to be used when the elevator in general gets an update
	return ec.addSubscriber(&ec.stateChangeSubscribers)
}

func (ec *ElevatorController) SubscribeFloor() <-chan struct{} {
	return ec.addSubscriber(&ec.floorSubscribers)
}

func (ec *ElevatorController) SubscribeButtons() <-chan struct{} {
	return ec.addSubscriber(&ec.buttonSubscribers)
}

func (ec *ElevatorController) subscribeCabOrders() <-chan struct{} {
	return ec.addSubscriber(&ec.cabOrderSubscriber)
}

func (ec *ElevatorController) subscribeHallOrders() <-chan struct{} {
	return ec.addSubscriber(&ec.hallOrderSubscriber)
}

// Private funcitons
func (ec *ElevatorController) addSubscriber(subs *[]chan struct{}) <-chan struct{} {
	ch := make(chan struct{}, 1)
	ec.subsLock.Lock()
	*subs = append(*subs, ch)
	ec.subsLock.Unlock()
	return ch
}

func (ec *ElevatorController) notifyFloor() {
	ec.notify(&ec.floorSubscribers)
}

func (ec *ElevatorController) notifyState() { // NOTE: might be better with a notify elevator as name, as it is inteded to be used when the elevator in general gets an update
	ec.notify(&ec.stateChangeSubscribers)
}

func (ec *ElevatorController) notfiyButton() {
	ec.notify(&ec.buttonSubscribers)
}

func (ec *ElevatorController) notfiyHallOrders() {
	ec.notify(&ec.hallOrderSubscriber)
}

func (ec *ElevatorController) notfiyCabOrders() {
	ec.notify(&ec.cabOrderSubscriber)
}

func (ec *ElevatorController) notifyObstruciton() {
	ec.notify(&[]chan struct{}{ec.obstructionSubscriber})
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

func (ec *ElevatorController) pollElevatorState() {

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
				ec.stateLock.Lock()
				ec.elevator.PressedHallButtons[v.Floor][1] = true
				ec.stateLock.Unlock()

			case elevio.BT_HallUp:
				ec.stateLock.Lock()
				ec.elevator.PressedHallButtons[v.Floor][0] = true
				ec.stateLock.Unlock()

			case elevio.BT_Cab:
				ec.stateLock.Lock()
				ec.elevator.PressedCabButtons[v.Floor] = true
				ec.stateLock.Unlock()

			}
			ec.notfiyButton()
			ec.notifyState()

		case v := <-floor:
			ec.stateLock.Lock()
			ec.elevator.CurrentFloor = v
			ec.stateLock.Unlock()
			ec.notifyFloor()
			ec.notifyState()

		case <-obstruction:
			ec.setState(Obstructed)
			ec.notifyObstruciton()
			ec.notifyState()

		}
	}
}

func (ec *ElevatorController) setState(state ElevatorState) {
	ec.stateLock.Lock()
	defer ec.stateLock.Unlock()
	ec.elevator.State = state
	ec.notifyState()
}

func (ec *ElevatorController) completeWaitingOrders() {
	floorCh := ec.SubscribeFloor()
	cabOrderCh := ec.subscribeCabOrders()
	hallOrderCh := ec.subscribeHallOrders()

	for {
		select {
		case <-floorCh:
			state := ec.GetElevatorState()

			switch state.State {
			case MovingUp:
				ec.handleArrivalAtFloorGoingUp()
			case MovingDown:
				ec.handleArrivalAtFloorGoingDown()
			}

		case <-cabOrderCh:
			ec.handleNewCabOrder()
		case <-hallOrderCh:
			ec.handleNewHallOrder()
		}
	}
}

func (ec *ElevatorController) moreOrdersAbove() bool {
	state := ec.GetElevatorState()
	floorAbove := state.CurrentFloor + 1

	if state.CurrentFloor == maxFloor {
		return false
	}

	for _, orders := range state.HallOrders[floorAbove:] {
		for _, orderAbove := range orders {
			if orderAbove {
				return true
			}
		}
	}

	for _, orderAbove := range state.CabOrders[floorAbove:] {
		if orderAbove {
			return true
		}

	}

	return false
}

func (ec *ElevatorController) moreOrdersBelow() bool {
	state := ec.GetElevatorState()
	floorBelow := state.CurrentFloor - 1

	if state.CurrentFloor == 0 {
		return false
	}

	for _, orders := range state.HallOrders[:floorBelow] {
		for _, orderBelow := range orders {
			if orderBelow {
				return true
			}
		}
	}
	for _, orderBelow := range state.CabOrders[:floorBelow] {
		if orderBelow {
			return true
		}
	}
	return false
}

func (ec *ElevatorController) moreOrders() bool {
	state := ec.GetElevatorState()

	for _, orders := range state.HallOrders {
		for _, orderActive := range orders {
			if orderActive {
				return true
			}
		}
	}

	for _, orderActive := range state.CabOrders {
		if orderActive {
			return true
		}
	}

	return false
}

func (ec *ElevatorController) handleArrivalAtFloorGoingUp() {
	state := ec.GetElevatorState()

	ec.clearCabOrder(state.CurrentFloor)

	if state.CurrentFloor == maxFloor || !ec.moreOrdersAbove() {
		if ec.moreOrdersBelow() {
			ec.clearHallorder(state.CurrentFloor, down)
			ec.setState(MovingDown)
			ec.stopElevatorAtCurrentFloor()
			ec.elevatorDriveDown()
		} else {
			ec.stopElevator()
			ec.setState(Idle)
		}
		return
	}

	if ec.moreOrdersAbove() {
		ec.clearHallorder(state.CurrentFloor, up)
		ec.stopElevatorAtCurrentFloor()
	}

}

func (ec *ElevatorController) handleArrivalAtFloorGoingDown() {
	state := ec.GetElevatorState()
	ec.clearCabOrder(state.CurrentFloor)

	if state.CurrentFloor == 0 || !ec.moreOrdersBelow() {
		if ec.moreOrdersAbove() {
			ec.clearHallorder(state.CurrentFloor, up)
			ec.setState(MovingUp)
			ec.stopElevatorAtCurrentFloor()
			ec.elevatorDriveUp()
		} else {
			ec.stopElevator()
			ec.setState(Idle)
		}
		return
	}

	if ec.moreOrdersBelow() {
		ec.clearHallorder(state.CurrentFloor, down)
		ec.stopElevatorAtCurrentFloor()
	}

}

func (ec *ElevatorController) handleNewCabOrder() {
	// TODO:
}

func (ec *ElevatorController) handleNewHallOrder() {
	// TODO: Add condition for the elevator being in the floor that is requested
	state := ec.GetElevatorState()

	if state.State == Idle {
		if ec.moreOrdersAbove() {
			ec.setState(MovingUp)
			ec.elevatorDriveUp()
		} else if ec.moreOrdersBelow() {
			ec.setState(MovingDown)
			ec.elevatorDriveDown()
		}
	}
}

func (ec *ElevatorController) openDoor() {
	elevio.SetDoorOpenLamp(true)
}

func (ec *ElevatorController) closeDoor() {
	state := ec.GetElevatorState()
	if state.State == Obstructed {
		for range ec.obstructionSubscriber {
		}
	}
	elevio.SetDoorOpenLamp(false)
} // FIX: Cant handle repeated obstruction

func (ec *ElevatorController) stopElevatorAtCurrentFloor() {
	ec.stopElevator()
	ec.openDoor()
	state := ec.GetElevatorState()

	switch state.State {
	case MovingUp:
		ec.setState(DoorOpenHeadingUp)
	case MovingDown:
		ec.setState(DoorOpenHeadingDown)
	}

	time.Sleep(doorOpenTime)
	ec.closeDoor()

	switch state.State {
	case DoorOpenHeadingUp:
		ec.setState(MovingUp)
	case DoorOpenHeadingDown:
		ec.setState(MovingDown)
	}

}

func (ec *ElevatorController) stopElevator() {
	elevio.SetMotorDirection(elevio.MD_Stop)
}

func (ec *ElevatorController) elevatorDriveUp() {
	elevio.SetMotorDirection(elevio.MD_Up)
}

func (ec *ElevatorController) elevatorDriveDown() {
	elevio.SetMotorDirection(elevio.MD_Down)
}

func (ec *ElevatorController) handleLights() {
	cabOrderCh := ec.subscribeCabOrders()
	hallOrderCh := ec.subscribeHallOrders()

	for {
		select {
		case <-cabOrderCh:
			state := ec.GetElevatorState()
			for floor, order := range state.CabOrders {
				elevio.SetButtonLamp(elevio.BT_Cab, floor, order)
			}

		case <-hallOrderCh:
			state := ec.GetElevatorState()
			for floor, orders := range state.HallOrders {
				// WARNING: might not be according to implementation of other parts of the system
				// as in it can turn on light for going down when it is actually supposed to go up
				elevio.SetButtonLamp(elevio.BT_HallUp, floor, orders[up])
				elevio.SetButtonLamp(elevio.BT_HallDown, floor, orders[down])
			}
		}
	}

}

func (ec *ElevatorController) clearCabOrder(floor int) {
	ec.stateLock.Lock()
	defer ec.stateLock.Unlock()
	ec.elevator.CabOrders[floor] = false
	ec.notfiyCabOrders()
	ec.notifyState()
}

func (ec *ElevatorController) clearHallorder(floor int, direction directions) {
	ec.stateLock.Lock()
	defer ec.stateLock.Unlock()
	ec.elevator.HallOrders[floor][direction] = false
	ec.notfiyHallOrders()
	ec.notifyState()
}

// TODO: Add functionality to consider latest cab button pressed when someone enters an elevator if it should change direction
// TODO: Add functionality to get configs from a file
