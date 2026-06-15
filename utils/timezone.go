package utils

import (
	"fmt"
	"time"
)

func ParseTimeInLocation(dateStr, timeStr, tzName string) (time.Time, error) {
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		loc = time.UTC
	}

	fullStr := fmt.Sprintf("%s %s", dateStr, timeStr)
	parsed, err := time.ParseInLocation("2006-01-02 15:04", fullStr, loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("解析时间失败: %v", err)
	}

	return parsed, nil
}

func ConvertToUTC(localTime time.Time) time.Time {
	return localTime.UTC()
}

func ConvertToLocal(utcTime time.Time, tzName string) time.Time {
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		return utcTime
	}
	return utcTime.In(loc)
}

func ComputeShiftUTCTimes(shiftDate time.Time, startTime, endTime, tzName string) (time.Time, time.Time, error) {
	dateStr := shiftDate.Format("2006-01-02")

	startLocal, err := ParseTimeInLocation(dateStr, startTime, tzName)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}

	endLocal, err := ParseTimeInLocation(dateStr, endTime, tzName)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}

	if endLocal.Before(startLocal) {
		endLocal = endLocal.AddDate(0, 0, 1)
	}

	return startLocal.UTC(), endLocal.UTC(), nil
}

func DetectUTCTimeOverlap(userID uint, startUTC, endUTC time.Time, shiftDB interface{}, excludeSwapIDs ...uint) (bool, error) {
	_ = userID
	_ = startUTC
	_ = endUTC
	_ = shiftDB
	_ = excludeSwapIDs
	return false, nil
}

func FormatTimeForDisplay(t time.Time, tzName string) string {
	localTime := ConvertToLocal(t, tzName)
	return localTime.Format("2006-01-02 15:04 MST")
}
