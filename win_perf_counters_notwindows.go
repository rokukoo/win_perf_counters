//go:build !windows

package win_perf_counters

import (
	_ "embed"
)

//go:embed sample.conf
var sampleConfig string

type WinPerfCounters struct {
	Log Logger `toml:"-"`
}

func (*WinPerfCounters) SampleConfig() string { return sampleConfig }

func (w *WinPerfCounters) Init() error {
	w.Log.Warn("Current platform is not supported")
	return nil
}