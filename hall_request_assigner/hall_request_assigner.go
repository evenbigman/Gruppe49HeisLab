package hallrequestassigner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"sanntidslab/controller"
)

type HallCallsAndElevatorStates struct {
	HallCalls [][2]bool
	Elevators []controller.Elevator
}

type ElevatorHallAssignments map[string][][2]bool

func elevatorStatusToString(elevatorStatus controller.ElevatorState) (string, error) {
	switch elevatorStatus {
	case controller.Idle:
		return "idle", nil
	case controller.MovingUp, controller.MovingDown:
		return "moving", nil
	case controller.DoorOpenHeadingUp, controller.DoorOpenHeadingDown, controller.DoorOpenIdle:
		return "doorOpen", nil
	default:
		return "", fmt.Errorf("unknown elevator status: %d", elevatorStatus)
	}
}

func elevatorMovingStateToString(movingState controller.ElevatorState) (string, error) {
	switch movingState {
	case controller.MovingUp:
		return "up", nil
	case controller.MovingDown:
		return "down", nil
	case controller.Idle, controller.DoorOpenIdle:
		return "stop", nil
	default:
		return "", fmt.Errorf("unknown elevator moving state: %d", movingState)
	}
}

func elevatorStateToJSON(cabCallsAndElevatorStates HallCallsAndElevatorStates) ([]byte, error) {
	elevatorStates := make(map[string]any, len(cabCallsAndElevatorStates.Elevators))

	for i, elevator := range cabCallsAndElevatorStates.Elevators {
		status, err := elevatorStatusToString(elevator.State)
		if err != nil {
			return nil, fmt.Errorf("failed to convert elevator status to string: %w", err)
		}

		direction, err := elevatorMovingStateToString(elevator.State)
		if err != nil {
			return nil, fmt.Errorf("failed to convert elevator direction to string: %w", err)
		}

		elevatorStates[fmt.Sprintf("id_%d", i+1)] = map[string]any{
			"behaviour":   status,
			"floor":       elevator.CurrentFloor,
			"direction":   direction,
			"cabRequests": elevator.CabOrders,
		}
	}

	elevatorAssignmentPayload := map[string]any{
		"hallRequests": cabCallsAndElevatorStates.HallCalls,
		"states":       elevatorStates,
	}

	return json.MarshalIndent(elevatorAssignmentPayload, "", "  ")
}

func elevatorAssignmentJSONToMap(jsonData []byte) (ElevatorHallAssignments, error) {
	var assignments ElevatorHallAssignments

	err := json.Unmarshal(jsonData, &assignments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse hall assignments: %w", err)
	}

	return assignments, nil
}

func GetHallAssignmentFromHallRequestAndElevatorStates(cabCallsAndElevatorStates HallCallsAndElevatorStates) (ElevatorHallAssignments, error) {
	cabCallsAndElevatorStates_JSON, err := elevatorStateToJSON(cabCallsAndElevatorStates)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal elevator state: %w", err)
	}

	cmd := exec.Command("./hall_request_assigner", "--input", string(cabCallsAndElevatorStates_JSON))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("hall request assignment command failed: %w (stderr: %s)", err, stderr.String())
	}

	out := bytes.TrimSpace(stdout.Bytes())

	ElevatorAssignment, err := elevatorAssignmentJSONToMap(out)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal elevator assignments: %w", err)
	}

	return ElevatorAssignment, nil
}
