package controller

import (
	"sanntidslab/elevio"
	"sync"
	"time"
)

const (
	//some constants used for the elevators to avoid magic numbers
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
	DoorOpenIdle
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
	obstructionSubscriber  []chan struct{}
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

func (ec *ElevatorController) SubscribeCabOrders() <-chan struct{} {
	return ec.addSubscriber(&ec.cabOrderSubscriber)
}

func (ec *ElevatorController) SubscribeHallOrders() <-chan struct{} {
	return ec.addSubscriber(&ec.hallOrderSubscriber)
}

func (ec *ElevatorController) SubscribeObstruction() <-chan struct{} {
	return ec.addSubscriber(&ec.obstructionSubscriber)
}

func (ec *ElevatorController) UnsubscribeObstruction(subber <-chan struct{}) {
	ec.removeSubscriber(&ec.obstructionSubscriber, subber)
}

// Private funcitons
func (ec *ElevatorController) addSubscriber(subs *[]chan struct{}) <-chan struct{} {
	ch := make(chan struct{}, 1)
	ec.subsLock.Lock()
	*subs = append(*subs, ch)
	ec.subsLock.Unlock()
	return ch
}

func (ec *ElevatorController) removeSubscriber(subsList *[]chan struct{}, subscriber <-chan struct{}) {
	ec.subsLock.Lock()
	defer ec.subsLock.Unlock()
	for i, subber := range *subsList {
		if subber == subscriber {
			*subsList = append((*subsList)[:i], (*subsList)[i+1:]...)
		}
	}
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
	ec.notify(&ec.obstructionSubscriber)
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
	cabOrderCh := ec.SubscribeCabOrders()
	hallOrderCh := ec.SubscribeHallOrders()

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

	if state.CurrentFloor == maxFloor {
		return false
	}

	if ec.moreHallOrdersAbove() {
		return true
	}

	if ec.moreCabOrdersAbove() {
		return true
	}

	return false
}

func (ec *ElevatorController) moreHallOrdersAbove() bool {
	state := ec.GetElevatorState()
	floorAbove := state.CurrentFloor + 1

	for _, orders := range state.HallOrders[floorAbove:] {
		for _, orderAbove := range orders {
			if orderAbove {
				return true
			}
		}
	}

	return false
}

func (ec *ElevatorController) moreCabOrdersAbove() bool {
	state := ec.GetElevatorState()
	floorAbove := state.CurrentFloor + 1

	if state.CurrentFloor == maxFloor {
		return false
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

	if state.CurrentFloor == 0 {
		return false
	}

	if ec.moreCabOrdersBelow() {
		return true
	}

	if ec.moreHallOrdersBelow() {
		return true
	}
	return false
}

func (ec *ElevatorController) moreHallOrdersBelow() bool {
	state := ec.GetElevatorState()

	if state.CurrentFloor == 0 {
		return false
	}

	for _, orders := range state.HallOrders[:state.CurrentFloor] {
		for _, orderBelow := range orders {
			if orderBelow {
				return true
			}
		}
	}

	return false
}

func (ec *ElevatorController) moreCabOrdersBelow() bool {
	state := ec.GetElevatorState()

	if state.CurrentFloor == 0 {
		return false
	}

	for _, orderBelow := range state.CabOrders[:state.CurrentFloor] {
		if orderBelow {
			return true
		}
	}
	return false
}

func (ec *ElevatorController) moreOrders() bool {
	state := ec.GetElevatorState()
	// NOTE: might change to depend on other functions as some of the checks are already in functions
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
	floor := state.CurrentFloor

	if state.CabOrders[floor] || state.HallOrders[floor][up] || floor == maxFloor {
		ec.stopElevatorAtCurrentFloor()
		ec.clearCabOrder(state.CurrentFloor)
		ec.clearHallorder(floor, up)
	}

	if ec.moreOrdersAbove() {
		ec.setState(MovingUp)
		ec.elevatorDriveUp()
		ec.clearHallorder(floor, up)
		return
	}

	if ec.moreOrdersBelow() {
		ec.setState(MovingDown)
		ec.elevatorDriveDown()
		ec.clearHallorder(floor, down)
		return
	}

	if state.HallOrders[floor][down] {
		ec.stopElevatorAtCurrentFloor()
		ec.clearHallorder(floor, down)
	}

	ec.setState(Idle)
}

func (ec *ElevatorController) handleArrivalAtFloorGoingDown() {
	state := ec.GetElevatorState()
	floor := state.CurrentFloor

	if state.CabOrders[floor] || state.HallOrders[floor][down] || floor == 0 {
		ec.stopElevatorAtCurrentFloor()
		ec.clearHallorder(floor, down)
		ec.clearCabOrder(state.CurrentFloor)
	}

	if ec.moreOrdersBelow() {
		ec.setState(MovingDown)
		ec.elevatorDriveDown()
		ec.clearHallorder(floor, down)
		return
	}

	if ec.moreOrdersAbove() {
		ec.setState(MovingUp)
		ec.elevatorDriveUp()
		ec.clearHallorder(floor, up)
		return
	}

	if state.HallOrders[floor][up] {
		ec.stopElevatorAtCurrentFloor()
		ec.clearHallorder(floor, up)
	}
	ec.setState(Idle)
}

func (ec *ElevatorController) handleNewCabOrder() {
	//should keep moving up only if there are moe cab orders above, and vice versa for down
	state := ec.GetElevatorState()

	headingUp := (state.State == MovingUp) || (state.State == DoorOpenHeadingUp)
	headingDown := (state.State == MovingDown) || (state.State == DoorOpenHeadingDown)

	//continue in the direction of travel if there are more cab orders in that direction
	if ec.moreCabOrdersAbove() && headingUp {
		return
	} else if ec.moreCabOrdersBelow() && headingDown {
		return
	}

	// set the direction as according to the new orders
	if ec.moreCabOrdersBelow() {
		ec.setState(MovingDown)
		ec.elevatorDriveDown()
	} else if ec.moreCabOrdersAbove() {
		ec.setState(MovingUp)
		ec.elevatorDriveUp()
	}

}

func (ec *ElevatorController) handleNewHallOrder() {
	state := ec.GetElevatorState()
	floor := state.CurrentFloor

	switch state.State {
	case MovingDown:
		return
	case MovingUp:
		return
	case DoorOpenHeadingDown:
		return
	case DoorOpenHeadingUp:
		return
	case Idle:
		state := ec.GetElevatorState()
		if state.HallOrders[floor][up] {
			ec.setState(MovingUp)
			ec.handleArrivalAtFloorGoingUp()
			return
		}

		if state.HallOrders[floor][down] {
			ec.setState(MovingDown)
			ec.handleArrivalAtFloorGoingDown()
			return
		}

		if ec.moreOrdersAbove() {
			ec.setState(MovingUp)
			ec.elevatorDriveUp()
			return
		}

		if ec.moreOrdersBelow() {
			ec.setState(MovingDown)
			ec.elevatorDriveDown()
			return
		}
	case DoorOpenIdle:
		if state.HallOrders[floor][up] {
			ec.setState(MovingUp)
			ec.handleArrivalAtFloorGoingUp()
			return
		}

		if state.HallOrders[floor][down] {
			ec.setState(MovingDown)
			ec.handleArrivalAtFloorGoingDown()
			return
		}

		if ec.moreOrdersAbove() {
			ec.setState(MovingUp)
			ec.elevatorDriveUp()
			return
		}

		if ec.moreOrdersBelow() {
			ec.setState(MovingDown)
			ec.elevatorDriveDown()
			return
		}

	case Obstructed:
		//This is handled by others, and might not need to be taken into account
	}
}

func (ec *ElevatorController) openDoor() {
	elevio.SetDoorOpenLamp(true)
}

func (ec *ElevatorController) closeDoor() {
	savedState := ec.GetElevatorState()
	obstrucitonCh := ec.SubscribeObstruction()
	defer ec.UnsubscribeObstruction(obstrucitonCh)

	if elevio.GetObstruction() {
		ec.setState(Obstructed)
		for range obstrucitonCh {
			if !elevio.GetObstruction() {
				ec.setState(savedState.State)
				break
			}
		}
	}

	elevio.SetDoorOpenLamp(false)

} // FIX: Cant handle repeated obstruction
// NOTE: Might not need to handle repeated obstruction, as the doors are "Instantanious"

func (ec *ElevatorController) stopElevatorAtCurrentFloor() {
	ec.stopElevator()
	ec.openDoor()
	state := ec.GetElevatorState()

	switch state.State {
	case MovingUp:
		ec.setState(DoorOpenHeadingUp)
	case MovingDown:
		ec.setState(DoorOpenHeadingDown)
	default:
		ec.setState(DoorOpenIdle)
	}

	time.Sleep(doorOpenTime)
	ec.closeDoor()

	state = ec.GetElevatorState()
	switch state.State {
	case DoorOpenHeadingUp:
		ec.setState(MovingUp)
	case DoorOpenHeadingDown:
		ec.setState(MovingDown)
	case DoorOpenIdle:
		ec.setState(Idle)
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
	cabOrderCh := ec.SubscribeCabOrders()
	hallOrderCh := ec.SubscribeHallOrders()

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
	ec.elevator.PressedCabButtons[floor] = false
	ec.notfiyCabOrders()
	ec.notifyState()
	ec.notfiyButton()
}

func (ec *ElevatorController) clearHallorder(floor int, direction directions) {
	ec.stateLock.Lock()
	defer ec.stateLock.Unlock()
	ec.elevator.HallOrders[floor][direction] = false
	ec.elevator.PressedHallButtons[floor][direction] = false
	ec.notfiyHallOrders()
	ec.notifyState()
}

func (es ElevatorState) String() string {
	switch es {
	case Idle:
		return "Idle"
	case MovingUp:
		return "Moving Up"
	case MovingDown:
		return "Moving Down"
	case DoorOpenHeadingUp:
		return "Door Open Moving Up"
	case DoorOpenHeadingDown:
		return "Door Open Moving Down"
	case DoorOpenIdle:
		return "Door Open Idle"
	case Obstructed:
		return "Obstructed"
	default:
		return ""
	}

}

// TODO: Add functionality to get configs from a file or as input to init
// TODO: Find what parts might benefit from an acceptance test
// TODO: Create Acceptance tests for the parts that need it
// FIX: The elevator does not anounce change in direction if the cab orders make the instrucitons different
// FIX: If it crashes between floors, no information on startup
// NOTE: Drive elevator up/down does not consider if the door is open or obstructed
