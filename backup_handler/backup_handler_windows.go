//go:build windows

package backuphandler

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

func createBackup() (*exec.Cmd, error) {
	myPid := os.Getpid()

	executablePath, err := os.Executable()
	if err != nil {
		return nil, err
	}

	const createNewConsole = 0x00000010
	cmd := exec.Command(executablePath, "-backup=true", "-pid="+strconv.Itoa(myPid))
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: createNewConsole,
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Start()
	return cmd, err
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
	const stillActive = 259
	return exitCode == stillActive
}