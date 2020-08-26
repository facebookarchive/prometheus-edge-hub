/*
 * Copyright (c) Facebook, Inc. and its affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

//go:generate bash -c "protoc --proto_path=. --go_out=plugins=grpc:. service.proto"
package grpc
