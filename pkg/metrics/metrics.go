/*
Copyright 2020 The cert-manager Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package metrics contains global structures related to metrics collection
// cert-manager exposes the following metrics:
// certificate_expiration_timestamp_seconds{name, namespace, issuer_name, issuer_kind, issuer_group}
// certificate_renewal_timestamp_seconds{name, namespace, issuer_name, issuer_kind, issuer_group}
// certificate_ready_status{name, namespace, condition, issuer_name, issuer_kind, issuer_group}
// acme_client_request_count{"scheme", "host", "path", "method", "status"}
// acme_client_request_duration_seconds{"scheme", "host", "path", "method", "status"}
// venafi_client_request_duration_seconds{"scheme", "host", "path", "method", "status"}
// controller_sync_call_count{"controller"}
package metrics

import (
	"net"
	"net/http"
	"time"

	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/utils/clock"
)

const (
	// Namespace is the namespace for cert-manager metric names
	namespace                             = "certmanager"
	prometheusMetricsServerReadTimeout    = 8 * time.Second
	prometheusMetricsServerWriteTimeout   = 8 * time.Second
	prometheusMetricsServerMaxHeaderBytes = 1 << 20 // 1 MiB
)

// Metrics is designed to be a shared object for updating the metrics exposed
// by cert-manager
type Metrics struct {
	log      logr.Logger
	registry *prometheus.Registry

	clockTimeSeconds                   prometheus.CounterFunc
	clockTimeSecondsGauge              prometheus.GaugeFunc
	certificateExpiryTimeSeconds       *prometheus.GaugeVec
	certificateRenewalTimeSeconds      *prometheus.GaugeVec
	certificateReadyStatus             *prometheus.GaugeVec
	acmeClientRequestDurationSeconds   *prometheus.SummaryVec
	acmeClientRequestCount             *prometheus.CounterVec
	venafiClientRequestDurationSeconds *prometheus.SummaryVec
	controllerSyncCallCount            *prometheus.CounterVec
	controllerSyncErrorCount           *prometheus.CounterVec
	certificateRequestCount            *prometheus.GaugeVec
}

var readyConditionStatuses = [...]cmmeta.ConditionStatus{cmmeta.ConditionTrue, cmmeta.ConditionFalse, cmmeta.ConditionUnknown}

// New creates a Metrics struct and populates it with prometheus metric types.
func New(log logr.Logger, c clock.Clock) *Metrics {
	var (
		// Deprecated in favour of clock_time_seconds_gauge.
		clockTimeSeconds = prometheus.NewCounterFunc(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "clock_time_seconds",
				Help:      "DEPRECATED: use clock_time_seconds_gauge instead. The clock time given in seconds (from 1970/01/01 UTC).",
			},
			func() float64 {
				return float64(c.Now().Unix())
			},
		)

		// The clockTimeSeconds metric was first added, however this was
		// erroneously made a "counter" metric type. Time can in fact go backwards,
		// see:
		// - https://github.com/cert-manager/cert-manager/issues/4560
		// - https://www.robustperception.io/are-increasing-timestamps-counters-or-gauges
		// In order to not break users relying on the `clock_time_seconds` metric,
		// a new `clock_time_seconds_gauge` metric of type gauge is added which
		// implements the same thing.
		clockTimeSecondsGauge = prometheus.NewGaugeFunc(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "clock_time_seconds_gauge",
				Help:      "The clock time given in seconds (from 1970/01/01 UTC).",
			},
			func() float64 {
				return float64(c.Now().Unix())
			},
		)

		certificateExpiryTimeSeconds = prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "certificate_expiration_timestamp_seconds",
				Help:      "The date after which the certificate expires. Expressed as a Unix Epoch Time.",
			},
			[]string{"name", "namespace", "issuer_name", "issuer_kind", "issuer_group"},
		)

		certificateRenewalTimeSeconds = prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "certificate_renewal_timestamp_seconds",
				Help:      "The number of seconds before expiration time the certificate should renew.",
			},
			[]string{"name", "namespace", "issuer_name", "issuer_kind", "issuer_group"},
		)

		certificateReadyStatus = prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "certificate_ready_status",
				Help:      "The ready status of the certificate.",
			},
			[]string{"name", "namespace", "condition", "issuer_name", "issuer_kind", "issuer_group"},
		)

		// acmeClientRequestCount is a Prometheus summary to collect the number of
		// requests made to each endpoint with the ACME client.
		acmeClientRequestCount = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "acme_client_request_count",
				Help:      "The number of requests made by the ACME client.",
				Subsystem: "http",
			},
			[]string{"scheme", "host", "path", "method", "status"},
		)

		// acmeClientRequestDurationSeconds is a Prometheus summary to collect request
		// times for the ACME client.
		acmeClientRequestDurationSeconds = prometheus.NewSummaryVec(
			prometheus.SummaryOpts{
				Namespace:  namespace,
				Name:       "acme_client_request_duration_seconds",
				Help:       "The HTTP request latencies in seconds for the ACME client.",
				Subsystem:  "http",
				Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
			},
			[]string{"scheme", "host", "path", "method", "status"},
		)

		// venafiClientRequestDurationSeconds is a Prometheus summary to
		// collect api call latencies for the Venafi client. This
		// metric is in alpha since cert-manager 1.9. Move it to GA once
		// we have seen that it helps to measure Venafi call latency.
		venafiClientRequestDurationSeconds = prometheus.NewSummaryVec(
			prometheus.SummaryOpts{
				Namespace:  namespace,
				Name:       "venafi_client_request_duration_seconds",
				Help:       "ALPHA: The HTTP request latencies in seconds for the Venafi client. This metric is currently alpha as we would like to understand whether it helps to measure Venafi call latency. Please leave feedback if you have any.",
				Subsystem:  "http",
				Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
			},
			[]string{"api_call"},
		)

		controllerSyncCallCount = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "controller_sync_call_count",
				Help:      "The number of sync() calls made by a controller.",
			},
			[]string{"controller"},
		)

		controllerSyncErrorCount = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Name:      "controller_sync_error_count",
				Help:      "The number of errors encountered during controller sync().",
			},
			[]string{"controller"},
		)

		certificateRequestCount = prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "current_certificate_request_count",
				Help: "The current number of certificate requests.",
			},
			[]string{"name", "namespace", "issuer_name", "issuer_kind", "issuer_group"},
		)
	)

	// Create server and register Prometheus metrics handler
	m := &Metrics{
		log:                                log.WithName("metrics"),
		registry:                           prometheus.NewRegistry(),
		certificateRequestCount:            certificateRequestCount,
		clockTimeSeconds:                   clockTimeSeconds,
		clockTimeSecondsGauge:              clockTimeSecondsGauge,
		certificateExpiryTimeSeconds:       certificateExpiryTimeSeconds,
		certificateRenewalTimeSeconds:      certificateRenewalTimeSeconds,
		certificateReadyStatus:             certificateReadyStatus,
		acmeClientRequestCount:             acmeClientRequestCount,
		acmeClientRequestDurationSeconds:   acmeClientRequestDurationSeconds,
		venafiClientRequestDurationSeconds: venafiClientRequestDurationSeconds,
		controllerSyncCallCount:            controllerSyncCallCount,
		controllerSyncErrorCount:           controllerSyncErrorCount,
	}

	return m
}

// NewServer registers Prometheus metrics and returns a new Prometheus metrics HTTP server.
func (m *Metrics) NewServer(ln net.Listener) *http.Server {
	metricsToRegister := []prometheus.Collector{
		m.certificateRequestCount,
		m.clockTimeSeconds,
		m.clockTimeSecondsGauge,
		m.certificateExpiryTimeSeconds,
		m.certificateRenewalTimeSeconds,
		m.certificateReadyStatus,
		m.acmeClientRequestDurationSeconds,
		m.venafiClientRequestDurationSeconds,
		m.acmeClientRequestCount,
		m.controllerSyncCallCount,
		m.controllerSyncErrorCount,
	}

	for _, metric := range metricsToRegister {
		// Check if the metric is already registered
		alreadyRegistered := m.registry.Unregister(metric)
		if !alreadyRegistered {
			// If metric was not already registered, register it
			err := m.registry.Register(metric)
			if err != nil {
				descChan := make(chan *prometheus.Desc, 1)
				metric.Describe(descChan)
				desc := <-descChan
				m.log.Error(err, "Failed to register metric", "metric", desc.String())
			} else {
				descChan := make(chan *prometheus.Desc, 1)
				metric.Describe(descChan)
				desc := <-descChan
				m.log.Info("Registered metric", "name", desc.String())
			}
		} else {
			descChan := make(chan *prometheus.Desc, 1)
			metric.Describe(descChan)
			desc := <-descChan
			m.log.Info("Metric already registered; skipping", "name", desc.String())
		}
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{}))

	server := &http.Server{
		Addr:           ln.Addr().String(),
		ReadTimeout:    prometheusMetricsServerReadTimeout,
		WriteTimeout:   prometheusMetricsServerWriteTimeout,
		MaxHeaderBytes: prometheusMetricsServerMaxHeaderBytes,
		Handler:        mux,
	}

	return server
}

// IncrementSyncCallCount will increase the sync counter for that controller.
func (m *Metrics) IncrementSyncCallCount(controllerName string) {
	m.controllerSyncCallCount.WithLabelValues(controllerName).Inc()
}

// IncrementSyncErrorCount will increase count of errors during sync of that controller.
func (m *Metrics) IncrementSyncErrorCount(controllerName string) {
	m.controllerSyncErrorCount.WithLabelValues(controllerName).Inc()
}

func (m *Metrics) IncrementCurrentCertificateRequest(name, namespace, issuerName, issuerKind, issuerGroup string) {
	m.log.Info("Incrementing certificateRequestCount", "name", name, "namespace", namespace, "issuerName", issuerName, "issuerKind", issuerKind, "issuerGroup", issuerGroup)
	m.certificateRequestCount.WithLabelValues(name, namespace, issuerName, issuerKind, issuerGroup).Inc()
}

func (m *Metrics) DecrementCurrentCertificateRequest(name, namespace, issuerName, issuerKind, issuerGroup string) {
	m.log.Info("Decrementing certificateRequestCount", "name", name, "namespace", namespace, "issuerName", issuerName, "issuerKind", issuerKind, "issuerGroup", issuerGroup)
	m.certificateRequestCount.WithLabelValues(name, namespace, issuerName, issuerKind, issuerGroup).Dec()
}
