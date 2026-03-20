package controller

import (
	"fmt"
	"sanntidslab/config"
	"sanntidslab/elevio"
	"strings"
	"sync"
	"time"
)

/********************************************************************************
*************************     Elevator Controller       *************************

This module contains the control of the local elevator, and serves as an
abstraction between the elevator driver provided on the course github.
It should register button-events, arrival at floor, handling of assigned and
confirmed orders as well as stopping both when the elevaotr motor malfunctions
and when the door is obstructed from closing.

********************************************************************************/

const (
	//some constants used for the elevators to avoid magic numbers
	numFloors      = config.NumFloors
	maxFloor       = config.MaxFloor
	doorOpenTime   = config.DoorOpenTime
	defaultPort    = config.DefaultElevioPort
	undefined      = -1
	motorTimeout   = 10 * time.Second
	motorRetryTime = 2 * time.Second
)

type directions int

const (
	up directions = iota
	down
)

type HallOrders [numFloors][2]bool
type CabOrders [numFloors]bool

type ElevatorState int

const (
	Idle ElevatorState = iota
	MovingUp
	MovingDown
	DoorOpenHeadingUp
	DoorOpenHeadingDown
	DoorOpenIdle
	Obstructed
	MotorFailure
)

type Elevator struct {
	CurrentFloor        int
	AssignedHallOrders  HallOrders
	ConfirmedHallOrders HallOrders
	CabOrders           CabOrders
	State               ElevatorState
	PressedHallButtons  HallOrders
	PressedCabButtons   CabOrders
}

type ElevatorController struct {
	elevator               Elevator
	stateLock              sync.RWMutex
	initOnce               sync.Once
	stateChangeSubscribers []chan struct{}
	floorSubscribers       []chan struct{}
	cabButtonSubscribers   []chan struct{}
	hallOrderSubscriber    []chan struct{}
	cabOrderSubscriber     []chan struct{}
	obstructionSubscriber  []chan struct{}
	subsLock               sync.RWMutex
}

var (
	instance     *ElevatorController
	instanceOnce sync.Once
)

/*
*******************************************************************************
**********************       Public functions       *****************************

The following functions are the ones that are used by external modules to either
run the elevator, or to set orders according to signaling from the other elevators

*******************************************************************************
*/
func GetController() *ElevatorController {
	instanceOnce.Do(func() {
		instance = &ElevatorController{
			elevator: Elevator{
				CurrentFloor: undefined,
				State:        Idle,
			},
		}
	})
	return instance
}

func (ec *ElevatorController) Start() error {

	if instance == nil {
		return fmt.Errorf("Error, not initialized. Call InitElevator, or InitElevatorWithConfig first")
	}
	go ec.completeWaitingOrders()
	go ec.handleLights()
	go ec.watchdogMotor()
	return nil
}

func (ec *ElevatorController) InitElevator(port ...string) {

	ec.initOnce.Do(func() {
		p := defaultPort
		if len(port) > 0 {
			p = port[0]
		}
		address := "localhost:" + p
		elevio.Init(address, numFloors)

		go ec.pollElevatorState()
		floorch := ec.SubscribeFloor()
		defer ec.UnsubscribeFloor(floorch)

		ec.stateLock.Lock()
		ec.elevator.CurrentFloor = elevio.GetFloor()
		ec.stateLock.Unlock()

		state := ec.GetElevatorState()
		if state.CurrentFloor == -1 {
			ec.closeDoor()
			ec.elevatorDriveDown()
			ec.setState(MovingDown)
			for range floorch {
				break
			}
			ec.stopElevator()
			ec.setState(Idle)
		}
	})
}

func (ec *ElevatorController) InitElevatorWithStates(elevator Elevator, port ...string) {
	ec.elevator = elevator

	p := defaultPort
	if len(port) > 0 {
		p = port[0]
	}
	ec.InitElevator(p)
}

func (ec *ElevatorController) SetGlobalHallOrders(confirmedHallOrders [numFloors][2]bool) {
	ec.stateLock.Lock()
	defer ec.stateLock.Unlock()
	ec.elevator.ConfirmedHallOrders = confirmedHallOrders
	ec.notifyState()
	ec.notifyHallOrders()
}

func (ec *ElevatorController) SetPressedHallButtons(pressedButtons HallOrders) {
	ec.stateLock.Lock()
	defer ec.stateLock.Unlock()
	ec.elevator.PressedHallButtons = pressedButtons
	ec.notifyState()
}

func (ec *ElevatorController) AssignHallOrders(assignedHallOrders [numFloors][2]bool) {
	ec.stateLock.Lock()
	defer ec.stateLock.Unlock()
	ec.elevator.AssignedHallOrders = assignedHallOrders
	ec.notifyHallOrders()

}

func (ec *ElevatorController) SetCabOrders(confirmedCabOrders [numFloors]bool) {
	ec.stateLock.Lock()
	defer ec.stateLock.Unlock()
	ec.elevator.CabOrders = confirmedCabOrders
	ec.notifyCabOrders()
}

func (ec *ElevatorController) GetElevatorState() Elevator {
	//locking to ensure no updates happen while it reads the state of the elevator
	ec.stateLock.RLock()
	defer ec.stateLock.RUnlock()
	return ec.elevator
}

/********************************************************************************
******************        Subscribe and notify        ***************************

The foloowing few functions (names with subscribe and notify) are used for
signaling between goroutines. If a routine needs to know about an event,
it can use the relevant subscribe function, which lets the subscribe-function to
get notified when the relevant event happens in the controller.
When the elevator changes something, it will signal that the event happened to
the relevant subscribers with the notify-functions.

********************************************************************************/

func (ec *ElevatorController) SubscribeState() <-chan struct{} { // NOTE: might be better with a subscirbe elevator as name, as it is inteded to be used when the elevator in general gets an update
	return ec.addSubscriber(&ec.stateChangeSubscribers)
}

func (ec *ElevatorController) SubscribeFloor() <-chan struct{} {
	return ec.addSubscriber(&ec.floorSubscribers)
}

func (ec *ElevatorController) SubscribeCabButtons() <-chan struct{} {
	return ec.addSubscriber(&ec.cabButtonSubscribers)
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

func (ec *ElevatorController) UnsubscribeFloor(subber <-chan struct{}) {
	ec.removeSubscriber(&ec.floorSubscribers, subber)
}

func (ec *ElevatorController) UnsubscribeState(subber <-chan struct{}) {
	ec.removeSubscriber(&ec.stateChangeSubscribers, subber)
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

func (ec *ElevatorController) notifyCabButton() {
	ec.notify(&ec.cabButtonSubscribers)
}

func (ec *ElevatorController) notifyHallOrders() {
	ec.notify(&ec.hallOrderSubscriber)
}

func (ec *ElevatorController) notifyCabOrders() {
	ec.notify(&ec.cabOrderSubscriber)
}

func (ec *ElevatorController) notifyObstruciton() {
	ec.notify(&ec.obstructionSubscriber)
}

func (ec *ElevatorController) notify(subscribers *[]chan struct{}) {
	ec.subsLock.RLock()
	defer ec.subsLock.RUnlock()
	for _, ch := range *subscribers {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (ec *ElevatorController) pollElevatorState() {
	/* Polling the elevator state, and updates the appropriate vairables for the local
	 * elevator, as well as signaling the relevant channels*/

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

				ec.notifyCabButton()
			}
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
			default:
				ec.openAndCloseDoorAtcurrentFloor()
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

	for _, orders := range state.AssignedHallOrders[floorAbove:] {
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

	for _, orders := range state.AssignedHallOrders[:state.CurrentFloor] {
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
	for _, orders := range state.AssignedHallOrders {
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

	if state.CabOrders[floor] || state.AssignedHallOrders[floor][up] {
		ec.stopElevator()
		time.Sleep(config.ElevatorClearDelay)
		// this delay had to be implemented to compensate for too fast clearing of orders, which created an inconsistent state
		ec.clearCabOrder(state.CurrentFloor)
		ec.clearHallorder(floor, up)
		ec.openAndCloseDoorAtcurrentFloor()
	}

	if floor == maxFloor {
		ec.stopElevator()
		time.Sleep(config.ElevatorClearDelay)
		ec.clearHallorder(floor, down)
		ec.clearCabOrder(state.CurrentFloor)
		ec.openAndCloseDoorAtcurrentFloor()
	}

	if ec.moreOrdersAbove() {
		ec.setState(MovingUp)
		ec.elevatorDriveUp()
		return
	}

	if ec.moreOrdersBelow() {
		ec.setState(MovingDown)
		ec.openAndCloseDoorAtcurrentFloor()
		ec.elevatorDriveDown()
		return
	}

	if state.AssignedHallOrders[floor][down] {
		ec.openAndCloseDoorAtcurrentFloor()
		ec.clearHallorder(floor, down)
	}

	ec.setState(Idle)
}

func (ec *ElevatorController) handleArrivalAtFloorGoingDown() {
	state := ec.GetElevatorState()
	floor := state.CurrentFloor

	if state.CabOrders[floor] || state.AssignedHallOrders[floor][down] {
		ec.stopElevator()
		time.Sleep(config.ElevatorClearDelay)
		ec.clearHallorder(floor, down)
		ec.clearCabOrder(state.CurrentFloor)
		ec.openAndCloseDoorAtcurrentFloor()
	}

	if floor == 0 {
		ec.stopElevator()
		time.Sleep(config.ElevatorClearDelay)
		ec.clearHallorder(floor, up)
		ec.clearCabOrder(state.CurrentFloor)
		ec.openAndCloseDoorAtcurrentFloor()

	}

	if ec.moreOrdersBelow() {
		ec.setState(MovingDown)
		ec.elevatorDriveDown()
		return
	}

	if ec.moreOrdersAbove() {
		ec.setState(MovingUp)
		ec.openAndCloseDoorAtcurrentFloor()
		ec.elevatorDriveUp()
		return
	}

	if state.AssignedHallOrders[floor][up] {
		ec.clearHallorder(floor, up)
		ec.openAndCloseDoorAtcurrentFloor()
	}
	ec.setState(Idle)
}

func (ec *ElevatorController) handleNewCabOrder() {
	//should keep moving up only if there are more cab orders above, and vice versa for down
	state := ec.GetElevatorState()

	moving := (state.State == MovingDown) || (state.State == MovingUp)

	headingUp := (state.State == MovingUp) || (state.State == DoorOpenHeadingUp)
	headingDown := (state.State == MovingDown) || (state.State == DoorOpenHeadingDown)

	if moving {
		return
	}

	if !moving && state.CabOrders[state.CurrentFloor] {
		ec.clearCabOrder(state.CurrentFloor)
		ec.openAndCloseDoorAtcurrentFloor()
	}

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
		state = ec.GetElevatorState()
		if state.AssignedHallOrders[floor][up] {
			ec.setState(MovingUp)
			ec.handleArrivalAtFloorGoingUp()
			return
		}

		if state.AssignedHallOrders[floor][down] {
			ec.setState(MovingDown)
			ec.handleArrivalAtFloorGoingDown()
			return
		}

		if ec.moreOrdersAbove() {
			fmt.Println("")
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
		if state.AssignedHallOrders[floor][up] {
			ec.setState(MovingUp)
			ec.handleArrivalAtFloorGoingUp()
			return
		}

		if state.AssignedHallOrders[floor][down] {
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

func (ec *ElevatorController) openDoor() error {
	if elevio.IsAtFloor() {
		elevio.SetDoorOpenLamp(true)
		return nil
	} else {
		return fmt.Errorf("Failed to open door, was not at a floor")
	}
}

func (ec *ElevatorController) closeDoor() {
	savedState := ec.GetElevatorState()
	obstrucitonCh := ec.SubscribeObstruction()
	defer ec.UnsubscribeObstruction(obstrucitonCh)

	//potential new solution to obstructions:
	/*
		closeDoorTimer := time.NewTimer(doorOpenTime)
		for {
			select{
			case <-closeDoorTimer.C:
			if elevio.GetObstruction {
				closeDoorTimer.Reset(doorOpenTime)
				ec.setState(Obstructed)
			} else {
				elevio.SetDoorOpenLamp(false)
				ec.setState(savedState.State)
				return
			}
			default:
			if elevio.GetObstruction {
				ec.setState(Obstructed)
				closeDoorTimer.Reset(doorOpenTime)

			}

			}
		}
	*/

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

}

func (ec *ElevatorController) openAndCloseDoorAtcurrentFloor() error {
	state := ec.GetElevatorState()
	ec.stopElevator()
	err := ec.openDoor()
	if err != nil {
		return err
	}

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
	return nil

}

func (ec *ElevatorController) stopElevator() {
	elevio.SetMotorDirection(elevio.MD_Stop)
}

func (ec *ElevatorController) elevatorDriveUp() {
	ec.setState(MovingUp)
	elevio.SetMotorDirection(elevio.MD_Up)
}

func (ec *ElevatorController) elevatorDriveDown() {
	ec.setState(MovingDown)
	elevio.SetMotorDirection(elevio.MD_Down)
}

func (ec *ElevatorController) handleLights() {
	cabOrderCh := ec.SubscribeCabOrders()
	hallOrderCh := ec.SubscribeHallOrders()
	tickerUpdate := time.NewTicker(500 * time.Millisecond)

	for {
		select {
		case <-cabOrderCh:
			ec.handleCabLights()
		case <-hallOrderCh:
			ec.handleHallLights()
		case <-tickerUpdate.C:
			ec.handleHallLights()
			ec.handleCabLights()
		}
	}

}

func (ec *ElevatorController) handleCabLights() {
	state := ec.GetElevatorState()
	for floor, order := range state.CabOrders {
		elevio.SetButtonLamp(elevio.BT_Cab, floor, order)
	}

}

func (ec *ElevatorController) handleHallLights() {
	state := ec.GetElevatorState()
	for floor, orders := range state.ConfirmedHallOrders {
		for i, value := range orders {
			elevio.SetButtonLamp(elevio.ButtonType(i), floor, value)
		}
	}
}

func (ec *ElevatorController) clearCabOrder(floor int) {
	ec.stateLock.Lock()
	defer ec.stateLock.Unlock()
	ec.elevator.CabOrders[floor] = false
	ec.elevator.PressedCabButtons[floor] = false
	ec.notifyCabOrders()
	ec.notifyState()
}

func (ec *ElevatorController) clearHallorder(floor int, direction directions) {
	ec.stateLock.Lock()
	defer ec.stateLock.Unlock()
	ec.elevator.AssignedHallOrders[floor][direction] = false
	ec.elevator.PressedHallButtons[floor][direction] = false
	ec.notifyHallOrders()
	ec.notifyState()
}

func (ec *ElevatorController) watchdogMotor() {
	floorCh := ec.SubscribeFloor()
	stateCh := ec.SubscribeState()
	timeout := time.NewTimer(motorTimeout)

	for {
		select {
		case <-stateCh:
			state := ec.GetElevatorState()
			if state.State != MotorFailure {
				timeout.Reset(motorTimeout)
			}
		case <-floorCh:
			timeout.Reset(motorTimeout)
		case <-timeout.C:
			state := ec.GetElevatorState()
			if state.State == MovingUp || state.State == MovingDown {
				ec.handleMotorFailure()
			} else {
				timeout.Reset(motorTimeout)
			}
		}
	}
}

func (ec *ElevatorController) handleMotorFailure() {
	//currently notifies all other parts of the system that the motor is not working, and retries running the motor
	//Might not be the best way to fail
	fmt.Println("Motor failed")
	floorCh := ec.SubscribeFloor()
	defer ec.UnsubscribeFloor(floorCh)
	savedState := ec.GetElevatorState()
	retryTimer := time.NewTimer(motorRetryTime)

	ec.setState(MotorFailure)
	ec.notifyState()

retry:
	for {
		fmt.Println("Retry in ", motorRetryTime)
		select {
		case <-floorCh:
			ec.setState(savedState.State)
			break retry
		case <-retryTimer.C:
			switch savedState.State {
			case MovingDown:
				ec.elevatorDriveDown()
			case MovingUp:
				ec.elevatorDriveUp()
			}
			retryTimer.Reset(motorRetryTime)
		}
	}
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
	case MotorFailure:
		return "MotorFailure"
	default:
		return fmt.Sprintf("%d", es)
	}

}

func (hallOrder HallOrders) String() string {
	var str strings.Builder
	for floor, orders := range hallOrder {
		fmt.Fprintf(&str, "Hall orders for floor %d are: %v \n", floor, orders)
	}
	return str.String()
}

func (cabOrders CabOrders) String() string {
	var str strings.Builder
	for floor, orders := range cabOrders {
		fmt.Fprintf(&str, "Cab order for floor %d is: %v \n", floor, orders)
	}
	return str.String()
}

func (e Elevator) String() string {
	str := fmt.Sprintf("Current floor: %d \n", e.CurrentFloor)
	str += fmt.Sprintf("Assigned Orders: %s \n", e.AssignedHallOrders)
	str += fmt.Sprintf("Confirmed hall orders: %s \n", e.ConfirmedHallOrders)
	str += fmt.Sprintf("CabOrders: %s \n", e.CabOrders)
	str += fmt.Sprintf("State: %s \n", e.State)
	str += fmt.Sprintf("Pressed Hall buttons: %s \n", e.PressedHallButtons)
	str += fmt.Sprintf("Pressed Cab Buttons: %s \n", e.PressedCabButtons)

	return str
}
