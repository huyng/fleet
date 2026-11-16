package config

import (
	"fmt"
	"time"
)

// Duration lets a TOML string such as "15s" fill a time.Duration field.
type Duration struct {
	time.Duration
}

// UnmarshalText parses a Go duration string.
//
// Raises:
//
//	An error if the text is not a duration, such as "15".
func (d *Duration) UnmarshalText(text []byte) error {
	parsed, err := time.ParseDuration(string(text))
	if err != nil {
		return fmt.Errorf("bad duration %q: %w", text, err)
	}
	d.Duration = parsed
	return nil
}

// MarshalText writes the duration back as a Go duration string.
func (d Duration) MarshalText() ([]byte, error) {
	return []byte(d.Duration.String()), nil
}
