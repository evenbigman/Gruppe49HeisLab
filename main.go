package main

import (
	"fmt"
	backuphandler "sanntidslab/backup_handler"
	"time"
)

func main() {
	backuphandler.EnsurePrimary()

	for {
		fmt.Println("Running elevator stuff")
		time.Sleep(2 * time.Second)
	}
}
