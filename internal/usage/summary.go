package usage

import (
	"sort"
	"time"
)

type Range int

const (
	RangeAllTime Range = iota
	RangeLast1Day
	RangeLast7Days
	RangeLast30Days
	RangeLast6Months
	RangeLast1Year
)

// Ranges lists the reporting windows in display order.
var Ranges = []Range{
	RangeAllTime,
	RangeLast1Day,
	RangeLast7Days,
	RangeLast30Days,
	RangeLast6Months,
	RangeLast1Year,
}

func (r Range) String() string {
	switch r {
	case RangeLast1Day:
		return "Last 1 day"
	case RangeLast7Days:
		return "Last 7 days"
	case RangeLast30Days:
		return "Last 30 days"
	case RangeLast6Months:
		return "Last 6 months"
	case RangeLast1Year:
		return "Last 1 year"
	default:
		return "All time"
	}
}

func rangeSince(r Range, now time.Time) time.Time {
	switch r {
	case RangeLast1Day:
		return now.AddDate(0, 0, -1)
	case RangeLast7Days:
		return now.AddDate(0, 0, -7)
	case RangeLast30Days:
		return now.AddDate(0, 0, -30)
	case RangeLast6Months:
		return now.AddDate(0, -6, 0)
	case RangeLast1Year:
		return now.AddDate(-1, 0, 0)
	default:
		return time.Time{}
	}
}

func (r Range) Since(now time.Time) time.Time {
	return rangeSince(r, now)
}

type ModelUsage struct {
	Provider, Model                                 string
	Input, Output, CacheRead, CacheWrite, Reasoning int
}

type Summary struct {
	Rows  []ModelUsage
	Total ModelUsage
}

// Summarize groups records by provider/model, filters by since
// (zero means all time), sorts rows by input descending, and totals them.
func Summarize(records []Record, since time.Time) Summary {
	byKey := make(map[[2]string]*ModelUsage)
	var order [][2]string
	for _, rec := range records {
		ts, ok := recordTime(rec)
		if !ok {
			continue
		}
		if !since.IsZero() && ts.Before(since) {
			continue
		}
		key := [2]string{rec.Provider, rec.Model}
		row, exists := byKey[key]
		if !exists {
			row = &ModelUsage{Provider: rec.Provider, Model: rec.Model}
			byKey[key] = row
			order = append(order, key)
		}
		row.Input += rec.Input
		row.Output += rec.Output
		row.CacheRead += rec.CacheRead
		row.CacheWrite += rec.CacheWrite
		row.Reasoning += rec.Reasoning
	}
	rows := make([]ModelUsage, 0, len(byKey))
	var total ModelUsage
	for _, key := range order {
		row := *byKey[key]
		rows = append(rows, row)
		total.Input += row.Input
		total.Output += row.Output
		total.CacheRead += row.CacheRead
		total.CacheWrite += row.CacheWrite
		total.Reasoning += row.Reasoning
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Input != rows[j].Input {
			return rows[i].Input > rows[j].Input
		}
		if rows[i].Provider != rows[j].Provider {
			return rows[i].Provider < rows[j].Provider
		}
		return rows[i].Model < rows[j].Model
	})
	return Summary{Rows: rows, Total: total}
}

func recordTime(rec Record) (time.Time, bool) {
	if rec.Rollup || rec.Date != "" {
		if rec.Date == "" {
			return time.Time{}, false
		}
		day, err := time.Parse(dateLayout, rec.Date)
		if err != nil {
			return time.Time{}, false
		}
		return day, true
	}
	if rec.TS.IsZero() {
		return time.Time{}, false
	}
	return rec.TS, true
}
