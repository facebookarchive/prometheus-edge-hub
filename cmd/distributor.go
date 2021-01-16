package cmd

import (
	"fmt"
	"log"
	"net"

	"github.com/facebookincubator/prometheus-edge-hub/distributor"
	hubGrpc "github.com/facebookincubator/prometheus-edge-hub/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func RunDistributor(port, grpcPort, maxGRPCMsgSizeBytes int, distributionKeyLabel string, edgeHubArray []string) {
	dist := distributor.NewDistributor(distributionKeyLabel, edgeHubArray)

	fmt.Printf("Grpc Port: %d\n", grpcPort)

	if grpcPort != 0 {
		log.Fatal(serveDistGRPC(grpcPort, maxGRPCMsgSizeBytes, dist))
	}
}

func serveDistGRPC(port, maxMsgSize int, dist *distributor.Distributor) error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	metricsGrpcServer := distributor.MetricsControllerServerImpl{Dist: dist}
	grpcServer := grpc.NewServer(grpc.MaxRecvMsgSize(maxMsgSize))
	reflection.Register(grpcServer)
	hubGrpc.RegisterMetricsControllerServer(grpcServer, &metricsGrpcServer)

	log.Printf("Serving GRPC on: %d\n", port)

	return grpcServer.Serve(lis)
}
