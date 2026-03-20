package hallrequestassigner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"sanntidslab/config"
	"sanntidslab/controller"
	"sanntidslab/peers/snapshots"
)

type ElevatorsSnapshot struct {
	HallCalls [config.NumFloors][2]bool
	Snapshot  []snapshots.Snapshot_t
}

type HallAssignments map[string][config.NumFloors][2]bool

// Public functions

func AssignHallRequests(snapshot ElevatorsSnapshot) (HallAssignments, error) {
	for i := range snapshot.Snapshot {
		elevator := snapshot.Snapshot[i].Elevator

		atBottomFloor := elevator.CurrentFloor == 0
		atTopFloor := elevator.CurrentFloor == config.MaxFloor

		if atBottomFloor && (elevator.State == controller.MovingDown || elevator.State == controller.DoorOpenHeadingDown) {
			elevator.State = controller.Idle
		}

		if atTopFloor && (elevator.State == controller.MovingUp || elevator.State == controller.DoorOpenHeadingUp) {
			elevator.State = controller.Idle
		}

		snapshot.Snapshot[i].Elevator = elevator
	}

	snapshotJSON, err := snapshotToJSON(snapshot)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal elevator state: %w", err)
	}

	cmd := exec.Command("./hall_request_assigner_script", "--input", string(snapshotJSON))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("hall request assignment command failed: %w (stderr: %s)", err, stderr.String())
	}

	assignmentsJSON := bytes.TrimSpace(stdout.Bytes())

	assignments, err := parseHallAssignments(assignmentsJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal elevator assignments: %w", err)
	}

	return assignments, nil
}

// Private functions

func statusToString(status controller.ElevatorState) (string, error) {
	switch status {
	case controller.Idle:
		return "idle", nil
	case controller.MovingUp, controller.MovingDown:
		return "moving", nil
	case controller.DoorOpenHeadingUp, controller.DoorOpenHeadingDown, controller.DoorOpenIdle:
		return "doorOpen", nil
	default:
		return "", fmt.Errorf("unknown elevator status: %d", status)
	}
}

func directionToString(state controller.ElevatorState) (string, error) {
	switch state {
	case controller.MovingUp, controller.DoorOpenHeadingUp:
		return "up", nil
	case controller.MovingDown, controller.DoorOpenHeadingDown:
		return "down", nil
	case controller.Idle, controller.DoorOpenIdle:
		return "stop", nil
	default:
		return "", fmt.Errorf("unknown elevator moving state: %d", state)
	}
}

func snapshotToJSON(snapshot ElevatorsSnapshot) ([]byte, error) {
	states := make(map[string]any, len(snapshot.Snapshot))

	for i, snap := range snapshot.Snapshot {
		elevator := snap.Elevator
		status, err := statusToString(elevator.State)
		if err != nil {
			return nil, fmt.Errorf("failed to convert elevator status to string: %w", err)
		}

		direction, err := directionToString(elevator.State)
		if err != nil {
			return nil, fmt.Errorf("failed to convert elevator direction to string: %w", err)
		}

		states[fmt.Sprintf("id_%d", i+1)] = map[string]any{
			"behaviour":   status,
			"floor":       elevator.CurrentFloor,
			"direction":   direction,
			"cabRequests": elevator.CabOrders,
		}
	}

	payload := map[string]any{
		"hallRequests": snapshot.HallCalls,
		"states":       states,
	}

	return json.MarshalIndent(payload, "", "  ")
}

func parseHallAssignments(assignmentsJSON []byte) (HallAssignments, error) {
	var assignments HallAssignments

	err := json.Unmarshal(assignmentsJSON, &assignments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse hall assignments: %w", err)
	}

	return assignments, nil
}
