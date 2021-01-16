/*
 * Copyright (c) Facebook, Inc. and its affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

package hub

import (
	"context"

	"github.com/facebookincubator/prometheus-edge-hub/grpc"
)

type MetricControllerServerImpl struct {
	Hub *MetricHub
}

func (h *MetricControllerServerImpl) Collect(ctx context.Context, req *grpc.MetricFamilies) (*grpc.Void, error) {
	h.Hub.ReceiveGRPC(req.GetFamilies())
	return &grpc.Void{}, nil
}
