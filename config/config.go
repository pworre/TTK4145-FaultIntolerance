package config

// - - - - - - Overview - - - - - - - - -

// This file contains configuration for command line arguments.
// An ID must be set in order for an elevator to join the network. This must be unique and set manually at each PC.
// The port argument is the TCP port used to connect to the elevatorserver, and has a default option.
// The backup arguments signals that the process should be started as a backup in a process-pairs configuration,
// with default being false.

import (
	"flag"
	"fmt"
	"os"
)

type Config struct {
	ID     string
	Port   int
	Backup bool
}

const STANDARD_PORT int = 15657

func ParseFlag() Config {
	id := flag.String("id", "-1", "Elevator ID")
	port := flag.Int("port", -1, "Simulator TCP port number")
	backup := flag.Bool("backup", false, "Start process as passive backup")

	flag.Parse()

	if *id == "-1" {
		fmt.Println("ERROR: You must provide --id")
		flag.Usage()
		os.Exit(1)
	}

	if *port == -1 {
		*port = STANDARD_PORT
	}

	return Config{
		ID:     *id,
		Port:   *port,
		Backup: *backup,
	}
}
