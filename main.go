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
	"net/http"

	"prometheus-edge-hub/hub"

	"github.com/labstack/echo"
)

const (
	defaultPort          = "9091"
	defaultLimit         = -1
	defaultScrapeTimeout = 10 // seconds
)

func main() {
	port := flag.String("port", defaultPort, fmt.Sprintf("Port to listen for requests. Default is %s", defaultPort))
	totalMetricsLimit := flag.Int("limit", defaultLimit, fmt.Sprintf("Limit the total metrics in the hub at one time. Will reject a push if hub is full. Default is %d which is no limit.", defaultLimit))
	scrapeTimeout := flag.Int("scrapeTimeout", defaultScrapeTimeout, fmt.Sprintf("Timeout for scrape calls. Default is %d", defaultScrapeTimeout))
	flag.Parse()

	metricHub := hub.NewMetricHub(*totalMetricsLimit, *scrapeTimeout)
	e := echo.New()

	e.POST("/metrics", metricHub.Receive)
	e.GET("/metrics", metricHub.Scrape)

	e.GET("/debug", metricHub.Debug)

	// For liveness probe
	e.GET("/", func(ctx echo.Context) error { return ctx.NoContent(http.StatusOK) })

	e.Logger.Fatal(e.Start(fmt.Sprintf(":%s", *port)))
}
