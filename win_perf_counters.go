//go:build windows

package win_perf_counters

import (
	_ "embed"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
	"time"
)

type CollectFunc func(measurement string, fields map[string]interface{}, tags map[string]string, timestamp time.Time)

//go:embed sample.conf
var sampleConfig string

// Size is an int64
type Size int64
type Duration time.Duration

var (
	defaultMaxBufferSize = Size(100 * 1024 * 1024)
	sanitizedChars       = strings.NewReplacer("/sec", "_persec", "/Sec", "_persec", " ", "_", "%", "Percent", `\`, "")
)

const emptyInstance = "------"

func NewWinPerfCounters(collectFunc CollectFunc) *WinPerfCounters {
	return &WinPerfCounters{
		CountersRefreshInterval:    Duration(time.Second * 60),
		LocalizeWildcardsExpansion: true,
		MaxBufferSize:              defaultMaxBufferSize,
		queryCreator:               &performanceQueryCreatorImpl{},
		collect:                  collectFunc,
	}
}

// WinPerfCounters 用于管理和采集 Windows 性能计数器数据的主要结构体。
type WinPerfCounters struct {
	// PrintValid 是否打印有效的计数器路径。
	PrintValid                 bool            `toml:"PrintValid"`
	// PreVistaSupport 是否支持 Vista 之前的系统（已废弃，动态判断）。
	PreVistaSupport            bool            `toml:"PreVistaSupport" deprecated:"1.7.0;1.35.0;determined dynamically"`
	// UsePerfCounterTime 是否使用性能计数器的时间戳。
	UsePerfCounterTime         bool            `toml:"UsePerfCounterTime"`
	// Object 配置的性能对象列表。
	Object                     []perfObject    `toml:"object"`
	// CountersRefreshInterval 性能计数器刷新间隔。
	CountersRefreshInterval    Duration `toml:"CountersRefreshInterval"`
	// UseWildcardsExpansion 是否启用通配符展开。
	UseWildcardsExpansion      bool            `toml:"UseWildcardsExpansion"`
	// LocalizeWildcardsExpansion 是否本地化通配符展开。
	LocalizeWildcardsExpansion bool            `toml:"LocalizeWildcardsExpansion"`
	// IgnoredErrors 需要忽略的错误列表。
	IgnoredErrors              []string        `toml:"IgnoredErrors"`
	// MaxBufferSize 最大缓冲区大小。
	MaxBufferSize              Size     `toml:"MaxBufferSize"`
	// Sources 数据源主机列表。
	Sources                    []string        `toml:"Sources"`
	// Log 日志记录器。
	Log Logger `toml:"-"`
	// lastRefreshed 上次刷新时间。	
	lastRefreshed time.Time
	// queryCreator 性能查询创建器。
	queryCreator  performanceQueryCreator
	// hostCounters 主机计数器信息映射。
	hostCounters  map[string]*hostCountersInfo
	// cachedHostname 缓存的主机名。
	cachedHostname string

	// collector 采集器。
	collect CollectFunc
}

// perfObject 表示一个性能对象的配置项，用于指定需要采集的性能计数器及其实例。
type perfObject struct {
	// Sources 指定采集该对象的主机列表。
	Sources       []string `toml:"Sources"`
	// ObjectName 性能对象名称。
	ObjectName    string   `toml:"ObjectName"`
	// Counters 需要采集的计数器名称列表。
	Counters      []string `toml:"Counters"`
	// Instances 需要采集的实例名称列表。
	Instances     []string `toml:"Instances"`
	// Measurement 采集数据对应的测量名称。
	Measurement   string   `toml:"Measurement"`
	// WarnOnMissing 缺失计数器时是否警告。
	WarnOnMissing bool     `toml:"WarnOnMissing"`
	// FailOnMissing 缺失计数器时是否报错并终止。
	FailOnMissing bool     `toml:"FailOnMissing"`
	// IncludeTotal 是否包含 _Total 实例。
	IncludeTotal  bool     `toml:"IncludeTotal"`
	// UseRawValues 是否采集原始值。
	UseRawValues  bool     `toml:"UseRawValues"`
}

// hostCountersInfo 存储主机性能计数器的相关信息。
type hostCountersInfo struct {
	// computer 用作键值和打印输出的计算机名称。
	computer string
	// tag 用于标签中的计算机名称。
	tag string
	// counters 该主机上的性能计数器列表。
	counters []*counter
	// query 性能计数器查询接口。
	query performanceQuery
	// timestamp 最近一次查询的时间戳。
	timestamp time.Time
}

// counter 表示一个性能计数器的配置和状态信息。
type counter struct {
	// counterPath 计数器的完整路径。
	counterPath   string
	// computer 计数器所属的计算机名称。
	computer      string
	// objectName 计数器所属的性能对象名称。
	objectName    string
	// counter 计数器名称。
	counter       string
	// instance 计数器实例名称。
	instance      string
	// measurement 计数器对应的测量名称。
	measurement   string
	// includeTotal 是否包含 _Total 实例。
	includeTotal  bool
	// useRawValue 是否使用原始值。
	useRawValue   bool
	// counterHandle 计数器句柄。
	counterHandle pdhCounterHandle
}

// instanceGrouping 用于将计数器数据分组为实例组。
type instanceGrouping struct {
	// name 实例组的名称。
	name       string
	// instance 实例名称。
	instance   string
	// objectName 性能对象名称。
	objectName string
}

type fieldGrouping map[instanceGrouping]map[string]interface{}

func (*WinPerfCounters) SampleConfig() string {
	return sampleConfig
}

func (m *WinPerfCounters) Init() error {
	// Check the buffer size
	if m.MaxBufferSize < Size(initialBufferSize) {
		return fmt.Errorf("maximum buffer size should at least be %d", 2*initialBufferSize)
	}
	if m.MaxBufferSize > math.MaxUint32 {
		return fmt.Errorf("maximum buffer size should be smaller than %d", uint32(math.MaxUint32))
	}

	if m.UseWildcardsExpansion && !m.LocalizeWildcardsExpansion {
		// Counters must not have wildcards with this option
		found := false
		wildcards := []string{"*", "?"}

		for _, object := range m.Object {
			for _, wildcard := range wildcards {
				if strings.Contains(object.ObjectName, wildcard) {
					found = true
					m.Log.Errorf("Object: %s, contains wildcard %s", object.ObjectName, wildcard)
				}
			}
			for _, counter := range object.Counters {
				for _, wildcard := range wildcards {
					if strings.Contains(counter, wildcard) {
						found = true
						m.Log.Errorf("Object: %s, counter: %s contains wildcard %s", object.ObjectName, counter, wildcard)
					}
				}
			}
		}

		if found {
			return errors.New("wildcards can't be used with LocalizeWildcardsExpansion=false")
		}
	}
	return nil
}

// Gather 收集性能计数器数据。
// 如果需要刷新计数器(根据 CountersRefreshInterval 配置)，会先清理旧的查询，重新解析配置并收集初始数据。
// 然后对每个主机并发收集计数器数据。
func (m *WinPerfCounters) Gather() error {
	// Parse the config once
	var err error

	// 检查是否需要刷新计数器
	if m.lastRefreshed.IsZero() || (m.CountersRefreshInterval > 0 && m.lastRefreshed.Add(time.Duration(m.CountersRefreshInterval)).Before(time.Now())) {
		if err := m.cleanQueries(); err != nil {
			return err
		}

		if err := m.parseConfig(); err != nil {
			return err
		}
		for _, hostCounterSet := range m.hostCounters {
			// some counters need two data samples before computing a value
			if err = hostCounterSet.query.collectData(); err != nil {
				return m.checkError(err)
			}
		}
		m.lastRefreshed = time.Now()
		// minimum time between collecting two samples
		time.Sleep(time.Second)
	}

	// 收集每个主机的计数器数据
	for _, hostCounterSet := range m.hostCounters {
		if m.UsePerfCounterTime && hostCounterSet.query.isVistaOrNewer() {
			// 使用性能计数器时间戳
			hostCounterSet.timestamp, err = hostCounterSet.query.collectDataWithTime()
			if err != nil {
				return err
			}
		} else {
			// 使用当前时间作为时间戳
			hostCounterSet.timestamp = time.Now()
			if err := hostCounterSet.query.collectData(); err != nil {
				return err
			}
		}
	}

	var wg sync.WaitGroup
	// iterate over computers
	for _, hostCounterInfo := range m.hostCounters {
		wg.Add(1)
		go func(hostInfo *hostCountersInfo) {
			m.Log.Debugf("Gathering from %s", hostInfo.computer)
			start := time.Now()
			err := m.gatherComputerCounters(hostInfo)
			m.Log.Debugf("Gathering from %s finished in %v", hostInfo.computer, time.Since(start))
			if err != nil && m.checkError(err) != nil {
				_ = fmt.Errorf("error during collecting data on host %q: %w", hostInfo.computer, err)
			}
			wg.Done()
		}(hostCounterInfo)
	}

	wg.Wait()
	return nil
}

func (m *WinPerfCounters) hostname() string {
	if m.cachedHostname != "" {
		return m.cachedHostname
	}
	hostname, err := os.Hostname()
	if err != nil {
		m.cachedHostname = "localhost"
	} else {
		m.cachedHostname = hostname
	}
	return m.cachedHostname
}

//nolint:revive //argument-limit conditionally more arguments allowed
func (m *WinPerfCounters) addItem(counterPath, computer, objectName, instance, counterName, measurement string, includeTotal bool, useRawValue bool) error {
	origCounterPath := counterPath
	var err error
	var counterHandle pdhCounterHandle

	sourceTag := computer
	if computer == "localhost" {
		sourceTag = m.hostname()
	}
	if m.hostCounters == nil {
		m.hostCounters = make(map[string]*hostCountersInfo)
	}
	hostCounter, ok := m.hostCounters[computer]
	if !ok {
		hostCounter = &hostCountersInfo{computer: computer, tag: sourceTag}
		m.hostCounters[computer] = hostCounter
		hostCounter.query = m.queryCreator.newPerformanceQuery(computer, uint32(m.MaxBufferSize))
		if err := hostCounter.query.open(); err != nil {
			return err
		}
		hostCounter.counters = make([]*counter, 0)
	}

	if !hostCounter.query.isVistaOrNewer() {
		counterHandle, err = hostCounter.query.addCounterToQuery(counterPath)
		if err != nil {
			return err
		}
	} else {
		counterHandle, err = hostCounter.query.addEnglishCounterToQuery(counterPath)
		if err != nil {
			return err
		}
	}

	if m.UseWildcardsExpansion {
		origInstance := instance
		counterPath, err = hostCounter.query.getCounterPath(counterHandle)
		if err != nil {
			return err
		}
		counters, err := hostCounter.query.expandWildCardPath(counterPath)
		if err != nil {
			return err
		}

		_, origObjectName, _, origCounterName, err := extractCounterInfoFromCounterPath(origCounterPath)
		if err != nil {
			return err
		}

		for _, counterPath := range counters {
			_, err := hostCounter.query.addCounterToQuery(counterPath)
			if err != nil {
				return err
			}

			computer, objectName, instance, counterName, err = extractCounterInfoFromCounterPath(counterPath)
			if err != nil {
				return err
			}

			var newItem *counter
			if !m.LocalizeWildcardsExpansion {
				// On localized installations of Windows, Telegraf
				// should return English metrics, but
				// expandWildCardPath returns localized counters. Undo
				// that by using the original object and counter
				// names, along with the expanded instance.

				var newInstance string
				if instance == "" {
					newInstance = emptyInstance
				} else {
					newInstance = instance
				}
				counterPath = formatPath(computer, origObjectName, newInstance, origCounterName)
				counterHandle, err = hostCounter.query.addEnglishCounterToQuery(counterPath)
				if err != nil {
					return err
				}
				newItem = newCounter(
					counterHandle,
					counterPath,
					computer,
					origObjectName, instance,
					origCounterName,
					measurement,
					includeTotal,
					useRawValue,
				)
			} else {
				counterHandle, err = hostCounter.query.addCounterToQuery(counterPath)
				if err != nil {
					return err
				}
				newItem = newCounter(
					counterHandle,
					counterPath,
					computer,
					objectName,
					instance,
					counterName,
					measurement,
					includeTotal,
					useRawValue,
				)
			}

			if instance == "_Total" && origInstance == "*" && !includeTotal {
				continue
			}

			hostCounter.counters = append(hostCounter.counters, newItem)

			if m.PrintValid {
				m.Log.Infof("Valid: %s", counterPath)
			}
		}
	} else {
		newItem := newCounter(
			counterHandle,
			counterPath,
			computer,
			objectName,
			instance,
			counterName,
			measurement,
			includeTotal,
			useRawValue,
		)
		hostCounter.counters = append(hostCounter.counters, newItem)
		if m.PrintValid {
			m.Log.Infof("Valid: %s", counterPath)
		}
	}

	return nil
}

func (m *WinPerfCounters) parseConfig() error {
	var counterPath string

	if len(m.Sources) == 0 {
		m.Sources = []string{"localhost"}
	}

	if len(m.Object) == 0 {
		err := errors.New("no performance objects configured")
		return err
	}

	for _, PerfObject := range m.Object {
		computers := PerfObject.Sources
		if len(computers) == 0 {
			computers = m.Sources
		}
		for _, computer := range computers {
			if computer == "" {
				// localhost as a computer name in counter path doesn't work
				computer = "localhost"
			}
			for _, counter := range PerfObject.Counters {
				if len(PerfObject.Instances) == 0 {
					m.Log.Warnf("Missing 'Instances' param for object %q", PerfObject.ObjectName)
				}
				for _, instance := range PerfObject.Instances {
					objectName := PerfObject.ObjectName
					counterPath = formatPath(computer, objectName, instance, counter)

					err := m.addItem(counterPath, computer, objectName, instance, counter,
						PerfObject.Measurement, PerfObject.IncludeTotal, PerfObject.UseRawValues)
					if err != nil {
						if PerfObject.FailOnMissing || PerfObject.WarnOnMissing {
							m.Log.Errorf("Invalid counterPath %q: %s", counterPath, err.Error())
						}
						if PerfObject.FailOnMissing {
							return err
						}
					}
				}
			}
		}
	}

	return nil
}

func (m *WinPerfCounters) gatherComputerCounters(hostCounterInfo *hostCountersInfo) error {
	var value interface{}
	var err error
	collectedFields := make(fieldGrouping)
	// For iterate over the known metrics and get the samples.
	for _, metric := range hostCounterInfo.counters {
		// collect
		if m.UseWildcardsExpansion {
			if metric.useRawValue {
				value, err = hostCounterInfo.query.getRawCounterValue(metric.counterHandle)
			} else {
				value, err = hostCounterInfo.query.getFormattedCounterValueDouble(metric.counterHandle)
			}
			if err != nil {
				// ignore invalid data  as some counters from process instances returns this sometimes
				if !isKnownCounterDataError(err) {
					return fmt.Errorf("error while getting value for counter %q: %w", metric.counterPath, err)
				}
				m.Log.Warnf("Error while getting value for counter %q, instance: %s, will skip metric: %v", metric.counterPath, metric.instance, err)
				continue
			}
			addCounterMeasurement(metric, metric.instance, value, collectedFields)
		} else {
			var counterValues []counterValue
			if metric.useRawValue {
				counterValues, err = hostCounterInfo.query.getRawCounterArray(metric.counterHandle)
			} else {
				counterValues, err = hostCounterInfo.query.getFormattedCounterArrayDouble(metric.counterHandle)
			}
			if err != nil {
				// ignore invalid data  as some counters from process instances returns this sometimes
				if !isKnownCounterDataError(err) {
					return fmt.Errorf("error while getting value for counter %q: %w", metric.counterPath, err)
				}
				m.Log.Warnf("Error while getting value for counter %q, instance: %s, will skip metric: %v", metric.counterPath, metric.instance, err)
				continue
			}
			for _, cValue := range counterValues {
				if strings.Contains(metric.instance, "#") && strings.HasPrefix(metric.instance, cValue.instanceName) {
					// If you are using a multiple instance identifier such as "w3wp#1"
					// phd.dll returns only the first 2 characters of the identifier.
					cValue.instanceName = metric.instance
				}

				if shouldIncludeMetric(metric, cValue) {
					addCounterMeasurement(metric, cValue.instanceName, cValue.value, collectedFields)
				}
			}
		}
	}
	for instance, fields := range collectedFields {
		var tags = map[string]string{
			"objectname": instance.objectName,
		}
		if len(instance.instance) > 0 {
			tags["instance"] = instance.instance
		}
		if len(hostCounterInfo.tag) > 0 {
			tags["source"] = hostCounterInfo.tag
		}
		if m.collect != nil {
			m.collect(instance.name, fields, tags, hostCounterInfo.timestamp)
		}
	}
	return nil
}

// cleanQueries 清理所有主机的性能计数器查询。
//
// 该方法会关闭所有主机的性能计数器查询，并清空 hostCounters 映射。
// 在重新解析配置和刷新计数器之前需要调用此方法。
//
// 返回值：
//   error：如果关闭查询时发生错误则返回相应错误，否则返回 nil。
func (m *WinPerfCounters) cleanQueries() error {
	for _, hostCounterInfo := range m.hostCounters {
		if err := hostCounterInfo.query.close(); err != nil {
			return err
		}
	}
	m.hostCounters = nil
	return nil
}

// shouldIncludeMetric 判断是否应该包含某个性能计数器指标。
//
// 参数：
//   metric *counter：计数器对象，包含计数器的相关信息。
//   cValue counterValue：计数器值对象，包含实例名称等信息。
//
// 返回值：
//   bool：如果应该包含该指标返回 true，否则返回 false。
func shouldIncludeMetric(metric *counter, cValue counterValue) bool {
	if metric.includeTotal {
		// 如果设置了 includeTotal，包含所有计数器
		return true
	}
	if metric.instance == "*" && !strings.Contains(cValue.instanceName, "_Total") {
		// 如果实例设置为 "*" 且不是 "_Total" 实例，则包含
		return true
	}
	if metric.instance == cValue.instanceName {
		// 如果实例名称完全匹配，则包含
		return true
	}
	if metric.instance == emptyInstance {
		// 如果是空实例，则包含
		return true
	}
	return false
}

// addCounterMeasurement 用于将采集到的计数器数据添加到收集字段中。
//
// 参数：
//   metric *counter：计数器对象，包含计数器的相关信息。
//   instanceName string：实例名称，用于区分不同的计数器实例。
//   value interface{}：计数器采集到的值。
//   collectFields fieldGrouping：用于收集所有计数器字段的映射。
func addCounterMeasurement(metric *counter, instanceName string, value interface{}, collectFields fieldGrouping) {
	var instance = instanceGrouping{metric.measurement, instanceName, metric.objectName}
	if collectFields[instance] == nil {
		collectFields[instance] = make(map[string]interface{})
	}
	collectFields[instance][sanitizedChars.Replace(metric.counter)] = value
}