package config

import "time"

const (
	NumFloors         = 4
	MaxFloor          = NumFloors - 1
	BcastPort         = 16569
	BcastInterval     = 250 //ms
	TimeoutInterval   = 2000
	DoorOpenTime      = 3 * time.Second
	InitDelay         = 5 * time.Second
	TimeoutAck        = 1000 * time.Second
	DefaultElevioPort = "15657"
)
