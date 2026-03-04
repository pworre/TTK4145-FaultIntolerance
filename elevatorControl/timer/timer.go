package timer

import "time"

const DOOR_OPEN_DURATION = 3 * time.Second
const INACTIVITY_TIMEOUT = 9 * time.Second
const OBSTRUCTION_TIMEOUT = 5 * time.Second

func initTimers() (*time.Timer, *time.Timer, *time.Timer) {
	doorTimer := time.NewTimer(0 * time.Second)
	inactivityTimer := time.NewTimer(0 * time.Second)
	obstructionTimer := time.NewTimer(0 * time.Second)
	<-doorTimer.C
	<-inactivityTimer.C
	<-obstructionTimer.C
	return doorTimer, inactivityTimer, obstructionTimer
}

func Timers(resetObstructionTimer chan bool, resetInactivityTimer chan bool, resetDoorTimer chan bool, doorTimeout chan bool, inactivityTimeout chan bool, obstructionTimeout chan bool) {
	doorTimer, inactivityTimer, obstructionTimer := initTimers()
	for {
		select {
		case <-resetObstructionTimer:
			obstructionTimer.Reset(OBSTRUCTION_TIMEOUT)
		case <-resetInactivityTimer:
			inactivityTimer.Reset(INACTIVITY_TIMEOUT)
		case <-resetDoorTimer:
			doorTimer.Reset(DOOR_OPEN_DURATION)
		case <-doorTimer.C:
			doorTimeout <- true
		case <-inactivityTimer.C:
			inactivityTimeout <- true
		case <-obstructionTimer.C:
			obstructionTimeout <- true
		}
	}
}
