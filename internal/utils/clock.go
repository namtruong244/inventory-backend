package utils

import (
	"time"
	_ "time/tzdata" // Lambda images have no zoneinfo; embed it so LoadLocation works
)

const DateLayout = "2006-01-02"

// LocationOrUTC resolves an IANA timezone name, falling back to UTC.
func LocationOrUTC(tz string) *time.Location {
	if tz == "" {
		return time.UTC
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return time.UTC
	}
	return loc
}

// TodayIn returns today's calendar date (YYYY-MM-DD) in the given timezone.
func TodayIn(tz string) string {
	return time.Now().In(LocationOrUTC(tz)).Format(DateLayout)
}

// DateAfter returns the calendar date `days` days from today in the given timezone.
func DateAfter(tz string, days int) string {
	return time.Now().In(LocationOrUTC(tz)).AddDate(0, 0, days).Format(DateLayout)
}
