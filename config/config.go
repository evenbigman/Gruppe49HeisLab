package config

import "time"

const (
	NumFloors         = 4
	MaxFloor          = NumFloors - 1
	BcastPort         = 16569
	BcastInterval     = 100 //ms
	TimeoutInterval   = 1000
	ConnectionTimeThreshold = 2000 * time.Millisecond
	DoorOpenTime      = 3 * time.Second
	InitDelay         = 5 * time.Second
	TimeoutAck        = 1 * time.Second
	DefaultElevioPort = "15657"
)
