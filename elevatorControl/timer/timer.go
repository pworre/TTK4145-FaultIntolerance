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

func Timers(stopInactivityTimer chan bool, resetDoorTimer chan bool, doorTimeout chan bool) {
	doorTimer, inactivityTimer := initTimers()
	for {
		select {
		case <-stopInactivityTimer:
			inactivityTimer.Stop()
		case <-resetDoorTimer:
			doorTimer.Reset(DOOR_OPEN_DURATION)
		case <-doorTimer.C:
			doorTimeout <- true
		}
	}
}
