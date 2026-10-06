package period

import (
	"strings"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	sp, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatal(err)
	}
	// 2026-10-06 01:00 UTC is still 2026-10-05 22:00 in São Paulo: "today" follows the zone.
	now := time.Date(2026, 10, 6, 1, 0, 0, 0, time.UTC)

	tests := []struct {
		name, from, to string
		defDays        int
		wantFrom       string // "" = zero bound
		wantTo         string
		wantFromInst   string // RFC3339 UTC
		wantToInst     string
		badFields      []string
	}{
		{name: "explicit", from: "2026-10-01", to: "2026-10-31", defDays: 30, wantFrom: "2026-10-01", wantTo: "2026-10-31",
			wantFromInst: "2026-10-01T03:00:00Z", wantToInst: "2026-11-01T03:00:00Z"},
		{name: "defaults follow the business zone", defDays: 30, wantFrom: "2026-09-06", wantTo: "2026-10-05",
			wantFromInst: "2026-09-06T03:00:00Z", wantToInst: "2026-10-06T03:00:00Z"},
		{name: "only to: from defaults", to: "2026-10-10", defDays: 7, wantFrom: "2026-10-04", wantTo: "2026-10-10",
			wantFromInst: "2026-10-04T03:00:00Z", wantToInst: "2026-10-11T03:00:00Z"},
		{name: "only from: to defaults to today", from: "2026-10-01", defDays: 30, wantFrom: "2026-10-01", wantTo: "2026-10-05",
			wantFromInst: "2026-10-01T03:00:00Z", wantToInst: "2026-10-06T03:00:00Z"},
		{name: "optional bounds: none", defDays: 0},
		{name: "optional bounds: only from", from: "2026-10-01", wantFrom: "2026-10-01", wantFromInst: "2026-10-01T03:00:00Z"},
		{name: "optional bounds: only to", to: "2026-10-01", wantTo: "2026-10-01", wantToInst: "2026-10-02T03:00:00Z"},
		{name: "single day", from: "2026-10-01", to: "2026-10-01", wantFrom: "2026-10-01", wantTo: "2026-10-01",
			wantFromInst: "2026-10-01T03:00:00Z", wantToInst: "2026-10-02T03:00:00Z"},
		{name: "max span (366 days, inclusive)", from: "2026-01-01", to: "2027-01-01", defDays: 30, wantFrom: "2026-01-01", wantTo: "2027-01-01",
			wantFromInst: "2026-01-01T03:00:00Z", wantToInst: "2027-01-02T03:00:00Z"},
		{name: "span too long", from: "2026-01-01", to: "2027-01-02", defDays: 30, badFields: []string{"to"}},
		{name: "from after to", from: "2026-10-02", to: "2026-10-01", defDays: 30, badFields: []string{"from"}},
		{name: "bad format", from: "01/10/2026", to: "2026-10-01", badFields: []string{"from"}},
		{name: "both bad", from: "x", to: "y", defDays: 30, badFields: []string{"to", "from"}},
		{name: "bad to, from defaulted", to: "nope", defDays: 30, badFields: []string{"to"}},
		{name: "not a real date", from: "2026-02-30", badFields: []string{"from"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := Parse(tt.from, tt.to, sp, now, tt.defDays)
			if tt.badFields != nil {
				if err == nil {
					t.Fatalf("expected an error, got %+v", r)
				}
				for _, f := range tt.badFields {
					if !strings.Contains(err.Error(), f+": ") {
						t.Errorf("error %q does not mention field %q", err, f)
					}
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if r.FromDay != tt.wantFrom || r.ToDay != tt.wantTo {
				t.Errorf("days = %q..%q, want %q..%q", r.FromDay, r.ToDay, tt.wantFrom, tt.wantTo)
			}
			if got := inst(r.From); got != tt.wantFromInst {
				t.Errorf("From = %q, want %q", got, tt.wantFromInst)
			}
			if got := inst(r.To); got != tt.wantToInst {
				t.Errorf("To = %q, want %q", got, tt.wantToInst)
			}
		})
	}
}

func inst(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
