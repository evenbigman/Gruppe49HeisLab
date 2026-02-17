package controller

import (
	"fmt"
	"sanntidslab/elevio"
	"time"
)

const (
	numFloors    = 4
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

func RunElevator(elevator chan Elevator, floor chan int, que chan [numFloors]bool) {
	//Runs the elevator, and continues in the direction of travle as long as there are more orders in the que
	//Does not handle buttonpresses
	var internalQue [numFloors]bool
	var upFlag, downFlag, stopFlag bool
	var internalElevator Elevator
	fmt.Println("Eleveator Controller started")
	for {
		select {
		case v := <-floor:
			internalElevator.CurrentFloor = v
			switch internalElevator.Direction {
			case MovingUp:
				fmt.Println("Going up and is at floor", internalElevator.CurrentFloor)
				//should check if the elevator should keep moving up, based on the elements in the que
				for i := 0; i <= numFloors-1; i++ {
					if !internalQue[i] {
						continue
					}
					if i < internalElevator.CurrentFloor {
						//set a flag for to make the elevator go down
						downFlag = true

					} else if i == internalElevator.CurrentFloor {
						stopFlag = true
					} else if i > internalElevator.CurrentFloor {
						//sets a flag to mark the direction as up, and clears the downflag
						upFlag = true
						downFlag = false
						break
					}
				}

				if stopFlag {
					//stops the elevator, anounces the direction of travel, and waits for the required amount of time
					elevio.SetMotorDirection(elevio.MD_Stop)
					internalElevator.Direction = Stopped
					elevio.SetDoorOpenLamp(true)
					if downFlag {
						elevio.SetButtonLamp(elevio.BT_HallDown, internalElevator.CurrentFloor, false)
					} else if upFlag {
						elevio.SetButtonLamp(elevio.BT_HallUp, internalElevator.CurrentFloor, false)
					}
					time.Sleep(doorOpenTime * time.Second)
				}
				if upFlag {
					elevio.SetMotorDirection(elevio.MD_Up)
					internalElevator.Direction = MovingUp
				}
				if downFlag {
					elevio.SetMotorDirection(elevio.MD_Down)
					internalElevator.Direction = MovingDown
				}

			case MovingDown:
				fmt.Println("Going down and is at floor", internalElevator.CurrentFloor)
				//should check if the elevator should keep moving down, based on the elements in the que
				for i := 0; i <= numFloors; i++ {
					if !internalQue[i] {
						continue
					}
					if i > internalElevator.CurrentFloor {
						//set a flag for to make the elevator go down
						upFlag = true

					} else if i == internalElevator.CurrentFloor {
						stopFlag = true
					} else if i < internalElevator.CurrentFloor {
						//sets a flag to mark the direction as up, and clears the downflag
						upFlag = false
						downFlag = true
						break
					}
				}

				if stopFlag {
					//stops the elevator, anounces the direction of travel, and waits for the required amount of time
					elevio.SetMotorDirection(elevio.MD_Stop)
					internalElevator.Direction = Stopped
					elevio.SetDoorOpenLamp(true)
					if downFlag {
						elevio.SetButtonLamp(elevio.BT_HallDown, internalElevator.CurrentFloor, false)
					} else if upFlag {
						elevio.SetButtonLamp(elevio.BT_HallUp, internalElevator.CurrentFloor, false)
					}
					time.Sleep(doorOpenTime * time.Second)
				}
				if upFlag {
					elevio.SetMotorDirection(elevio.MD_Up)
					internalElevator.Direction = MovingUp
				}
				if downFlag {
					elevio.SetMotorDirection(elevio.MD_Down)
					internalElevator.Direction = MovingDown
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
