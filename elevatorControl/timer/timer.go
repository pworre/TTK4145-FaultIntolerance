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

func Timers(resetDoorTimer chan bool, resetInactivityTimer chan bool, resetMotorStallTimer chan bool, startMotorStallTimer chan bool, stopMotorStallTimeout chan bool, doorTimeout chan bool, inactivityTimeout chan bool, motorStallTimeout chan bool) {
	doorTimer, inactivityTimer, motorStallTimer := initTimers()
	motorStallRunning := false

	for {
		select {
		case <-resetDoorTimer:
			safeReset(doorTimer, DOOR_OPEN_DURATION)

		case <-resetInactivityTimer:
			safeReset(inactivityTimer, INACTIVITY_TIMEOUT)

		case <-startMotorStallTimer:
			motorStallRunning = true
			safeReset(motorStallTimer, MOTOR_STALL_TIMEOUT)

		case <-stopMotorStallTimeout:
			motorStallRunning = false
			safeStop(motorStallTimer)

		case <-doorTimer.C:
			doorTimeout <- true

		case <-inactivityTimer.C:
			inactivityTimeout <- true

		case <-motorStallTimer.C:
			if motorStallRunning {
				motorStallTimeout <- true
				motorStallRunning = false
			}

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

func safeStop(timer *time.Timer) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}
