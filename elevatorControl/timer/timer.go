package timer

import "time"

const DOOR_OPEN_DURATION = 3 * time.Second
const INACTIVITY_TIMEOUT = 9 * time.Second
const MOTOR_STALL_TIMEOUT = 3 * time.Second

func initTimers() (*time.Timer, *time.Timer, *time.Timer) {
	doorTimer := time.NewTimer(0 * time.Second)
	inactivityTimer := time.NewTimer(0 * time.Second)
	motorStallTimer := time.NewTimer(0 * time.Second)
	<-doorTimer.C
	<-inactivityTimer.C
	<-motorStallTimer.C
	return doorTimer, inactivityTimer, motorStallTimer
}

func Timers(resetDoorTimer chan bool, resetInactivityTimer chan bool, resetMotorStallTimer chan bool, doorTimeout chan bool, inactivityTimeout chan bool, motorStallTimeout chan bool) {
	doorTimer, inactivityTimer, motorStallTimer := initTimers()
	for {
		select {
		case <-resetDoorTimer:
			safeReset(doorTimer, DOOR_OPEN_DURATION)
		case <-resetInactivityTimer:
			safeReset(inactivityTimer, INACTIVITY_TIMEOUT)
		case <-resetMotorStallTimer:
			safeReset(motorStallTimer, MOTOR_STALL_TIMEOUT)
		case <-doorTimer.C:
			doorTimeout <- true
		case <-inactivityTimer.C:
			inactivityTimeout <- true
		case <-motorStallTimer.C:
			motorStallTimeout <- true
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
