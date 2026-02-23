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
	"encoding/json"
	"math/rand/v2"
	"sanntidslab/config"
	"time"
	"log"
	"strconv"
)

type Msg struct{
	Sender string
	States map[string]State
}

type Order struct{
	Id int
	Floor int
	Direction int//Up/down/cab
}

type State struct{
	Version int
	LastSeen time.Time
	Floor int
	Direction string
	Door string
	HallButtons [config.Floors]int//None/Up/Down/Both
	CabButtons [config.Floors]bool
	Orders []Order
	CompletedOrders []Order
}

var States = make(map[string]State)

var myId string

func Broadcaster(){
// ------Testing------
	myId = strconv.Itoa(rand.Int())
	log.Printf("Started broadcaster with id: %s \r\n", myId)
	
	myState := State{
		Version: 1,
		LastSeen: time.Now(),
		Floor: 2,
		Direction: "stop",
		Door: "closed",
		HallButtons: [4]int{0,0,0,0},
		CabButtons: [4]bool{false, false, false, false},
		Orders: []Order{},
		CompletedOrders: []Order{},
	}
	
	States[myId] = myState

	jsondata, _ := json.MarshalIndent(States, "", "  ")	
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

				for rcvdId, rcvdState := range msg.States{
					HandleReceivedState(rcvdId, rcvdState)
				}

				for id, _ := range States{ log.Printf("Saved id: %x \r\n", id) }
				//jsondata, _ := json.MarshalIndent(States, "", "  ")	
				//log.Println(string(jsondata))
			}

		case <-ticker.C:
			tx <- Msg{Sender: myId, States: States}
			log.Printf("Sent message\r\n")
		}
	}
}

func HandleReceivedState(id string, state State){
	//Either store, update or ignore received state
	if _, ok := States[id]; !ok{ //If state is not stored
		States[id] = state
	} else if state.Version > States[id].Version	{
		//Check if order is to be handled
		States[id] = state
	}
}
