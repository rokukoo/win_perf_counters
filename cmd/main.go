//go:build windows

package main

import (
	_ "embed"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/rokukoo/win_perf_counters"
)

//go:embed config.conf
var config string

var logger = win_perf_counters.Logger{
	Name: "win_perf_counters",
	Quiet: false,
}

// 定义采集回调函数
func collectFunc(measurement string, fields map[string]interface{}, tags map[string]string, timestamp time.Time) {
	logger.Infof("[采集时间]%v [测量]%s [标签]%v [字段]%v\n", timestamp, measurement, tags, fields)
}

func main() {
	winPerfCounters := win_perf_counters.NewWinPerfCounters(collectFunc)
	if _, err := toml.Decode(config, winPerfCounters); err != nil {
		panic(err)
	}
	winPerfCounters.Init()

	ticker := time.NewTicker(1 * time.Second)
    defer ticker.Stop()
    for {
        <-ticker.C
        winPerfCounters.Gather()
    }
}