package api

import "testing"

func TestParseRange(t *testing.T) {
	cases := map[string]int64{
		"15m":     900,
		"1h":      3600,
		"6h":      21600,
		"24h":     86400,
		"7d":      604800,
		"1w":      604800,
		"3mo":     3 * 30 * 86400,
		"1y":      31536000, // the regression: must NOT fall through to 3600
		"2y":      2 * 31536000,
		"":        3600, // bad inputs default to 1h
		"garbage": 3600,
		"-5h":     3600,
		"0d":      3600,
	}
	for in, want := range cases {
		if got := parseRange(in); got != want {
			t.Errorf("parseRange(%q) = %d, want %d", in, got, want)
		}
	}
}
