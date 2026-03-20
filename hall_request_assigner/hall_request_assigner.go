package hallrequestassigner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"os/exec"
	"runtime"
	"sanntidslab/config"
	"sanntidslab/controller"
	"sanntidslab/peers/snapshots"
	"slices"
)

/********************************************************************************
**********************      Hall Request Assigner      **************************

This module prepares confirmed hall orders and network snapshots for the
external hall request assigner process, executes the assigner, and returns the
local elevator's hall assignment.

********************************************************************************/

type HallAssignment_t = controller.HallOrders_t
type HallAssignments_t map[string]HallAssignment_t

// Public functions

func GetAssignedHallRequests(confirmedHallOrders controller.HallOrders_t, snapshotsOnNetwork map[uint64]snapshots.Snapshot_t, myID uint64) (HallAssignment_t, error) {
	sanitizedSnapshots := sanitizeSnapshots(snapshotsOnNetwork)

	sortedSnapshots, myIndex, err := sortSnapshotsAndFindMyIndex(sanitizedSnapshots, myID)
	if err != nil {
		return HallAssignment_t{}, err
	}

	snapshotJSON, err := snapshotToJSON(confirmedHallOrders, sortedSnapshots)
	if err != nil {
		return HallAssignment_t{}, fmt.Errorf("failed to marshal elevator state: %w", err)
	}

	myAssignment, err := runAssignmentAndGetMyOrders(snapshotJSON, myIndex)
	if err != nil {
		return HallAssignment_t{}, err
	}

	return myAssignment, nil
}

// Priavte functions

func runAssignmentAndGetMyOrders(snapshotJSON []byte, myIndex int) (HallAssignment_t, error) {
	cmd, err := initAssignmentCommand(snapshotJSON)
	if err != nil {
		return HallAssignment_t{}, err
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		return HallAssignment_t{}, fmt.Errorf("hall request assignment command failed: %w (stderr: %s)", err, stderr.String())
	}

	assignmentsJSON := bytes.TrimSpace(stdout.Bytes())
	if len(assignmentsJSON) == 0 {
		return HallAssignment_t{}, fmt.Errorf("hall request assignment command returned empty output")
	}

	assignments, err := parseHallAssignments(assignmentsJSON)
	if err != nil {
		return HallAssignment_t{}, fmt.Errorf("failed to unmarshal elevator assignments: %w", err)
	}

	assignmentKey := fmt.Sprintf("id_%d", myIndex+1)
	myAssignment, ok := assignments[assignmentKey]
	if !ok {
		return HallAssignment_t{}, fmt.Errorf("missing hall assignment for %s", assignmentKey)
	}

	return myAssignment, nil
}

func initAssignmentCommand(snapshotJSON []byte) (*exec.Cmd, error) {
	operativeSystem := runtime.GOOS
	switch operativeSystem {
	case "windows":
		return exec.Command("./hall_request_assigner", "--input", string(snapshotJSON)), nil
	case "linux":
		return exec.Command("./hall_request_assigner_script", "--input", string(snapshotJSON)), nil
	default:
		return nil, fmt.Errorf("unsupported operating system for hall request assigner: %s", operativeSystem)
	}
}

func sanitizeSnapshots(snapshotsByID map[uint64]snapshots.Snapshot_t) map[uint64]snapshots.Snapshot_t {
	sanitized := maps.Clone(snapshotsByID)
	removeObstructedElevators(sanitized)
	removeImpossibleStates(sanitized)
	return sanitized
}

func removeImpossibleStates(snapshotsByID map[uint64]snapshots.Snapshot_t) {
	for id, snapshot := range snapshotsByID {
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
		snapshotsByID[id] = snapshot
	}
}

func removeObstructedElevators(snapshotsByID map[uint64]snapshots.Snapshot_t) {
	for id, snapshot := range snapshotsByID {
		if snapshot.Elevator.State == controller.Obstructed {
			delete(snapshotsByID, id)
		}
	}
}

func sortSnapshotsAndFindMyIndex(snapshotsByID map[uint64]snapshots.Snapshot_t, myID uint64) ([]snapshots.Snapshot_t, int, error) {
	ids := make([]uint64, 0, len(snapshotsByID))
	for id := range snapshotsByID {
		ids = append(ids, id)
	}

	slices.Sort(ids)

	myIndex, found := slices.BinarySearch(ids, myID)
	if !found {
		return nil, -1, fmt.Errorf("could not find my id %d in sorted ids", myID)
	}

	sortedSnapshots := make([]snapshots.Snapshot_t, len(ids))
	for i, id := range ids {
		sortedSnapshots[i] = snapshotsByID[id]
	}

	return sortedSnapshots, myIndex, nil
}

func statusToString(status controller.ElevatorState_t) (string, error) {
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

func directionToString(state controller.ElevatorState_t) (string, error) {
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

func snapshotToJSON(hallCalls controller.HallOrders_t, snapshotsList []snapshots.Snapshot_t) ([]byte, error) {
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

func parseHallAssignments(assignmentsJSON []byte) (HallAssignments_t, error) {
	var assignments HallAssignments_t

	err := json.Unmarshal(assignmentsJSON, &assignments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse hall assignments: %w", err)
	}

	return assignments, nil
}
