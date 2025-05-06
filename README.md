# Windows Performance Counters

## 介绍

本项目用于在 Windows 系统上采集和管理性能计数器数据，适合系统监控、性能分析等场景。核心模块包括 performance_query 和 win_perf_counters。

## 主要模块介绍

### 1. performance_query

`performance_query` 封装了 Windows 性能计数器（PDH API）的底层调用，提供了 Go 语言友好的接口。你可以通过它：

- 创建和管理性能计数器查询（Open/Close）
- 添加计数器（支持英文和本地化名称）
- 支持通配符展开（ExpandWildCardPath）
- 获取计数器的原始值或格式化值（单值或数组）
- 支持 Vista 及以上系统的时间戳采集

接口定义如下（简要）：

```go
type PerformanceQuery interface {
    Open() error
    Close() error
    AddCounterToQuery(counterPath string) (pdhCounterHandle, error)
    AddEnglishCounterToQuery(counterPath string) (pdhCounterHandle, error)
    GetCounterPath(counterHandle pdhCounterHandle) (string, error)
    ExpandWildCardPath(counterPath string) ([]string, error)
    GetFormattedCounterValueDouble(hCounter pdhCounterHandle) (float64, error)
    GetRawCounterValue(hCounter pdhCounterHandle) (int64, error)
    GetFormattedCounterArrayDouble(hCounter pdhCounterHandle) ([]counterValue, error)
    GetRawCounterArray(hCounter pdhCounterHandle) ([]counterValue, error)
    CollectData() error
    CollectDataWithTime() (time.Time, error)
    IsVistaOrNewer() bool
}
```

简单的使用案例：

```go
func CollectTotalProcessorInformation() error {
    counterPath := "\\Processor Information(_Total)\\% Processor Time"
    query := NewPerformanceQuery(uint32(defaultMaxBufferSize))
    query.Open()
    defer query.Close()
    handle, err := query.AddCounterToQuery(counterPath)
    if err != nil {
        return err
    }
    // 必须要先执行一次数据收集后且过一段时间才能获取真正有用的数据
    query.CollectData()
    time.Sleep(time.Second)
    if err := query.CollectData(); err != nil {
        return err
    }
    fcounter, err := query.GetFormattedCounterValueDouble(handle)
    if err != nil {
        return err
    }
    fmt.Printf("%s: %f", counterPath, fcounter)
}
```

### 2. win_perf_counters

`win_perf_counters` 是对 performance_query 的进一步封装，支持批量配置、采集和标签化性能计数器数据。它支持：

- 多主机、多对象、多计数器的灵活配置
- 通配符展开与本地化兼容
- 支持自定义采集周期、缓冲区大小、错误忽略等
- 采集数据后可通过回调函数（CollectFunc）自定义处理

常用方法：

- `NewWinPerfCounters(collectFunc CollectFunc) *WinPerfCounters`：创建采集器实例
- `(*WinPerfCounters) Init() error`：初始化配置
- `(*WinPerfCounters) Gather() error`：采集一次数据

配置示例:

```toml
# 获取 CPU 占用率
[[object]]
	Measurement = "win_cpu"
	ObjectName = "Processor Information"
	Instances = ["_Total"]
	Counters = [
		"% Processor Utility",
	]
```

简单的使用案例：

```golang
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
```

请参考 sample.conf，以下为常见配置项：

#### PrintValid

布尔值。如果设置为 true，将打印出所有匹配的性能对象。

示例：PrintValid=true

#### LocalizeWildcardsExpansion

当 UseWildcardsExpansion 为 true 且 Telegraf 运行在本地化的
Windows 上时，选择对象和计数器名称是否本地化。

- 为 true 时，即使对象和计数器名为英文，Telegraf 也会生成本地化的标签和字段。
- 为 false 时，Telegraf 期望对象和计数器名为英文，并生成英文标签和字段。
- 为 false 时，通配符只能用于实例，对象和计数器名不能有通配符。
  示例：LocalizeWildcardsExpansion=true

示例：LocalizeWildcardsExpansion=true

#### CountersRefreshInterval

配置的计数器会按照 CountersRefreshInterval 参数指定的间隔与可用计数器进行匹配。默认值为 1m（1 分钟）。

如果实例名或计数器名中使用了通配符，并且 UseWildcardsExpansion 为 true，则会在此时进行扩展。

设置过低（如几秒）的刷新间隔可能导致 Telegraf 占用较高的 CPU。

设置为 0s 可禁用定期刷新。

示例：CountersRefreshInterval=1m

#### PreVistaSupport

> 1.7 版本弃用；Vista 及更高版本所需功能会动态检测

布尔值。如果为 true，插件将使用 Vista 之前的本地化 PerfCounter 接口以兼容旧系统。

建议在 Vista 及更高版本操作系统上不要使用，因为配置比新接口更复杂。

如在 Windows Server 2003 上应设置为 true：PreVistaSupport=true

#### UsePerfCounterTime

布尔值。如果为 true，将请求带有时间戳的 PerfCounter 数据；为 false 时使用当前时间。

支持 Windows Vista/Server 2008 及更高版本。

示例：UsePerfCounterTime=true

#### IgnoredErrors

IgnoredErrors 接受一个 PDH 错误码列表（在 pdh.go 中定义），遇到这些错误时会被忽略。例如，可以提供 "PDH_NO_DATA" 来忽略没有实例的性能计数器。默认不忽略任何错误。

示例：IgnoredErrors=["PDH_NO_DATA"]

#### Sources（可选）

要采集性能计数器的主机名或 IP 地址。运行 Telegraf 的用户必须对远程计算机有认证权限（如通过 Windows 共享 net use \\SQL-SERVER-01）。

使用 "localhost" 或本机名可同时采集本地计数器。仅采集本地时可省略。

如果某个性能计数器只在特定主机上存在，可在该计数器级别配置 Sources 覆盖全局设置。

示例：Sources = ["localhost", "SQL-SERVER-01", "SQL-SERVER-02", "SQL-SERVER-03"]
默认：Sources = ["localhost"]

#### Object

一个新的配置项以 [[object]] 的 TOML 头开始，需放在主 win_perf_counters 配置下方。

接下来有 3 个必需的键值对和 3 个可选参数。

**ObjectName（必需）**

要查询的对象名称，如 Processor、DirectoryServices、LogicalDisk 等。

示例：ObjectName = "LogicalDisk"

**Instances（必需）**

instances 键（数组）声明要返回的计数器实例，可以是一个或多个值。

示例：Instances = ["C:","D:","E:"]

只返回 C:、D:、E: 的相关实例。要获取所有实例，使用 [""]。默认会去除包含 \_Total 的结果，除非明确指定。

也可设置部分通配符，如 ["chrome"]，需 UseWildcardsExpansion=true。

有些对象没有实例，此时需设置 Instances = ["------"]。

**Counters（必需）**

Counters 键（数组）声明要返回的对象计数器，可以是一个或多个值。

示例：Counters = ["% Idle Time", "% Disk Read Time", "% Disk Write Time"]

每个需要结果的计数器都要指定，或用 [""] 获取所有计数器（需 UseWildcardsExpansion=true）。

**Sources（对象级）（可选）**

覆盖当前性能对象的全局 Sources 参数，详见上文 Sources。

**Measurement（可选）**

不设置时默认为 win_perf_counters。在 InfluxDB 中作为数据存储的键。建议为不同类型数据（如 IIS、Disk、Processor）分别设置。

示例：Measurement = "win_disk"

**UseRawValues（可选）**

布尔值。为 true 时，计数器值以原始整数形式返回（带 Raw 后缀），否则以格式化形式返回。原始值适合进一步计算。

注意：基于时间的计数器（如 % Processor Time）以百分之一纳秒为单位。

示例：UseRawValues = true

**IncludeTotal（可选）**

布尔值。仅当 Instances = [""] 时有效，且希望返回所有包含 \_Total 的实例时设置为 true。

如 Processor Information。

**WarnOnMissing（可选）**

布尔值。仅在插件首次执行时有效。会打印所有未匹配的 ObjectName/Instance/Counter 组合，便于调试新配置。

**FailOnMissing（内部参数）**

不建议使用，仅供测试。布尔值。为 true 时，若有无效组合，插件会中止运行。

## 相关资料

[telegraf-win_perf_counters](https://github.com/influxdata/telegraf/blob/master/plugins/inputs/win_perf_counters)
