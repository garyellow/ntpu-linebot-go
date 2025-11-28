// Package lineutil provides utility functions for building LINE messages and actions.
package lineutil

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// PeriodTime represents a class period with start and end times.
type PeriodTime struct {
	Period    int    // Period number (1-13)
	StartTime string // Start time in HH:MM format
	EndTime   string // End time in HH:MM format
}

// periodTimes maps period numbers to their time ranges.
// Based on NTPU's official class schedule:
// https://sea.cc.ntpu.edu.tw/pls/dev_stud/course_query_all.time_memo
var periodTimes = map[int]PeriodTime{
	1:  {Period: 1, StartTime: "08:10", EndTime: "09:00"},
	2:  {Period: 2, StartTime: "09:10", EndTime: "10:00"},
	3:  {Period: 3, StartTime: "10:10", EndTime: "11:00"},
	4:  {Period: 4, StartTime: "11:10", EndTime: "12:00"},
	5:  {Period: 5, StartTime: "13:10", EndTime: "14:00"},
	6:  {Period: 6, StartTime: "14:10", EndTime: "15:00"},
	7:  {Period: 7, StartTime: "15:10", EndTime: "16:00"},
	8:  {Period: 8, StartTime: "16:10", EndTime: "17:00"},
	9:  {Period: 9, StartTime: "17:10", EndTime: "18:00"},
	10: {Period: 10, StartTime: "18:30", EndTime: "19:20"},
	11: {Period: 11, StartTime: "19:25", EndTime: "20:15"},
	12: {Period: 12, StartTime: "20:25", EndTime: "21:15"},
	13: {Period: 13, StartTime: "21:20", EndTime: "22:10"},
}

// periodRegex matches patterns like "5~6", "1~2", "10~11" (period ranges)
var periodRegex = regexp.MustCompile(`(\d{1,2})~(\d{1,2})`)

// FormatCourseTime converts a course time string to include actual times.
// Input format: "每週一5~6" or "每週三10~11"
// Output format: "每週一 5~6 (13:10-15:00)" or "每週三 10~11 (18:30-20:15)"
//
// If the input doesn't match the expected pattern, returns the original string.
func FormatCourseTime(timeStr string) string {
	if timeStr == "" {
		return timeStr
	}

	// Find period range in the string
	matches := periodRegex.FindStringSubmatch(timeStr)
	if len(matches) != 3 {
		return timeStr
	}

	startPeriod, err1 := strconv.Atoi(matches[1])
	endPeriod, err2 := strconv.Atoi(matches[2])
	if err1 != nil || err2 != nil {
		return timeStr
	}

	// Get time range
	startTime, ok1 := periodTimes[startPeriod]
	endTime, ok2 := periodTimes[endPeriod]
	if !ok1 || !ok2 {
		return timeStr
	}

	// Build the result string
	// Replace the period range with "period range (start-end)"
	// e.g., "每週一5~6" -> "每週一 5~6 (13:10-15:00)"
	periodPart := matches[0]
	timePart := fmt.Sprintf("%s-%s", startTime.StartTime, endTime.EndTime)

	// Find position of period range and insert formatted time
	periodIdx := strings.Index(timeStr, periodPart)
	if periodIdx == -1 {
		return timeStr
	}

	// Build result: prefix + space + period + space + (time)
	prefix := strings.TrimSpace(timeStr[:periodIdx])
	suffix := strings.TrimSpace(timeStr[periodIdx+len(periodPart):])

	result := prefix
	if result != "" {
		result += " "
	}
	result += periodPart + " (" + timePart + ")"
	if suffix != "" {
		result += " " + suffix
	}

	return result
}

// FormatCourseTimes converts multiple course time strings to include actual times.
// Uses FormatCourseTime for each time string in the slice.
func FormatCourseTimes(times []string) []string {
	if len(times) == 0 {
		return times
	}

	result := make([]string, len(times))
	for i, t := range times {
		result[i] = FormatCourseTime(t)
	}
	return result
}

// GetPeriodTime returns the time range for a specific period number.
// Returns empty strings if the period is invalid.
func GetPeriodTime(period int) (startTime, endTime string) {
	if pt, ok := periodTimes[period]; ok {
		return pt.StartTime, pt.EndTime
	}
	return "", ""
}

// GetPeriodRangeTime returns the time range for a period range (e.g., 5~6).
// Returns the start time of the first period and end time of the last period.
func GetPeriodRangeTime(startPeriod, endPeriod int) (startTime, endTime string) {
	if start, ok := periodTimes[startPeriod]; ok {
		if end, ok := periodTimes[endPeriod]; ok {
			return start.StartTime, end.EndTime
		}
	}
	return "", ""
}
