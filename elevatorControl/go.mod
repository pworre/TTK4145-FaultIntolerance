module elevatorControl

require elevatorDriver v0.0.0
replace elevatorDriver => ../elevatorDriver

require syncOrders v0.0.0
replace syncOrders => ../syncOrders

require config v0.0.0
replace config => ../config

require networkDriver v0.0.0
replace networkDriver => ../networkDriver

go 1.25.5
