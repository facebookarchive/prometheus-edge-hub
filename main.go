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
	"os"

	"github.com/facebookincubator/prometheus-edge-hub/cmd"
)

const (
	defaultPort                = 9091
	defaultGRPCPort            = 0
	defaultLimit               = -1
	defaultScrapeTimeout       = 10                 // seconds
	defaultMaxGRPCMsgSizeBytes = 1024 * 1024 * 1024 //1 GB
)

func main() {
	flag.Parse()
	// Subcommands
	hubCommand := flag.NewFlagSet("hub", flag.PanicOnError)
	distCommand := flag.NewFlagSet("distributor", flag.PanicOnError)

	hubPort := hubCommand.Int("port", defaultPort, fmt.Sprintf("Port to listen for requests. Default is %d", defaultPort))
	hubGrpcPort := hubCommand.Int("grpc-port", defaultGRPCPort, fmt.Sprintf("Port to listen for GRPC requests"))
	hubMaxGRPCMsgSizeBytes := hubCommand.Int("grpc-max-msg-size", defaultMaxGRPCMsgSizeBytes, fmt.Sprintf("Max message size (bytes) for GRPC receives"))
	totalMetricsLimit := hubCommand.Int("limit", defaultLimit, fmt.Sprintf("Limit the total metrics in the hub at one time. Will reject a push if hub is full. Default is %d which is no limit.", defaultLimit))
	scrapeTimeout := hubCommand.Int("scrapeTimeout", defaultScrapeTimeout, fmt.Sprintf("Timeout for scrape calls. Default is %d", defaultScrapeTimeout))

	// distributor flags
	distPort := distCommand.Int("port", defaultPort, fmt.Sprintf("Port to listen for requests. Default is %d", defaultPort))
	distGrpcPort := distCommand.Int("grpc-port", defaultGRPCPort, fmt.Sprintf("Port to listen for GRPC requests"))
	distMaxGRPCMsgSizeBytes := distCommand.Int("grpc-max-msg-size", defaultMaxGRPCMsgSizeBytes, fmt.Sprintf("Max message size (bytes) for GRPC receives"))
	distributionKeyLabel := distCommand.String("key-label", "", "Label used to determine where to distribute metrics.")
	var edgeHubArray hubArray
	distCommand.Var(&edgeHubArray, "edge-hub", "Repeatable address of edge-hub to distribute metrics to.")

	switch os.Args[1] {
	case "hub":
		hubCommand.Parse(os.Args[2:])
		cmd.RunHub(*hubPort, *hubGrpcPort, *hubMaxGRPCMsgSizeBytes, *totalMetricsLimit, *scrapeTimeout)
	case "distributor":
		fmt.Printf("Parsed EdgeHub Array: %v\n", edgeHubArray)
		distCommand.Parse(os.Args[2:])
		cmd.RunDistributor(*distPort, *distGrpcPort, *distMaxGRPCMsgSizeBytes, *distributionKeyLabel, edgeHubArray)
	default:
		fmt.Println("usage: (hub|distributor) <flags>")
		os.Exit(1)
	}
}

type hubArray []string

func (a *hubArray) String() string {
	return "test"
}

func (a *hubArray) Set(value string) error {
	*a = append(*a, value)
	return nil
}


