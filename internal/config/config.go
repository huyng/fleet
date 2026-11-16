// Package config loads the TOML configuration for each role.
//
// Loading is the only place that reads files; everything below this package
// receives an already validated Master or Node value.
package config

import (
	"fmt"
	"os"
	"time"

	"github.com/BurntSushi/toml"
)

// Defaults applied when a config field is left out.
const (
	DefaultListen       = "127.0.0.1:8080"
	DefaultInterval     = 15 * time.Second
	DefaultReportPath   = "/" // filesystem measured for disk usage
	staleIntervalFactor = 3   // a node is stale after this many intervals
)

// Master is the configuration of the machine that collects reports.
type Master struct {
	// Listen is the address the HTTP API and web page bind to.
	Listen string `toml:"listen"`
	// Secret is the shared token every request must carry.
	Secret string `toml:"secret"`
	// ReportInterval is the interval nodes are told to use. It only sets the
	// point at which a silent node is called stale.
	ReportInterval Duration `toml:"report_interval"`
}

// Node is the configuration of a machine that reports to a master.
type Node struct {
	// Master is the base URL of the master, such as http://10.0.0.1:8080.
	Master string `toml:"master"`
	// Secret is the shared token, matching the master's.
	Secret string `toml:"secret"`
	// Interval is how often a report is sent.
	Interval Duration `toml:"interval"`
	// Hostname overrides the detected hostname.
	Hostname string `toml:"hostname"`
	// ReportPath is the path whose filesystem is measured for disk usage.
	ReportPath string `toml:"report_path"`
	// RebootCommand is run when the master asks for a reboot.
	RebootCommand []string `toml:"reboot_command"`
}

// StaleAfter is how long a silent node still counts as healthy.
func (m Master) StaleAfter() time.Duration {
	return m.ReportInterval.Duration * staleIntervalFactor
}

// LoadMaster reads and validates a master config file.
//
// Args:
//
//	path: file to read.
//
// Returns:
//
//	The validated config with defaults applied.
//
// Raises:
//
//	An error if the file cannot be read, parsed, or a required field is empty.
func LoadMaster(path string) (Master, error) {
	var cfg Master
	if err := decode(path, &cfg); err != nil {
		return Master{}, err
	}

	if cfg.Listen == "" {
		cfg.Listen = DefaultListen
	}
	if cfg.ReportInterval.Duration == 0 {
		cfg.ReportInterval = Duration{Duration: DefaultInterval}
	}

	if cfg.Secret == "" {
		return Master{}, fmt.Errorf("%s: secret must not be empty", path)
	}
	return cfg, nil
}

// LoadNode reads and validates a node config file.
//
// Args:
//
//	path: file to read.
//
// Returns:
//
//	The validated config with defaults applied.
//
// Raises:
//
//	An error if the file cannot be read, parsed, or a required field is empty.
func LoadNode(path string) (Node, error) {
	var cfg Node
	if err := decode(path, &cfg); err != nil {
		return Node{}, err
	}

	if cfg.Interval.Duration == 0 {
		cfg.Interval = Duration{Duration: DefaultInterval}
	}
	if cfg.ReportPath == "" {
		cfg.ReportPath = DefaultReportPath
	}
	if len(cfg.RebootCommand) == 0 {
		cfg.RebootCommand = []string{"systemctl", "reboot"}
	}

	switch {
	case cfg.Master == "":
		return Node{}, fmt.Errorf("%s: master must not be empty", path)
	case cfg.Secret == "":
		return Node{}, fmt.Errorf("%s: secret must not be empty", path)
	case cfg.Interval.Duration <= 0:
		return Node{}, fmt.Errorf("%s: interval must be positive", path)
	}
	return cfg, nil
}

// decode reads a TOML file into target.
func decode(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	meta, err := toml.Decode(string(data), target)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	// Unknown keys are an error so a typo in the config is reported rather
	// than silently ignored.
	if unknown := meta.Undecoded(); len(unknown) > 0 {
		return fmt.Errorf("%s: unknown key %q", path, unknown[0].String())
	}
	return nil
}
