package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"log"
	"net/http"
)



func StartMetricsServer(r *prometheus.Registry, metricsServerPort *string) {

	handler := promhttp.HandlerFor(r, promhttp.HandlerOpts{})

	http.Handle("/metrics", handler)
	log.Fatal(http.ListenAndServe(":"  +  *metricsServerPort, nil))

}