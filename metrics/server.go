package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"log"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)



func StartMetricsServer(r *prometheus.Registry, metricsServerPort *string) {

	handler := promhttp.HandlerFor(r, promhttp.HandlerOpts{})

	http.Handle("/metrics", handler)
	log.Fatal(http.ListenAndServe(":"  +  *metricsServerPort, nil))

}