package controller

import (
	"fmt"
	"sanntidslab/elevio"
	"time"
)

const (
	numFloors    = 4
	maxFloor     = numFloors - 1
	doorOpenTime = 3
)

type ElevatorStatus int

const (
	Idle ElevatorStatus = iota
	Driving
	DoorOpen
	Obstruction
)

type Direction int

const (
	Stopped Direction = iota
	MovingUp
	MovingDown
)

type Elevator struct {
	CurrentFloor int
	NextFloor    int
	Orders       [numFloors]bool
	Direction    Direction
	State        ElevatorStatus
}

func InitElevator(elevator *Elevator, floorCh chan int) {
	fmt.Println("Initializing elevator")
	if elevator.CurrentFloor < 0 || elevator.CurrentFloor > 4 {
		elevio.SetMotorDirection(elevio.MD_Down)
	}
	for v := range floorCh {
		elevio.SetMotorDirection(elevio.MD_Stop)
		elevator.CurrentFloor = v
		fmt.Println("Elevator Initialized")
		break
	}
}

func RunElevator(elevator chan Elevator, floor chan int, que chan [numFloors]bool) {
	//Runs the elevator, and continues in the direction of travle as long as there are more orders in the que
	//Does not handle buttonpresses
	var internalQue [numFloors]bool
	var upFlag, downFlag, stopFlag bool
	var internalElevator Elevator
	fmt.Println("Elevator Controller started")
	for {
		select {
		case v := <-floor:
			internalElevator.CurrentFloor = v
			if internalElevator.CurrentFloor == maxFloor || internalElevator.CurrentFloor == 0 {
				stopFlag = true
			}
			for i := 0; i <= maxFloor; i++ {
				if !internalQue[i] {
					continue
				}
				if i < internalElevator.CurrentFloor {
					//set a flag for to make the elevator go down
					downFlag = true
				} else if i == internalElevator.CurrentFloor {
					stopFlag = true
				} else if i > internalElevator.CurrentFloor {
					//sets a flag to mark the next direction as up, and clears the downflag
					upFlag = true
					break
				}
			}

			if stopFlag {
				//stops the elevator, anounces the direction of travel, and waits for the required amount of time
				elevatorStop(&internalElevator, upFlag, downFlag)
			}

			switch internalElevator.Direction {
			case MovingUp:
				fmt.Println("Going up and is at floor", internalElevator.CurrentFloor)
				//should check if the elevator should keep moving up, based on the elements in the que
				if upFlag {
					elevio.SetMotorDirection(elevio.MD_Up)
					internalElevator.Direction = MovingUp
				} else if downFlag {
					elevio.SetMotorDirection(elevio.MD_Down)
					internalElevator.Direction = MovingDown
				}

			case MovingDown:
				fmt.Println("Going down and is at floor", internalElevator.CurrentFloor)
				//should check if the elevator should keep moving down, based on the elements in the que
				if downFlag {
					elevio.SetMotorDirection(elevio.MD_Down)
					internalElevator.Direction = MovingDown
				} else if upFlag {
					elevio.SetMotorDirection(elevio.MD_Up)
					internalElevator.Direction = MovingUp
				}
			}

		case v := <-que:
			//Updates the que that is used by this function to run the elevator
			//Should also make the elevator start moving if it is at rest
			internalQue = v
			fmt.Println(internalQue)

		case v := <-elevator:
			internalElevator = v

		case elevator <- internalElevator:
		}

	}

}

func elevatorStop(elevator *Elevator, upFlag bool, downFlag bool) {
	elevio.SetMotorDirection(elevio.MD_Stop)
	elevator.Direction = Stopped
	elevio.SetDoorOpenLamp(true)
	if downFlag {
		elevio.SetButtonLamp(elevio.BT_HallDown, elevator.CurrentFloor, false)
	} else if upFlag {
		elevio.SetButtonLamp(elevio.BT_HallUp, elevator.CurrentFloor, false)
	}
	time.Sleep(doorOpenTime * time.Second)

}
