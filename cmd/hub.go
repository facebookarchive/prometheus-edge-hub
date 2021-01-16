package cmd

import (
	"fmt"
	"log"
	"net"
	"net/http"

	"github.com/facebookincubator/prometheus-edge-hub/hub"
	hubGrpc "github.com/facebookincubator/prometheus-edge-hub/grpc"
	"github.com/labstack/echo"
	"google.golang.org/grpc"
)

func RunHub(port, grpcPort, maxGRPCMsgSizeBytes int, totalMetricsLimit int, scrapeTimeout int) {
	metricHub := hub.NewMetricHub(totalMetricsLimit, scrapeTimeout)
	e := echo.New()

	e.POST("/metrics", metricHub.Receive)
	e.GET("/metrics", metricHub.Scrape)

	e.GET("/debug", metricHub.Debug)

	// For liveness probe
	e.GET("/", func(ctx echo.Context) error { return ctx.NoContent(http.StatusOK) })

	e.GET("/internal", serveInternalMetrics)

	if grpcPort != 0 {
		go func() {
			log.Fatal(serveHubGRPC(grpcPort, maxGRPCMsgSizeBytes, metricHub))
		}()
	}

	go e.Logger.Fatal(e.Start(fmt.Sprintf(":%d", port)))
}

func serveInternalMetrics(ctx echo.Context) error {
	text, err := hub.WriteInternalMetrics()
	if err != nil {
		return ctx.NoContent(http.StatusInternalServerError)
	}
	return ctx.String(http.StatusOK, text)
}

func serveHubGRPC(port, maxMsgSize int, metricHub *hub.MetricHub) error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	metricsGrpcServer := hub.MetricControllerServerImpl{Hub: metricHub}
	grpcServer := grpc.NewServer(grpc.MaxRecvMsgSize(maxMsgSize))
	hubGrpc.RegisterMetricsControllerServer(grpcServer, &metricsGrpcServer)

	log.Printf("Serving GRPC on: %d\n", port)

	return grpcServer.Serve(lis)
}
