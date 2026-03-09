package peers

import(
	"sanntidslab/peers/status"
	"sanntidslab/peers/snapshots"
	"sanntidslab/network/bcast"
	"sanntidslab/config"
	"time"
	"log"
	"sync"
)

type Msg struct{
	Sender string
	Snapshots map[string]snapshots.Snapshot
}

