package hallrequestassigner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"runtime"
	"sanntidslab/config"
	"sanntidslab/controller"
	"sanntidslab/peers/snapshots"
	"slices"
)

type order [2]bool
type HallAssignment [config.NumFloors]order
type HallAssignments map[string]HallAssignment

// Public functions

func GetAssignedHallRequests(confirmedHallCalls [config.NumFloors][2]bool, SnapshotsOnNetwork map[uint64]snapshots.Snapshot, myID uint64) (HallAssignment, error) {
	removeObstructedElevators(&SnapshotsOnNetwork)
	removeImpossibleStates(&SnapshotsOnNetwork)

	sortedSnapshots, myIndex, err := sortSnapshotsAndFindMyIndex(SnapshotsOnNetwork, myID)
	if err != nil {
		return HallAssignment{}, err
	}

	snapshotJSON, err := snapshotToJSON(confirmedHallCalls, sortedSnapshots)
	if err != nil {
		return HallAssignment{}, fmt.Errorf("failed to marshal elevator state: %w", err)
	}

	myAssignment, err := runAssignmentAndGetMyOrders(snapshotJSON, myIndex)
	if err != nil {
		return HallAssignment{}, err
	}

	return myAssignment, nil
}

// Priavte functions

func runAssignmentAndGetMyOrders(snapshotJSON []byte, myIndex int) (HallAssignment, error) {
	cmd := initAssignmentCommand(snapshotJSON)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return HallAssignment{}, fmt.Errorf("hall request assignment command failed: %w (stderr: %s)", err, stderr.String())
	}

	assignmentsJSON := bytes.TrimSpace(stdout.Bytes())
	assignments, err := parseHallAssignments(assignmentsJSON)
	if err != nil {
		return HallAssignment{}, fmt.Errorf("failed to unmarshal elevator assignments: %w", err)
	}

	assignmentKey := fmt.Sprintf("id_%d", myIndex+1)
	myAssignment, ok := assignments[assignmentKey]
	if !ok {
		return HallAssignment{}, fmt.Errorf("missing hall assignment for %s", assignmentKey)
	}

	return myAssignment, nil
}

func initAssignmentCommand(snapshotJSON []byte) *exec.Cmd {
	operativeSystem := runtime.GOOS
	var cmd *exec.Cmd
	switch operativeSystem {
	case "windows":
		cmd = exec.Command("./hall_request_assigner", "--input", string(snapshotJSON))
	case "linux":
		cmd = exec.Command("./hall_request_assigner_script", "--input", string(snapshotJSON))
	}
	return cmd
}

func removeImpossibleStates(snapshotsByID *map[uint64]snapshots.Snapshot) {
	for id, snapshot := range *snapshotsByID {
		elevator := snapshot.Elevator

		atBottomFloor := elevator.CurrentFloor == 0
		atTopFloor := elevator.CurrentFloor == config.MaxFloor

		if atBottomFloor && (elevator.State == controller.MovingDown || elevator.State == controller.DoorOpenHeadingDown) {
			elevator.State = controller.Idle
		}

		if atTopFloor && (elevator.State == controller.MovingUp || elevator.State == controller.DoorOpenHeadingUp) {
			elevator.State = controller.Idle
		}

		snapshot.Elevator = elevator
		(*snapshotsByID)[id] = snapshot
	}
}

func removeObstructedElevators(snapshotsByID *map[uint64]snapshots.Snapshot) {
	for id, snapshot := range *snapshotsByID {
		if snapshot.Elevator.State == controller.Obstructed {
			delete(*snapshotsByID, id)
		}
	}
}

func sortSnapshotsAndFindMyIndex(snapshotsByID map[uint64]snapshots.Snapshot, myID uint64) ([]snapshots.Snapshot, int, error) {
	ids := make([]uint64, 0, len(snapshotsByID))
	for id := range snapshotsByID {
		ids = append(ids, id)
	}

	slices.Sort(ids)

	myIndex, found := slices.BinarySearch(ids, myID)
	if !found {
		return nil, -1, fmt.Errorf("could not find my id %d in sorted ids", myID)
	}

	sortedSnapshots := make([]snapshots.Snapshot, len(ids))
	for i, id := range ids {
		sortedSnapshots[i] = snapshotsByID[id]
	}

	return sortedSnapshots, myIndex, nil
}

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

func snapshotToJSON(hallCalls [config.NumFloors][2]bool, snapshotsList []snapshots.Snapshot) ([]byte, error) {
	states := make(map[string]any, len(snapshotsList))

	for i, snap := range snapshotsList {
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
		"hallRequests": hallCalls,
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
