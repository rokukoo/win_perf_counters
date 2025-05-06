//go:build windows

package win_perf_counters

import (
	"errors"
	"fmt"
	"slices"
)

// extractCounterInfoFromCounterPath gets object name, instance name (if available) and counter name from counter path
// General Counter path pattern is: \\computer\object(parent/instance#index)\counter
// parent/instance#index part is skipped in single instance objects (e.g. Memory): \\computer\object\counter
//
//nolint:revive //function-result-limit conditionally 5 return results allowed
func extractCounterInfoFromCounterPath(counterPath string) (computer string, object string, instance string, counter string, err error) {
	leftComputerBorderIndex := -1
	rightObjectBorderIndex := -1
	leftObjectBorderIndex := -1
	leftCounterBorderIndex := -1
	rightInstanceBorderIndex := -1
	leftInstanceBorderIndex := -1
	var bracketLevel int

	for i := len(counterPath) - 1; i >= 0; i-- {
		switch counterPath[i] {
		case '\\':
			if bracketLevel == 0 {
				if leftCounterBorderIndex == -1 {
					leftCounterBorderIndex = i
				} else if leftObjectBorderIndex == -1 {
					leftObjectBorderIndex = i
				} else if leftComputerBorderIndex == -1 {
					leftComputerBorderIndex = i
				}
			}
		case '(':
			bracketLevel--
			if leftInstanceBorderIndex == -1 && bracketLevel == 0 && leftObjectBorderIndex == -1 && leftCounterBorderIndex > -1 {
				leftInstanceBorderIndex = i
				rightObjectBorderIndex = i
			}
		case ')':
			if rightInstanceBorderIndex == -1 && bracketLevel == 0 && leftCounterBorderIndex > -1 {
				rightInstanceBorderIndex = i
			}
			bracketLevel++
		}
	}
	if rightObjectBorderIndex == -1 {
		rightObjectBorderIndex = leftCounterBorderIndex
	}
	if rightObjectBorderIndex == -1 || leftObjectBorderIndex == -1 {
		return "", "", "", "", errors.New("cannot parse object from: " + counterPath)
	}

	if leftComputerBorderIndex > -1 {
		// validate there is leading \\ and not empty computer (\\\O)
		if leftComputerBorderIndex != 1 || leftComputerBorderIndex == leftObjectBorderIndex-1 {
			return "", "", "", "", errors.New("cannot parse computer from: " + counterPath)
		}
		computer = counterPath[leftComputerBorderIndex+1 : leftObjectBorderIndex]
	}

	if leftInstanceBorderIndex > -1 && rightInstanceBorderIndex > -1 {
		instance = counterPath[leftInstanceBorderIndex+1 : rightInstanceBorderIndex]
	} else if (leftInstanceBorderIndex == -1 && rightInstanceBorderIndex > -1) || (leftInstanceBorderIndex > -1 && rightInstanceBorderIndex == -1) {
		return "", "", "", "", errors.New("cannot parse instance from: " + counterPath)
	}
	object = counterPath[leftObjectBorderIndex+1 : rightObjectBorderIndex]
	counter = counterPath[leftCounterBorderIndex+1:]
	return computer, object, instance, counter, nil
}


//nolint:revive //argument-limit conditionally more arguments allowed for helper function
func newCounter(
	counterHandle pdhCounterHandle,
	counterPath string,
	computer string,
	objectName string,
	instance string,
	counterName string,
	measurement string,
	includeTotal bool,
	useRawValue bool,
) *counter {
	measurementName := sanitizedChars.Replace(measurement)
	if measurementName == "" {
		measurementName = "win_perf_counters"
	}
	newCounterName := sanitizedChars.Replace(counterName)
	if useRawValue {
		newCounterName += "_Raw"
	}
	return &counter{counterPath, computer, objectName, newCounterName, instance, measurementName,
		includeTotal, useRawValue, counterHandle}
}

func formatPath(computer, objectName, instance, counter string) string {
	path := ""
	if instance == emptyInstance {
		path = fmt.Sprintf(`\%s\%s`, objectName, counter)
	} else {
		path = fmt.Sprintf(`\%s(%s)\%s`, objectName, instance, counter)
	}
	if computer != "" && computer != "localhost" {
		path = fmt.Sprintf(`\\%s%s`, computer, path)
	}
	return path
}

// checkError 检查错误是否需要被忽略。
//
// 参数：
//   err error：需要检查的错误对象。
//
// 返回值：
//   error：如果错误需要被忽略返回 nil，否则返回原始错误。
//
// 说明：
//   该函数会检查错误是否为 PDH 错误，如果是且该错误码在 IgnoredErrors 列表中，
//   则忽略该错误并返回 nil。否则返回原始错误。
func (m *WinPerfCounters) checkError(err error) error {
	var pdhErr *pdhError
	if errors.As(err, &pdhErr) {
		if slices.Contains(m.IgnoredErrors, pdhErrors[pdhErr.errorCode]) {
			return nil
		}
		return err
	}
	return err
}

// isKnownCounterDataError 判断错误是否为已知的性能计数器数据错误。
//
// 参数：
//   err error：需要判断的错误对象。
// 返回值：
//   bool：如果是已知的性能计数器数据错误，返回 true，否则返回 false。
func isKnownCounterDataError(err error) bool {
	var pdhErr *pdhError
	if errors.As(err, &pdhErr) && (pdhErr.errorCode == pdhInvalidData ||
		pdhErr.errorCode == pdhCalcNegativeDenominator ||
		pdhErr.errorCode == pdhCalcNegativeValue ||
		pdhErr.errorCode == pdhCstatusInvalidData ||
		pdhErr.errorCode == pdhCstatusNoInstance ||
		pdhErr.errorCode == pdhNoData) {
		return true
	}
	return false
}
