package queue

import (
	"testing"
	"time"
)

func TestCronMatches(t *testing.T) {
	at := func(value string) time.Time {
		parsed, err := time.Parse("2006-01-02 15:04", value)
		if err != nil {
			t.Fatal(err)
		}
		return parsed
	}

	tests := []struct {
		expr string
		time string // 2026-11-27 is a Friday
		want bool
	}{
		{"35 6 * * *", "2026-11-27 06:35", true},
		{"35 6 * * *", "2026-11-27 06:36", false},
		{"35 6 * * *", "2026-11-27 18:35", false},
		{"40 4,16 * * *", "2026-11-27 16:40", true},
		{"40 4,16 * * *", "2026-11-27 10:40", false},
		{"20 */6 * * *", "2026-11-27 12:20", true},
		{"20 */6 * * *", "2026-11-27 13:20", false},
		{"*/5 * * * *", "2026-11-27 09:55", true},
		{"*/5 * * * *", "2026-11-27 09:56", false},
		{"0 9-17 * * 1-5", "2026-11-27 17:00", true},
		{"0 9-17 * * 1-5", "2026-11-28 12:00", false}, // Saturday
		{"0 0 1 * *", "2026-12-01 00:00", true},
		{"0 0 1 * *", "2026-12-02 00:00", false},
		{"0 0 * * 7", "2026-11-29 00:00", true}, // Sunday written as 7
		{"0 0 * * 0", "2026-11-29 00:00", true},
		{"5/15 * * * *", "2026-11-27 09:35", true},
		{"5/15 * * * *", "2026-11-27 09:30", false},
		// Both day fields restricted: either one is enough.
		{"0 0 13 * 5", "2026-11-27 00:00", true}, // a Friday, not the 13th
		{"0 0 13 * 5", "2026-11-13 00:00", true}, // the 13th
		{"0 0 13 * 5", "2026-11-12 00:00", false},
	}

	for _, tt := range tests {
		c, err := parseCron(tt.expr)
		if err != nil {
			t.Errorf("parseCron(%q) = %v", tt.expr, err)
			continue
		}
		if got := c.matches(at(tt.time)); got != tt.want {
			t.Errorf("%q at %s = %v, want %v", tt.expr, tt.time, got, tt.want)
		}
	}
}

// A schedule is read in UTC whatever zone the time carries.
func TestCronReadsTheTimeInUTC(t *testing.T) {
	c, err := parseCron("35 6 * * *")
	if err != nil {
		t.Fatal(err)
	}

	bogota := time.FixedZone("America/Bogota", -5*60*60)
	if !c.matches(time.Date(2026, 11, 27, 1, 35, 0, 0, bogota)) {
		t.Error("01:35 in Bogota is 06:35 UTC, want a match")
	}
}

func TestParseCronRejectsWhatItCannotRead(t *testing.T) {
	for _, expr := range []string{
		"",
		"* * * *",
		"* * * * * *",
		"60 * * * *",
		"* 24 * * *",
		"* * 0 * *",
		"* * * 13 *",
		"* * * * 8",
		"*/0 * * * *",
		"5-1 * * * *",
		"a * * * *",
		"@daily",
		"0 0 * * MON",
	} {
		if _, err := parseCron(expr); err == nil {
			t.Errorf("parseCron(%q) = nil, want an error", expr)
		}
	}
}
