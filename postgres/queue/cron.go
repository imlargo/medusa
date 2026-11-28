package queue

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// cron is a parsed five-field cron expression: minute, hour, day of month,
// month, day of week. Each field is a set of allowed values, one bit each.
type cron struct {
	minute, hour, dom, month, dow uint64

	// A day matches when both day fields allow it, unless both are
	// restricted: then either is enough, as in every cron since Vixie's.
	domRestricted, dowRestricted bool
}

// cronFields are the bounds of the five fields, in order.
var cronFields = [5]struct {
	name     string
	min, max int
}{
	{"minute", 0, 59},
	{"hour", 0, 23},
	{"day of month", 1, 31},
	{"month", 1, 12},
	{"day of week", 0, 7}, // 0 and 7 are both Sunday
}

// parseCron reads "minute hour day-of-month month day-of-week". A field is
// "*", a number, a range "a-b", a list "a,b,c", or any of those with a step
// "/n". Names (MON, JAN) and the @daily shortcuts are not supported.
func parseCron(expr string) (cron, error) {
	fields := strings.Fields(expr)
	if len(fields) != len(cronFields) {
		return cron{}, fmt.Errorf("cron %q: want 5 fields (minute hour day-of-month month day-of-week), got %d", expr, len(fields))
	}

	var sets [5]uint64
	for i, field := range fields {
		set, err := parseCronField(field, cronFields[i].min, cronFields[i].max)
		if err != nil {
			return cron{}, fmt.Errorf("cron %q: %s: %w", expr, cronFields[i].name, err)
		}
		sets[i] = set
	}

	// Fold Sunday written as 7 onto 0.
	if sets[4]&(1<<7) != 0 {
		sets[4] = sets[4]&^(1<<7) | 1
	}

	return cron{
		minute: sets[0], hour: sets[1], dom: sets[2], month: sets[3], dow: sets[4],
		domRestricted: !strings.HasPrefix(fields[2], "*"),
		dowRestricted: !strings.HasPrefix(fields[4], "*"),
	}, nil
}

func parseCronField(field string, lo, hi int) (uint64, error) {
	var set uint64
	for part := range strings.SplitSeq(field, ",") {
		spec, stepText, hasStep := strings.Cut(part, "/")
		step := 1
		if hasStep {
			var err error
			if step, err = strconv.Atoi(stepText); err != nil || step < 1 {
				return 0, fmt.Errorf("step %q is not a positive number", stepText)
			}
		}

		from, to := lo, hi
		switch {
		case spec == "*":
		case strings.Contains(spec, "-"):
			a, b, _ := strings.Cut(spec, "-")
			var errA, errB error
			from, errA = strconv.Atoi(a)
			to, errB = strconv.Atoi(b)
			if errA != nil || errB != nil {
				return 0, fmt.Errorf("range %q is not two numbers", spec)
			}
		default:
			value, err := strconv.Atoi(spec)
			if err != nil {
				return 0, fmt.Errorf("%q is not a number", spec)
			}
			from, to = value, value
			if hasStep {
				to = hi // "5/15" means from 5 on, every 15
			}
		}
		if from < lo || to > hi || from > to {
			return 0, fmt.Errorf("%q is outside %d-%d", part, lo, hi)
		}

		for v := from; v <= to; v += step {
			set |= 1 << v
		}
	}
	return set, nil
}

// matches reports whether t, read in UTC, is a minute the expression names.
func (c cron) matches(t time.Time) bool {
	t = t.UTC()
	if c.minute&(1<<t.Minute()) == 0 || c.hour&(1<<t.Hour()) == 0 || c.month&(1<<int(t.Month())) == 0 {
		return false
	}

	dom := c.dom&(1<<t.Day()) != 0
	dow := c.dow&(1<<int(t.Weekday())) != 0
	if c.domRestricted && c.dowRestricted {
		return dom || dow
	}
	return dom && dow
}
