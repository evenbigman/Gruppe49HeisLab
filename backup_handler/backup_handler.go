package backuphandler

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

// TODO: make linux compatible

func EnsurePrimary() {
	isBackupPtr := flag.Bool("backup", false, "run in backup mode")
	primaryPidPtr := flag.Int("pid", 0, "pid of the primary to watch")
	flag.Parse()
	isBackup := *isBackupPtr
	primaryPid := *primaryPidPtr

	if isBackup {
		waitForPrimaryToDie(primaryPid)
	}

	err := createBackup()
	if err != nil {
		fmt.Printf("error spawning backup: %v\n", err)
	}
}

func createBackup() error {
	myPid := os.Getpid()

	executablePath, err := os.Executable()
	if err != nil {
		return err
	}

	cmd := exec.Command("cmd", "/C", "start", "Backup Monitor", "cmd", "/K", executablePath, "-backup=true", "-pid="+strconv.Itoa(myPid))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Start()
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

func isProcessAlive(pid int) bool {
	handle, err := syscall.OpenProcess(syscall.SYNCHRONIZE|syscall.PROCESS_QUERY_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(handle)

	var exitCode uint32
	err = syscall.GetExitCodeProcess(handle, &exitCode)
	if err != nil {
		return false
	}
	const stillActive = 259 // STILL_ACTIVE
	return exitCode == stillActive
}
