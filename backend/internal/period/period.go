// Package period turns the calendar dates a client sends (from/to, YYYY-MM-DD, both
// inclusive) into the half-open instant range [From, To) in the business time zone.
package period

import (
	"time"

	"github.com/andreccls/go-react-business-suite/backend/internal/validation"
)

// MaxDays bounds a range, so a request cannot ask for an unbounded scan.
const MaxDays = 366

// Range is a validated period. From/To are instants (To is exclusive: midnight after
// the last day); FromDay/ToDay echo the inclusive dates. A bound that was not given
// (and has no default) is the zero value.
type Range struct {
	From, To       time.Time
	FromDay, ToDay string
}

// Parse validates from/to. With defaultDays > 0 a missing `to` means today and a
// missing `from` means defaultDays-1 days before `to`; with 0, each bound is optional.
func Parse(from, to string, loc *time.Location, now time.Time, defaultDays int) (Range, error) {
	var v validation.Errors
	day := func(field, s string) (time.Time, bool) {
		t, err := time.ParseInLocation(time.DateOnly, s, loc)
		if err != nil {
			v.Add(field, "must be a date as YYYY-MM-DD")
		}
		return t, err == nil
	}

	var fromD, toD time.Time
	var hasFrom, hasTo bool
	switch {
	case to != "":
		toD, hasTo = day("to", to)
	case defaultDays > 0:
		y, m, d := now.In(loc).Date()
		toD, hasTo = time.Date(y, m, d, 0, 0, 0, 0, loc), true
	}
	switch {
	case from != "":
		fromD, hasFrom = day("from", from)
	case defaultDays > 0 && hasTo:
		fromD, hasFrom = toD.AddDate(0, 0, -(defaultDays-1)), true
	}
	if hasFrom && hasTo {
		switch {
		case fromD.After(toD):
			v.Add("from", "must not be after to")
		case fromD.AddDate(0, 0, MaxDays-1).Before(toD): // MaxDays counted inclusively
			v.Add("to", "the range must not exceed 366 days")
		}
	}
	if err := v.Err(); err != nil {
		return Range{}, err
	}
	var r Range
	if hasFrom {
		r.From, r.FromDay = fromD, fromD.Format(time.DateOnly)
	}
	if hasTo {
		r.To, r.ToDay = toD.AddDate(0, 0, 1), toD.Format(time.DateOnly)
	}
	return r, nil
}
