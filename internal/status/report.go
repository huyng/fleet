// Package status describes what a node reports to the master and how the
// node gathers it.
//
// The Report type is the wire format: both the node that sends it and the
// master that receives it use this package, so the two always agree.
package status

import (
	"time"
)

// Report is one status sample from a node.
type Report struct {
	Hostname      string    `json:"hostname"`
	IP            string    `json:"ip"`
	UptimeSeconds float64   `json:"uptime_seconds"`
	Load          Load      `json:"load"`
	Memory        Usage     `json:"memory"`
	Disk          Usage     `json:"disk"`
	AgentVersion  string    `json:"agent_version"`
	ReportedAt    time.Time `json:"reported_at"`
}

// Load is the kernel load average over three windows.
type Load struct {
	OneMinute     float64 `json:"1m"`
	FiveMinutes   float64 `json:"5m"`
	FifteenMinute float64 `json:"15m"`
}

// Usage is a used and total pair of byte counts.
type Usage struct {
	Used  uint64 `json:"used"`
	Total uint64 `json:"total"`
}

// Percent is the used share of the total, from 0 to 100. A missing total
// yields zero rather than a division by zero.
func (u Usage) Percent() float64 {
	if u.Total == 0 {
		return 0
	}
	return float64(u.Used) / float64(u.Total) * 100
}
