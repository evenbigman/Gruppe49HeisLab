package backuphandler

import (
	"flag"
	"fmt"
	"time"
)

func EnsurePrimary() {
	isBackupPtr := flag.Bool("backup", false, "run in backup mode")
	primaryPidPtr := flag.Int("pid", 0, "pid of the primary to watch")
	flag.Parse()
	isBackup := *isBackupPtr
	primaryPid := *primaryPidPtr

	if isBackup {
		waitForPrimaryToDie(primaryPid)
		fmt.Println("primary died, promoting to primary")
	}

	go watchBackupAndRespawn()
}

func watchBackupAndRespawn() {
	for {
		cmd, err := createBackup()
		if err != nil {
			fmt.Printf("error spawning backup: %v\n", err)
			time.Sleep(time.Second)
			continue
		}
		cmd.Wait()
		fmt.Println("backup died, respawning...")
	}
}

func waitForPrimaryToDie(pid int) {
	ticker := time.NewTicker(time.Millisecond * 500)
	defer ticker.Stop()

	for range ticker.C {
		if !isProcessAlive(pid) {
			return
		}
	}
}
