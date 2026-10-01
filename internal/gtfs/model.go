package gtfs

// Stop is one GTFS public transport stop.
type Stop struct {
	ID        string
	Name      string
	Latitude  float64
	Longitude float64
}

// Route contains the GTFS route identity and route_type.
type Route struct {
	ID        string
	ShortName string
	Type      int
}

// Trip links a timetable trip to a route.
type Trip struct {
	ID      string
	RouteID string
}

// StopTime is one ordered stop event within a trip.
type StopTime struct {
	TripID           string
	StopID           string
	ArrivalSeconds   int
	DepartureSeconds int
	Sequence         int
}

// Feed is the subset of GTFS needed to build static transit edges.
type Feed struct {
	Stops     map[string]Stop
	Routes    map[string]Route
	Trips     map[string]Trip
	StopTimes []StopTime
}
