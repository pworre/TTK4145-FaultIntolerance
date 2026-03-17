package config

import (
	"flag"
	"fmt"
	"os"
)

// CONTENT: This file contains config for arguments in terminal when running the program.
// 			Both ID and Port must be set in order for an elevator to work on the network.
//			This must be unique and set manually at each PC.

type Config struct {
	ID   string
	Port int
}

const STANDARD_PORT int = 15657

func ParseFlag() Config {
	id := flag.String("id", "-1", "Elevator ID")
	port := flag.Int("port", -1, "UDP Port Number")

	flag.Parse()

	if *id == "-1" {
		fmt.Println("ERROR: You must provide both --id")
		flag.Usage()
		os.Exit(1)
	}

	if *port == -1 {
		*port = STANDARD_PORT
	}

	return Config{
		ID:   *id,
		Port: *port,
	}
}
