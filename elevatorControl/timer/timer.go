package timer

import "time"

const DOOR_OPEN_DURATION = 3 * time.Second
const INACTIVITY_TIMEOUT = 9 * time.Second
const OBSTRUCTION_TIMEOUT = 5 * time.Second

func initTimers() (*time.Timer, *time.Timer, *time.Timer) {
	doorTimer := time.NewTimer(0)
	inactivityTimer := time.NewTimer(0)
	obstructionTimer := time.NewTimer(0)
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
			safeReset(obstructionTimer, OBSTRUCTION_TIMEOUT)
		case <-resetInactivityTimer:
			safeReset(inactivityTimer, INACTIVITY_TIMEOUT)
		case <-resetDoorTimer:
			safeReset(doorTimer, DOOR_OPEN_DURATION)
		case <-doorTimer.C:
			doorTimeout <- true
		case <-inactivityTimer.C:
			inactivityTimeout <- true
		case <-obstructionTimer.C:
			obstructionTimeout <- true
		}
	}
}

func safeReset(timer *time.Timer, duration time.Duration) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(duration)
}
