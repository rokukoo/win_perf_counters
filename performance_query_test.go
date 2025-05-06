//go:build windows

package win_perf_counters

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPerformanceQueryImplIntegration(t *testing.T) {
	query := &performanceQueryImpl{maxBufferSize: uint32(defaultMaxBufferSize)}

	t.Logf("Test close before open")
	err := query.Close()
	require.ErrorIs(t, err, errUninitializedQuery)
	
	t.Logf("Test addCounterToQuery before open")
	_, err = query.AddCounterToQuery("")
	require.ErrorIs(t, err, errUninitializedQuery)

	t.Logf("Test addEnglishCounterToQuery before open")
	_, err = query.AddEnglishCounterToQuery("")
	require.ErrorIs(t, err, errUninitializedQuery)

	t.Logf("Test collectData before open")
	err = query.CollectData()
	require.ErrorIs(t, err, errUninitializedQuery)
	
	counterPath := "\\Processor Information(_Total)\\% Processor Time"

	t.Logf("Test addCounterToQuery")
	require.NoError(t, query.Open())
	hCounter, err := query.AddCounterToQuery(counterPath)
	require.NoError(t, err)
	require.NotEqual(t, 0, hCounter)
	require.NoError(t, query.Close())

	t.Logf("Test addEnglishCounterToQuery")
	require.NoError(t, query.Open())
	hCounter, err = query.AddEnglishCounterToQuery(counterPath)
	require.NoError(t, err)
	require.NotEqual(t, 0, hCounter)

	t.Logf("Test getCounterPath")
	cp, err := query.GetCounterPath(hCounter)
	require.NoError(t, err)
	require.True(t, strings.HasSuffix(cp, counterPath))

	require.NoError(t, query.CollectData())
	time.Sleep(time.Second)

	require.NoError(t, query.CollectData())

	t.Logf("Test getFormattedCounterValueDouble")
	fcounter, err := query.GetFormattedCounterValueDouble(hCounter)
	require.NoError(t, err)
	require.Greater(t, fcounter, float64(0))
	t.Logf("fcounter %s: %f", counterPath, fcounter)

	t.Logf("Test getRawCounterValue")
	rcounter, err := query.GetRawCounterValue(hCounter)
	require.NoError(t, err)
	require.Greater(t, rcounter, int64(10000000))
	t.Logf("rcounter %s: %d", counterPath, rcounter)

	t.Logf("Test collectDataWithTime")
	now := time.Now()
	mtime, err := query.CollectDataWithTime()
	require.NoError(t, err)
	require.Less(t, mtime.Sub(now), time.Second)
	t.Logf("mtime %s: %s", counterPath, mtime.Format(time.RFC3339))

	counterPath = "\\Process(*)\\% Processor Time"

	t.Logf("Test expandWildCardPath")
	paths, err := query.ExpandWildCardPath(counterPath)
	require.NoError(t, err)
	require.NotNil(t, paths)
	require.Greater(t, len(paths), 1)
	t.Logf("paths %s: %v", counterPath, paths)

	counterPath = "\\Process(_Total)\\*"

	t.Logf("Test expandWildCardPath with _Total")
	paths, err = query.ExpandWildCardPath(counterPath)
	require.NoError(t, err)
	require.NotNil(t, paths)
	require.Greater(t, len(paths), 1)
	t.Logf("paths %s: %v", counterPath, paths)

	counterPath = "\\Process(*)\\% Processor Time"
	
	t.Logf("Test addEnglishCounterToQuery")
	require.NoError(t, query.Open())
	hCounter, err = query.AddEnglishCounterToQuery(counterPath)
	require.NoError(t, err)
	require.NotEqual(t, 0, hCounter)

	t.Logf("Test collectData")
	require.NoError(t, query.CollectData())
	time.Sleep(time.Second)

	require.NoError(t, query.CollectData())

	t.Logf("Test getFormattedCounterArrayDouble")
	farr, err := query.GetFormattedCounterArrayDouble(hCounter)
	var phdErr *pdhError
	if errors.As(err, &phdErr) && phdErr.errorCode != pdhInvalidData && phdErr.errorCode != pdhCalcNegativeValue {
		time.Sleep(time.Second)
		farr, err = query.GetFormattedCounterArrayDouble(hCounter)
	}
	require.NoError(t, err)
	require.NotEmpty(t, farr)
	t.Logf("farr %s: %v", counterPath, farr)

	t.Logf("Test getRawCounterArray")
	rarr, err := query.GetRawCounterArray(hCounter)
	require.NoError(t, err)
	require.NotEmpty(t, rarr, "Too")
	t.Logf("rarr %s: %v", counterPath, rarr)
	require.NoError(t, query.Close())
}

func ExampleNewPerformanceQueryCreator() {
	counterPath := "\\Processor Information(_Total)\\% Processor Time"
	query := NewPerformanceQuery(uint32(defaultMaxBufferSize))
	query.Open()
	defer query.Close()
	handle, err := query.AddCounterToQuery(counterPath)
	if err != nil {
		fmt.Println(err)
		return
	}
	query.CollectData()
	time.Sleep(time.Second)
	if err := query.CollectData(); err != nil {
		fmt.Println(err)
		return
	}
	fcounter, err := query.GetFormattedCounterValueDouble(handle)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Printf("%s: %f", counterPath, fcounter)

	// Output:
	// \Processor Information(_Total)\% Processor Time: 0.000000
}