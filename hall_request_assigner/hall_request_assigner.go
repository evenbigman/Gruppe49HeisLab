package hallrequestassigner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"sanntidslab/controller"
)

type ElevatorSnapshot struct {
	HallCalls [][2]bool
	Elevators []controller.Elevator
}

type HallAssignments map[string][][2]bool

// Public functions

func AssignHallRequests(snapshot ElevatorSnapshot) (HallAssignments, error) {
	snapshotJSON, err := snapshotToJSON(snapshot)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal elevator state: %w", err)
	}

	cmd := exec.Command("./hall_request_assigner", "--input", string(snapshotJSON))

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
	case controller.MovingUp:
		return "up", nil
	case controller.MovingDown:
		return "down", nil
	case controller.Idle, controller.DoorOpenIdle:
		return "stop", nil
	default:
		return "", fmt.Errorf("unknown elevator moving state: %d", state)
	}
}

func snapshotToJSON(snapshot ElevatorSnapshot) ([]byte, error) {
	states := make(map[string]any, len(snapshot.Elevators))

	for id, elevator := range snapshot.Elevators {
		status, err := statusToString(elevator.State)
		if err != nil {
			return nil, fmt.Errorf("failed to convert elevator status to string: %w", err)
		}

		direction, err := directionToString(elevator.State)
		if err != nil {
			return nil, fmt.Errorf("failed to convert elevator direction to string: %w", err)
		}

		states[fmt.Sprintf("id_%d", id+1)] = map[string]any{
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
