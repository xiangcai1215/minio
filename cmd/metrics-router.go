// Copyright (c) 2015-2021 MinIO, Inc.
//
// This file is part of MinIO Object Storage stack
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package cmd

import (
	"strings"

	"github.com/minio/mux"
	"github.com/minio/pkg/v2/env"
)

const (
	prometheusMetricsPathLegacy     = "/prometheus/metrics"
	prometheusMetricsV2ClusterPath  = "/v2/metrics/cluster"
	prometheusMetricsV2BucketPath   = "/v2/metrics/bucket"
	prometheusMetricsV2NodePath     = "/v2/metrics/node"
	prometheusMetricsV2ResourcePath = "/v2/metrics/resource"
)

// Standard env prometheus auth type
const (
	EnvPrometheusAuthType = "MINIO_PROMETHEUS_AUTH_TYPE"
)

type prometheusAuthType string

const (
	prometheusJWT    prometheusAuthType = "jwt"
	prometheusPublic prometheusAuthType = "public"
)

// registerMetricsRouter - add handler functions for metrics.
func registerMetricsRouter(router *mux.Router) {
	// metrics router
	metricsRouter := router.NewRoute().PathPrefix(minioReservedBucketPath).Subrouter()
	authType := strings.ToLower(env.Get(EnvPrometheusAuthType, string(prometheusJWT)))

	auth := AuthMiddleware
	if prometheusAuthType(authType) == prometheusPublic {
		auth = NoAuthMiddleware
	}
	//minio/prometheus/metrics 这个数据以及被废弃了，现在应该用的v2版本了
	metricsRouter.Handle(prometheusMetricsPathLegacy, auth(metricsHandler()))

	metricsRouter.Handle(prometheusMetricsV2ClusterPath, auth(metricsServerHandler()))
	metricsRouter.Handle(prometheusMetricsV2BucketPath, auth(metricsBucketHandler()))
	metricsRouter.Handle(prometheusMetricsV2NodePath, auth(metricsNodeHandler()))
	metricsRouter.Handle(prometheusMetricsV2ResourcePath, auth(metricsResourceHandler()))
}
