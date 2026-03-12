package timer

import "time"

const DOOR_OPEN_DURATION = 3 * time.Second
const INACTIVITY_TIMEOUT = 9 * time.Second

func initTimers() (*time.Timer, *time.Timer) {
	doorTimer := time.NewTimer(0 * time.Second)
	inactivityTimer := time.NewTimer(0 * time.Second)
	<-doorTimer.C
	<-inactivityTimer.C
	return doorTimer, inactivityTimer
}

func Timers(resetDoorTimer chan bool, resetInactivityTimer chan bool, doorTimeout chan bool, inactivityTimeout chan bool) {
	doorTimer, inactivityTimer := initTimers()
	for {
		select {
		case <-resetDoorTimer:
			safeReset(doorTimer, DOOR_OPEN_DURATION)
		case <-resetInactivityTimer:
			safeReset(inactivityTimer, INACTIVITY_TIMEOUT)
		case <-doorTimer.C:
			doorTimeout <- true
		case <-inactivityTimer.C:
			inactivityTimeout <- true
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
