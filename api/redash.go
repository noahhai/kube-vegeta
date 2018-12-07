package main

import (
	"encoding/json"
	"math"
	"strings"
	"time"

	vegeta "github.com/tsenart/vegeta/lib"
)

type column struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	FriendlyName string `json:"friendly_name"`
}

type row struct {
	Total    float32 `json:"total"`
	Mean     float32 `json:"mean"`
	P50th    float32 `json:"p50th"`
	P95th    float32 `json:"p95th"`
	P99th    float32 `json:"p99th"`
	Max      float32 `json:"max"`
	Requests uint64  `json:"requests"`
	Rate     float32 `json:"rate"`
	//Date        time.Time `json:"date"`
	Duration    float32 `json:"duration"`
	Success     bool    `json:"success"`
	StatusCodes string  `json:"statusCodes"`
	Errors      string  `json:"errors"`
}

type redashData struct {
	Columns []column `json:"columns"`
	Rows    []row    `json:"rows"`
}

func vegetaResultsToRedash(metrics []vegeta.Metrics) *redashData {
	var requests uint64
	var mean, p50, p95, p99, max, total, duration, rate float64
	success := true
	var err string
	var statusCodesBytes []byte
	statusCodes := map[string]int{}
	var date *time.Time
	for _, m := range metrics {
		requests += m.Requests
		if m.Success != 1.0 {
			success = false
		}
		if date == nil {
			date = &m.Earliest
		}
		if duration == 0 {
			duration = m.Duration.Seconds()
		}
		total += m.Latencies.Total.Seconds() * 1000.0
		fractionThis := float64(m.Requests) / float64(requests)
		fractionRest := 1.0 - fractionThis
		mean = fractionThis*m.Latencies.Mean.Seconds()*1000.0 + fractionRest*mean
		p50 = fractionThis*m.Latencies.P50.Seconds()*1000.0 + fractionRest*p50
		p95 = fractionThis*m.Latencies.P95.Seconds()*1000.0 + fractionRest*p95
		p99 = fractionThis*m.Latencies.P99.Seconds()*1000.0 + fractionRest*p99
		max = math.Max(fractionThis*m.Latencies.Max.Seconds()*1000.0, max)

		rate += m.Rate

		if len(m.Errors) > 0 {
			err = err + ", " + strings.Join(m.Errors, ",")
		}
		for k, v := range m.StatusCodes {
			statusCodes[k] += v
		}
	}

	statusCodesBytes, _ = json.Marshal(statusCodes)

	return &redashData{
		Rows: []row{
			row{
				Total:    float32(total),
				Mean:     float32(mean),
				P50th:    float32(p50),
				P95th:    float32(p95),
				P99th:    float32(p99),
				Max:      float32(max),
				Requests: requests,
				//Date:        *date,
				Rate:        float32(rate),
				Duration:    float32(duration),
				Success:     success,
				StatusCodes: string(statusCodesBytes),
				Errors:      err,
			},
		},
		Columns: []column{
			column{
				Name:         "total",
				Type:         "float",
				FriendlyName: "total",
			},
			column{
				Name:         "mean",
				Type:         "float",
				FriendlyName: "mean",
			},
			column{
				Name:         "p50th",
				Type:         "float",
				FriendlyName: "p50th",
			},
			column{
				Name:         "p95th",
				Type:         "float",
				FriendlyName: "p95th",
			},
			column{
				Name:         "p99th",
				Type:         "float",
				FriendlyName: "p99th",
			},
			column{
				Name:         "max",
				Type:         "float",
				FriendlyName: "max",
			},
			column{
				Name:         "requests",
				Type:         "integer",
				FriendlyName: "requests",
			},
			column{
				Name:         "duration",
				Type:         "integer",
				FriendlyName: "duration",
			},
			// column{
			// 	Name: "date",
			// 	Type: "datetime",
			// 	FriendlyName: "date",
			// },
			column{
				Name:         "rate",
				Type:         "float",
				FriendlyName: "rate",
			},
			column{
				Name:         "success",
				Type:         "bool",
				FriendlyName: "success",
			},
			column{
				Name:         "statusCodes",
				Type:         "string",
				FriendlyName: "statusCodes",
			},
			column{
				Name:         "errors",
				Type:         "string",
				FriendlyName: "errors",
			},
		},
	}
}
