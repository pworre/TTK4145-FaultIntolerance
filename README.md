# TTK4145 - Elevator project

Elevator project for the course TTK4145 - Real-time Programming at NTNU. This a distributed system which is designed to control multiple elevators on a network concurrently in a fault-tolerant manner. 

## Overview

|                       General information                           |
|---------------------|-----------------------------------------------|
| Course              | TTK4145 – Real-time programming               |
| Project type        | Distributed Elevator Control System           |
| Authors             | Hans Tomren, Paul Eirik Worre, Oscar Skjelvik |
| Institution         | NTNU                                          |
| Year                | 2026                                          |

---
### Implementation overview

This program uses a peer-to-peer (P2P) network topology to create redundancy in the distributed control system. Each peer is connected to its own elevator instance, and states are shared and synchronized between peers. This allows the system to perform forward error recovery and keep functioning, even in the event of any/all errors.

The network communication uses **UDP Broadcast**. Every client is listening on the same predefined port and periodically transmits messages via broadcast to this same port. This enables automatic peer discovery, and puts no bound on the number of peers that can be connected to the network.

## Dependencies

This project requires the following dependencies to be installed on the host machine:
- `elevatorserver` (for the physical elevator hardware) or [`simelevatorserver`](https://github.com/TTK4145/Simulator-v2)
 needs to be installed and in the path of the root user
- `golang` >= 1.25
- `dmd` D-lang compiler for the hall request assigner

To build and install the `hall_request_assigner` run the following command:
`./scripts/hra_install.sh`

## Cloning the repository

Since we have git submodules you need to clone the repository recursively:
`git clone --recursive link.to.repo.git`

## Deployment

To deploy the elevator you first run the elevatorserver (or simulator):

`elevatorserver --port=myport`

and then run the elevator program:

`go run main.go --id=myid --port=myport`
