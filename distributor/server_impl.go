/*
 * Copyright (c) Facebook, Inc. and its affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

package distributor

import (
	"context"

	"github.com/facebookincubator/prometheus-edge-hub/grpc"
)

type MetricsControllerServerImpl struct {
	Dist *Distributor
}

func (d *MetricsControllerServerImpl) Collect(ctx context.Context, req *grpc.MetricFamilies) (*grpc.Void, error) {
	d.Dist.ReceiveGRPC(req.GetFamilies())
	return &grpc.Void{}, nil
}
