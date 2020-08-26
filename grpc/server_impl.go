/*
 * Copyright (c) Facebook, Inc. and its affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

package grpc

import (
	"context"

	"github.com/facebookincubator/prometheus-edge-hub/hub"
)

type MetricsControllerServerImpl struct {
	MetricHub *hub.MetricHub
}

func (m *MetricsControllerServerImpl) Collect(ctx context.Context, req *MetricFamilies) (*Void, error) {
	m.MetricHub.ReceiveGRPC(req.GetFamilies())
	return &Void{}, nil
}
