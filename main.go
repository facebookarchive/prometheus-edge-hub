/*
 * Copyright (c) Facebook, Inc. and its affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

package main

import (
	"flag"
	"fmt"
	"google.golang.org/grpc"
	"log"
	"net"
	"net/http"

	hubgrpc "github.com/facebookincubator/prometheus-edge-hub/grpc"
	"github.com/facebookincubator/prometheus-edge-hub/hub"
	"github.com/labstack/echo"
)

const (
	defaultPort                = 9091
	defaultGRPCPort            = 0
	defaultLimit               = -1
	defaultScrapeTimeout       = 10                 // seconds
	defaultMaxGRPCMsgSizeBytes = 1024 * 1024 * 1024 //1 GB
)

func main() {
	port := flag.Int("port", defaultPort, fmt.Sprintf("Port to listen for requests. Default is %d", defaultPort))
	totalMetricsLimit := flag.Int("limit", defaultLimit, fmt.Sprintf("Limit the total metrics in the hub at one time. Will reject a push if hub is full. Default is %d which is no limit.", defaultLimit))
	scrapeTimeout := flag.Int("scrapeTimeout", defaultScrapeTimeout, fmt.Sprintf("Timeout for scrape calls. Default is %d", defaultScrapeTimeout))
	grpcPort := flag.Int("grpc-port", defaultGRPCPort, fmt.Sprintf("Port to listen for GRPC requests"))
	grpcMaxGRPCMsgSizeBytes := flag.Int("grpc-max-msg-size", defaultMaxGRPCMsgSizeBytes, fmt.Sprintf("Max message size (bytes) for GRPC receives"))
	flag.Parse()

	metricHub := hub.NewMetricHub(*totalMetricsLimit, *scrapeTimeout)
	e := echo.New()

	e.POST("/metrics", metricHub.Receive)
	e.GET("/metrics", metricHub.Scrape)

	e.GET("/debug", metricHub.Debug)

	// For liveness probe
	e.GET("/", func(ctx echo.Context) error { return ctx.NoContent(http.StatusOK) })

	e.GET("/internal", serveInternalMetrics)

	if *grpcPort != 0 {
		go func() {
			log.Fatal(serveGRPC(*grpcPort, *grpcMaxGRPCMsgSizeBytes, metricHub))
		}()
	}

	go e.Logger.Fatal(e.Start(fmt.Sprintf(":%d", *port)))
}

func serveInternalMetrics(ctx echo.Context) error {
	text, err := hub.WriteInternalMetrics()
	if err != nil {
		return ctx.NoContent(http.StatusInternalServerError)
	}
	return ctx.String(http.StatusOK, text)
}

func serveGRPC(port, maxMsgSize int, metricHub *hub.MetricHub) error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	metricsGrpcServer := hubgrpc.MetricsControllerServerImpl{MetricHub: metricHub}
	grpcServer := grpc.NewServer(grpc.MaxRecvMsgSize(maxMsgSize))
	hubgrpc.RegisterMetricsControllerServer(grpcServer, &metricsGrpcServer)

	log.Printf("Serving GRPC on: %d\n", port)

	return grpcServer.Serve(lis)
}
