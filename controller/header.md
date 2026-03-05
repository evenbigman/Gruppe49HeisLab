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

# public Functions
func CompleteWaitingOrders()
func InitElevator()
func SetHallOrders(confirmedHallOrders [numFloors][2]bool)
func SetCabOrders(confirmedCabOrders [numFloors]bool)
func GetElevatorState() Elevator


# private funcitons
func updateElevatorState()
func moreOrdersAbove() bool
func moreOrdersBelow() bool
func moreOrders() bool
func setLight(floor int, lightType elevio.ButtonType, lightState bool)
func openDoor()
func closeDoor()
func goToFloor(orderedFloor int)
func stopElevatorAtCurrentFloor()
func stopElevator()
func elevatorDriveDown()
func elevatorDriveUp()
