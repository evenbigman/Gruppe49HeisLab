package hallrequestassigner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"sanntidslab/controller"
)

// TODO: need better names for this sctruct
type State struct {
	HallCalls [][2]bool
	Elevators []controller.Elevator
}

func elevatorStatusToString(elevatorStatus controller.ElevatorStatus) string {
	switch elevatorStatus {
	case controller.Idle:
		return "idle"
	case controller.Driving:
		return "moving"
	case controller.DoorOpen:
		return "doorOpen"
	default:
		// TODO: maybe change this to thow an error
		return "idle"
	}
}

func elevatorDirectionToString(direction controller.Direction) string {
	switch direction {
	case controller.MovingUp:
		return "up"
	case controller.MovingDown:
		return "down"
	default:
		return "stop"
	}
}

func elevatorStateToJSON(s State) ([]byte, error) {
	states := make(map[string]any, len(s.Elevators))

	for i, e := range s.Elevators {
		cabRequests := make([][2]bool, len(e.Orders))
		for floor, hasCabOrder := range e.Orders {
			cabRequests[floor] = [2]bool{hasCabOrder, false}
		}

		states[fmt.Sprintf("id_%d", i+1)] = map[string]any{
			"behaviour":   elevatorStatusToString(e.State),
			"floor":       e.CurrentFloor,
			"direction":   elevatorDirectionToString(e.Direction),
			"cabRequests": cabRequests,
		}
	}

	payload := map[string]any{
		"hallRequests": s.HallCalls,
		"states":       states,
	}

	return json.MarshalIndent(payload, "", "  ")
}

func GetElevatorAssignmentFromHallRequest(cabCallsAndElevatorStates State) ([]byte, error) {
	// TODO: catch error
	cabCallsAndElevatorStates_JSON, _ := elevatorStateToJSON(cabCallsAndElevatorStates)
	cmd := exec.Command("./hall_request_assigner", "--input", string(cabCallsAndElevatorStates_JSON))

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("hall_request_assigner failed: %w (stderr: %s)", err, stderr.String())
	}

	// Trim trailing
	out := bytes.TrimSpace(stdout.Bytes())
	return out, nil
}
