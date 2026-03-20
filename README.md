# Gruppe49HeisLab

Distributed elevator controller project for TTK4145 (Group 49).

## Project Structure

- `main.go`: app entrypoint and event loop
- `controller/`: local elevator state machine and IO handling
- `peers/`: peer discovery, broadcast, snapshots, order confirmation
- `hall_request_assigner/`: for interfacing with assigner script
- `backup_handler/`: crash recovery support
- `config/`: timing and system constants

## Requirements

- Go 1.25+
- `hall_request_assigner` executable available in project root

## Run

```bash
go run .
```

To test network behavior, run multiple instances on different machines on the same network.

## Credits

Elevator/network driver is based on the TTK4145 resources:
https://github.com/ttk4145
