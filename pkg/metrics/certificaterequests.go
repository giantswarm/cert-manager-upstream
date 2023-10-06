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

package metrics

import (
	"context"
	"log"

	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"k8s.io/client-go/tools/cache"
)

var (
	currentCertificateRequestCount = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "current_certificate_request_count",
			Help: "The current number of certificate requests.",
		},
		[]string{"name", "namespace", "issuer_name", "issuer_kind", "issuer_group"},
	)
)

// UpdateCurrentCertificateRequestCount updates the current number of CertificateRequests
func (m *Metrics) UpdateCurrentCertificateRequestCount(ctx context.Context, crs []cmapi.CertificateRequest) {
	for _, cr := range crs {
		labels := prometheus.Labels{
			"name":         cr.Name,
			"namespace":    cr.Namespace,
			"issuer_name":  cr.Spec.IssuerRef.Name,
			"issuer_kind":  cr.Spec.IssuerRef.Kind,
			"issuer_group": cr.Spec.IssuerRef.Group,
		}
		currentCertificateRequestCount.With(labels).Inc()
	}
}

// getCurrentCertificateRequests fetches the current list of CertificateRequests
func (m *Metrics) getCurrentCertificateRequests(ctx context.Context) ([]cmapi.CertificateRequest, error) {
	crsList := cmapi.CertificateRequestList{}
	err := m.client.List(ctx, &crsList)
	if err != nil {
		return nil, err
	}
	return crsList.Items, nil
}

// This function can be triggered whenever there's a change in the number of CertificateRequests
func (m *Metrics) HandleCertificateRequestEvent(ctx context.Context, cr *cmapi.CertificateRequest, event cache.ResourceEventHandler) {
	crs, err := m.getCurrentCertificateRequests(ctx)
	if err != nil {
		log.Println("Error fetching CertificateRequests:", err)
		return
	}
	m.UpdateCurrentCertificateRequestCount(ctx, crs)
}
