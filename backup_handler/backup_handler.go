package backuphandler

import (
	"flag"
	"fmt"
	"time"
)

// Public funcitons

func InitPrimaryBackup() {
	waitUntilPrimary()
	go keepBackupRunning()
}

// Private funcitons

func waitUntilPrimary() {
	isBackup, primaryPID := parseRoleFlags()

	if isBackup {
		waitForPrimaryToDie(primaryPID)
	}

	fmt.Printf("running as primary...")
}

func keepBackupRunning() {
	for {
		cmd, err := createBackup()
		if err != nil {
			fmt.Printf("error spawning backup: %v\n", err)
			time.Sleep(time.Second)
			continue
		}

		err = cmd.Wait()
		if err != nil {
			fmt.Printf("backup exited with error: %v\n", err)
		}

		fmt.Println("backup died, respawning...")
	}
}

func parseRoleFlags() (bool, int) {
	backupModeFlag := flag.Bool("backup", false, "run in backup mode")
	primaryPIDFlag := flag.Int("pid", 0, "pid of the primary to watch")
	flag.Parse()

	return *backupModeFlag, *primaryPIDFlag
}

func waitForPrimaryToDie(primaryPID int) {
	ticker := time.NewTicker(time.Millisecond * 500)
	defer ticker.Stop()

	for range ticker.C {
		if !isProcessAlive(primaryPID) {
			return
		}
	}
}
