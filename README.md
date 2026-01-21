# TTK4145 - Sanntidsprogrammering
## Heislabb
### Project Information

| Category            | Description                                   |
|---------------------|-----------------------------------------------|
| Course              | TTK4145 – Real-time programming               |
| Project             | Distributed Elevator Control System           |
| Architecture        | Peer-to-Peer (P2P)                            |
| Programming Language| Go (Golang)                                   |
| Communication       | UDP Broadcast                                 |
| Network Port        | 34933 (UDP)                                   |
| Number of Elevators | 1–n (scalable design)                         |
| Floors              | 4                                             |
| Fault Tolerance     | Application-level (timeouts and state sharing)|
| Authors             | Hans Tomren, Paul Eirik Worre, Oscar Skjelvik |
| Institution         | NTNU                                          |
| Year                | 2026                                          |

---
### Network Topology
For this project, we are using peer-to-peers (P2P) by having all elevators as equal nodes, sharing their orders and state between eachother. This design removes single points of failure and improves fault tolerance. 

The communication is based on **UDP Broadcast**. Every client is listening on the same predefined port (e.g. `34933`) and periodically transmits messages to the broadcast-adress `255.255.255.255:34933`. This broadcast-method enables a automatic peer discovery and makes it simpler to join or rejoin the network.  
