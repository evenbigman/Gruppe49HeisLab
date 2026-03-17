package config

import "time"

const (
	NumFloors         = 4
	MaxFloor          = NumFloors - 1
	BcastPort         = 16569
	BcastInterval     = 1000 //ms
	TimeoutInterval   = 4000
	DoorOpenTime      = 3 * time.Second
	InitDelay         = 5 * time.Second
	TimeoutAck        = 4000 * time.Second
	DefaultElevioPort = "15657"
)
