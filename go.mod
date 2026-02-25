module elevator_project

require elevatorDriver v0.0.0
replace elevatorDriver => ./elevatorDriver

require elevatorControl v0.0.0
replace elevatorControl => ./elevatorControl

require networkDriver v0.0.0
replace networkDriver => ./networkDriver

require syncOrders v0.0.0
replace syncOrders => ./syncOrders

require config v0.0.0
replace config => ./config

go 1.25.5
