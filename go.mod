module elevator_project

require elevatorDriver v0.0.0
replace elevatorDriver => ./elevatorDriver

require elevatorControl v0.0.0
replace elevatorControl => ./elevatorControl

require networkDriver v0.0.0
replace networkDriver => ./networkDriver

go 1.25.5
