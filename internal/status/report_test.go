package status

import "testing"

func TestUsagePercent(t *testing.T) {
	cases := []struct {
		name string
		in   Usage
		want float64
	}{
		{"half", Usage{Used: 5, Total: 10}, 50},
		{"empty", Usage{}, 0},
		{"full", Usage{Used: 10, Total: 10}, 100},
	}
	for _, c := range cases {
		if got := c.in.Percent(); got != c.want {
			t.Errorf("%s: Percent() = %v, want %v", c.name, got, c.want)
		}
	}
}
