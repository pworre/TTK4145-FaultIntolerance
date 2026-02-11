module elevator_project

require elevatorDriver v0.0.0
replace elevatorDriver => ./elevatorDriver

require elevatorControl v0.0.0
replace elevatorControl => ./elevatorControl

require networkDriver v0.0.0
replace networkDriver => ./networkDriver

require order v0.0.0
replace order => ./order

go 1.25.5
