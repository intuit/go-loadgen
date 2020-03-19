package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)


func GetTotalBytesProcessedCounter() (promTotalBytesProcessedCounter prometheus.Counter){
	promTotalBytesProcessedCounter = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "total_bytes_processed",
			Help: "Total bytes processed",
		},
	)
	return
}

func GetEventsProcessedCounter() (promCounter prometheus.Counter){
	promCounter = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "total_events_processed",
			Help: "Total log events generated by the tool.",
		},
	)
	return
}