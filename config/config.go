package config

import "time"

const (
	NumFloors         = 4
	MaxFloor          = NumFloors - 1
	BcastPort         = 16569
	BcastInterval     = 4000 //ms
	TimeoutInterval   = 3000
	DoorOpenTime      = 3 * time.Second
	InitDelay         = 5 * time.Second
	TimeoutAck        = 500 * time.Second
	DefaultElevioPort = "15657"
)
