package workflows

import (
	"testing"
	"time"

	"cosmicbizwitch/pkg/workflow/testutil"
)

// TestFormatDate exercises format_date with 10 varied dates, using both string
// inputs (in multiple parseable layouts) and time.Time inputs.
func TestFormatDate(t *testing.T) {
	fn := newFormatDateFn()

	cases := []struct {
		name  string
		input any // string or time.Time
		want  string
	}{
		// ── string inputs ──────────────────────────────────────────────────────
		{
			name:  "ISO date string",
			input: "1990-03-15",
			want:  "Mar 15, 1990",
		},
		{
			name:  "US slash format",
			input: "07/04/1776",
			want:  "Jul 4, 1776",
		},
		{
			name:  "long month format",
			input: "December 25, 2023",
			want:  "Dec 25, 2023",
		},
		{
			name:  "RFC3339 with time",
			input: "2024-06-21T00:00:00Z",
			want:  "Jun 21, 2024",
		},
		{
			name:  "datetime with millis",
			input: "2001-09-11 08:46:00.000Z",
			want:  "Sep 11, 2001",
		},
		{
			name:  "New Year's Eve string",
			input: "1999-12-31",
			want:  "Dec 31, 1999",
		},

		// ── time.Time inputs ───────────────────────────────────────────────────
		{
			name:  "time.Time - leap day",
			input: time.Date(2000, time.February, 29, 0, 0, 0, 0, time.UTC),
			want:  "Feb 29, 2000",
		},
		{
			name:  "time.Time - Halloween",
			input: time.Date(2025, time.October, 31, 0, 0, 0, 0, time.UTC),
			want:  "Oct 31, 2025",
		},
		{
			name:  "time.Time - January 1st",
			input: time.Date(2026, time.January, 1, 12, 0, 0, 0, time.UTC),
			want:  "Jan 1, 2026",
		},
		{
			name:  "time.Time - pi day",
			input: time.Date(2015, time.March, 14, 9, 26, 53, 0, time.UTC),
			want:  "Mar 14, 2015",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := testutil.New(fn).
				WithInput("date", tc.input).
				WithInput("value", "date").
				WithInput("format", "Jan 2, 2006").
				Run()

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			got, ok := out["date"].(string)
			if !ok {
				t.Fatalf("expected string in out[\"date\"], got %T: %v", out["date"], out["date"])
			}

			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
