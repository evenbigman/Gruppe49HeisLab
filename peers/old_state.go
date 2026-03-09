package state
/*
Thoughts:
	Broadcast FULL state all the time, implicit heartbeat and ACK.
	Decide if order is worth taking isolated.
	Maybe track order state?
*/

import(
	"sanntidslab/network/bcast"
	//"sanntidslab/network/localip"
	"sanntidslab/controller"
	"encoding/json"
	"math/rand/v2"
	"sanntidslab/config"
	"time"
	"log"
	"strconv"
)

type Elevator struct {
	LastSeen time.Time
	Version int
	State controller.Elevator
}

type Msg struct{
	Sender string
	Elevators map[string]Elevator
}

var Elevators = make(map[string]Elevator)
var myId string

func Broadcaster(){
// ------Testing------
	myId = strconv.Itoa(rand.Int())
	log.Printf("Started broadcaster with id: %s \r\n", myId)
	myElevatorState := controller.Elevator{
		CurrentFloor: 0,
		NextFloor: 3,
		Direction: 0,
		State: 0,
		//HallButtons: [config.Floors]int{0,0,0,0},
		//CabButtons: [config.Floors]bool{false, false, false, false},
	}
	
	myElevator := Elevator{
		Version: 1,
		LastSeen: time.Now(),
		State: myElevatorState,
	}
	
	Elevators[myId] = myElevator

	jsondata, _ := json.MarshalIndent(Elevators, "", "  ")	
	log.Println(string(jsondata))
// -------------------

	tx := make(chan Msg)
	rx := make(chan Msg)
	
	go bcast.Transmitter(config.BcastPort, tx)
	go bcast.Receiver(config.BcastPort, rx)

	ticker := time.NewTicker(config.BcastInterval * time.Millisecond)
	defer ticker.Stop()

	for{
		select{
		case msg := <-rx:
			if msg.Sender != myId{
				log.Printf("Received message\r\n")

				for rcvdId, rcvdElevator := range msg.Elevators{
					HandleReceivedState(rcvdId, rcvdElevator)
				}
				rcvdElevator := Elevators[msg.Sender]
				rcvdElevator.LastSeen = time.Now()
				Elevators[msg.Sender] = rcvdElevator

				for id, _ := range Elevators{ log.Printf("Saved id: %x \r\n", id) }
				//jsondata, _ := json.MarshalIndent(States, "", "  ")	
				//log.Println(string(jsondata))
			}

		case <-ticker.C:
			tx <- Msg{Sender: myId, Elevators: Elevators}
			log.Printf("Sent message\r\n")
		}
	}
}

func HandleReceivedState(id string, elevator Elevator){
	//Either store, update or ignore received state
	if _, ok := Elevators[id]; !ok || //If state is not stored
	elevator.Version > Elevators[id].Version {  //If rcvd elevator state is newer than stored state
		Elevators[id] = elevator
	}
}
