//go:build linux

package backuphandler

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

func createBackup() (*exec.Cmd, error) {
	myPid := os.Getpid()

	executablePath, err := executablePathForRespawn()
	if err != nil {
		return nil, err
	}

	backupArgs := []string{"-backup=true", "-pid=" + strconv.Itoa(myPid)}

	if isWSL() {
		cmd := exec.Command("cmd.exe", "/c", "start", "", "wsl.exe", executablePath, backupArgs[0], backupArgs[1])
		err = cmd.Start()
		return cmd, err
	}

	cmd, err := buildLinuxTerminalCommand(executablePath, backupArgs)
	if err != nil {
		return nil, err
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	err = cmd.Start()
	return cmd, err
}

func executablePathForRespawn() (string, error) {
	path, err := os.Executable()
	if err == nil {
		if _, statErr := os.Stat(path); statErr == nil {
			return path, nil
		}
	}

	if _, statErr := os.Stat("/proc/self/exe"); statErr == nil {
		return "/proc/self/exe", nil
	}

	if err != nil {
		return "", err
	}
	return "", fmt.Errorf("resolved executable path no longer exists: %s", path)
}

func buildLinuxTerminalCommand(executablePath string, backupArgs []string) (*exec.Cmd, error) {
	terminalCandidates := []struct {
		binary string
		args   []string
	}{
		{binary: "gnome-terminal", args: []string{"--wait", "--", executablePath, backupArgs[0], backupArgs[1]}},
		{binary: "konsole", args: []string{"-e", executablePath, backupArgs[0], backupArgs[1]}},
		{binary: "xfce4-terminal", args: []string{"-e", strings.Join(append([]string{executablePath}, backupArgs...), " ")}},
		{binary: "xterm", args: []string{"-e", executablePath, backupArgs[0], backupArgs[1]}},
		{binary: "kitty", args: []string{executablePath, backupArgs[0], backupArgs[1]}},
		{binary: "alacritty", args: []string{"-e", executablePath, backupArgs[0], backupArgs[1]}},
		{binary: "wezterm", args: []string{"start", "--always-new-process", executablePath, backupArgs[0], backupArgs[1]}},
		{binary: "x-terminal-emulator", args: []string{"-e", executablePath, backupArgs[0], backupArgs[1]}},
	}

	for _, candidate := range terminalCandidates {
		if _, err := exec.LookPath(candidate.binary); err == nil {
			return exec.Command(candidate.binary, candidate.args...), nil
		}
	}

	return nil, fmt.Errorf("no supported Linux terminal emulator found")
}

func isWSL() bool {
	if os.Getenv("WSL_DISTRO_NAME") != "" || os.Getenv("WSL_INTEROP") != "" {
		return true
	}

	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return false
	}

	return strings.Contains(strings.ToLower(string(data)), "microsoft")
}

func isProcessAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	err = process.Signal(syscall.Signal(0))
	return err == nil
}
