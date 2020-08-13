/*
 * Copyright (c) Facebook, Inc. and its affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

package hub

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/labstack/echo"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
)

const (
	internalMetricHubSize  = "hub_size"
	internalMetricHubLimit = "hub_limit"
	scrapeWorkerPoolSize   = 100
)

var (
	hubLimit = prometheus.NewGauge(prometheus.GaugeOpts{Name: internalMetricHubLimit, Help: "Maximum number of datapoints in hub"})
	hubSize  = prometheus.NewGauge(prometheus.GaugeOpts{Name: internalMetricHubSize, Help: "Number of datapoints in hub"})
)

func init() {
	prometheus.MustRegister(hubLimit, hubSize)
}

// MetricHub serves as a replacement for the prometheus pushgateway. Accepts
// timestamps with metrics, and stores them in a queue to allow multiple
// datapoints per metric series to be scraped
type MetricHub struct {
	metricFamiliesByName map[string]*familyAndMetrics
	limit                int
	stats                hubStats
	sync.Mutex
	scrapeTimeout int
}

type hubStats struct {
	lastScrapeTime        int64
	lastScrapeSize        int64
	lastScrapeNumFamilies int

	lastReceiveTime        int64
	lastReceiveSize        int64
	lastReceiveNumFamilies int

	currentCountFamilies   int
	currentCountSeries     int
	currentCountDatapoints int
}

func NewMetricHub(limit int, scrapeTimeout int) *MetricHub {
	if limit > 0 {
		glog.Infof("Prometheus-Edge-Hub created with a limit of %d\n", limit)
	} else {
		glog.Info("Prometheus-Edge-Hub created with no limit\n")
	}

	hubLimit.Set(float64(limit))

	return &MetricHub{
		metricFamiliesByName: make(map[string]*familyAndMetrics),
		limit:                limit,
		scrapeTimeout:        scrapeTimeout,
	}
}

// Receive is a handler function to receive metric pushes
func (c *MetricHub) Receive(ctx echo.Context) error {
	var (
		err    error
		parser expfmt.TextParser
	)

	parsedFamilies, err := parser.TextToMetricFamilies(ctx.Request().Body)
	if err != nil {
		return ctx.String(http.StatusBadRequest, fmt.Sprintf("error parsing metrics: %v", err))
	}

	newDatapoints := 0
	for _, fam := range parsedFamilies {
		newDatapoints += len(fam.Metric)
	}

	// Check if new datapoints will exceed the specified limit
	if c.limit > 0 {
		if c.stats.currentCountDatapoints+newDatapoints > c.limit {
			errString := fmt.Sprintf("Not accepting push of size %d. Would overfill hub limit of %d. Current hub size: %d\n", newDatapoints, c.limit, c.stats.currentCountDatapoints)
			glog.Error(errString)
			return ctx.String(http.StatusNotAcceptable, errString)
		}
	}

	c.hubMetrics(parsedFamilies)

	c.stats.lastReceiveTime = time.Now().Unix()
	c.stats.lastReceiveSize = ctx.Request().ContentLength
	c.stats.lastReceiveNumFamilies = len(parsedFamilies)
	c.stats.currentCountDatapoints += newDatapoints
	hubSize.Set(float64(c.stats.currentCountDatapoints))

	return ctx.NoContent(http.StatusOK)
}

func (c *MetricHub) hubMetrics(families map[string]*dto.MetricFamily) {
	c.Lock()
	defer c.Unlock()
	for _, fam := range families {
		if families, ok := c.metricFamiliesByName[fam.GetName()]; ok {
			families.addMetrics(fam.Metric)
		} else {
			c.metricFamiliesByName[fam.GetName()] = newFamilyAndMetrics(fam)
		}
	}
}

// Scrape is a handler function for prometheus scrape requests. Formats the
// metrics for scraping.
func (c *MetricHub) Scrape(ctx echo.Context) error {
	c.Lock()
	scrapeMetrics := c.metricFamiliesByName
	c.clearMetrics()
	c.Unlock()

	expositionString := c.exposeMetrics(scrapeMetrics, scrapeWorkerPoolSize)

	c.stats.lastScrapeTime = time.Now().Unix()
	c.stats.lastScrapeSize = int64(len(expositionString))
	c.stats.lastScrapeNumFamilies = len(scrapeMetrics)
	c.stats.currentCountDatapoints = 0
	hubSize.Set(0)

	return ctx.String(http.StatusOK, expositionString)
}

func (c *MetricHub) clearMetrics() {
	c.metricFamiliesByName = make(map[string]*familyAndMetrics)
}

func (c *MetricHub) exposeMetrics(metricFamiliesByName map[string]*familyAndMetrics, workers int) string {
	fams := make(chan *familyAndMetrics, workers)
	results := make(chan string, workers)
	respCh := make(chan string, 1)

	waitGroup := &sync.WaitGroup{}

	for i := 0; i < workers; i++ {
		waitGroup.Add(1)
		go processFamilyWorker(fams, results, waitGroup)
	}

	go processFamilyStringsWorker(results, respCh)

	for _, fam := range metricFamiliesByName {
		fams <- fam
	}

	close(fams)
	waitGroup.Wait()
	close(results)

	select {
	case resp := <-respCh:
		return resp
	case <-time.After(time.Duration(c.scrapeTimeout) * time.Second):
		log.Print("Timeout reached for building metrics string")
		return ""
	}
}

func processFamilyWorker(fams <-chan *familyAndMetrics, results chan<- string, waitGroup *sync.WaitGroup) {
	defer waitGroup.Done()
	for fam := range fams {
		pullFamily := fam.popDatapoints()
		familyStr, err := familyToString(pullFamily)
		if err != nil {
			log.Printf("metric %s dropped. error converting metric to string: %v", *pullFamily.Name, err)
		} else {
			results <- familyStr
		}
	}
}

func processFamilyStringsWorker(results <-chan string, respCh chan<- string) {
	var resp strings.Builder
	for result := range results {
		resp.WriteString(result)
	}
	respCh <- resp.String()
}

// Debug is a handler function to show the current state of the hub without
// consuming any datapoints
func (c *MetricHub) Debug(ctx echo.Context) error {
	verbose := ctx.QueryParam("verbose")

	c.updateCountStats()
	hostname, _ := os.Hostname()
	var limitValue, utilizationValue string
	if c.limit <= 0 {
		limitValue = "None"
		utilizationValue = "0"
	} else {
		limitValue = strconv.Itoa(c.limit)
		utilizationValue = strconv.FormatFloat(float64(c.stats.currentCountDatapoints)*100/float64(c.limit), 'f', 2, 64)
	}

	debugString := fmt.Sprintf(`Prometheus Edge Hub running on %s
Hub Limit:       %s
Hub Utilization: %s%%

Last Scrape: %d
	Scrape Size: %d
	Number of Familes: %d

Last Receive: %d
	Receive Size: %d
	Number of Families: %d

Current Count Families:   %d
Current Count Series:     %d
Current Count Datapoints: %d `, hostname, limitValue, utilizationValue,
		c.stats.lastScrapeTime, c.stats.lastScrapeSize, c.stats.lastScrapeNumFamilies,
		c.stats.lastReceiveTime, c.stats.lastReceiveSize, c.stats.lastReceiveNumFamilies,
		c.stats.currentCountFamilies, c.stats.currentCountSeries, c.stats.currentCountDatapoints)

	if verbose != "" {
		debugString += fmt.Sprintf("\n\nCurrent Exposition Text:\n%s\n", c.exposeMetrics(c.metricFamiliesByName, scrapeWorkerPoolSize))
	}

	return ctx.String(http.StatusOK, debugString)
}

func (c *MetricHub) updateCountStats() {
	numFamilies := len(c.metricFamiliesByName)
	numSeries := 0
	numDatapoints := 0
	for _, family := range c.metricFamiliesByName {
		numSeries += len(family.metrics)
		for _, series := range family.metrics {
			numDatapoints += len(series)
		}
	}
	c.stats.currentCountFamilies = numFamilies
	c.stats.currentCountSeries = numSeries
	c.stats.currentCountDatapoints = numDatapoints
}

type familyAndMetrics struct {
	family  *dto.MetricFamily
	metrics map[string][]*dto.Metric
}

func newFamilyAndMetrics(family *dto.MetricFamily) *familyAndMetrics {
	metrics := make(map[string][]*dto.Metric)
	for _, metric := range family.Metric {
		name := makeLabeledName(metric, family.GetName())
		if metricQueue, ok := metrics[name]; ok {
			if *metric.TimestampMs >= *metricQueue[len(metricQueue)-1].TimestampMs {
				metrics[name] = append(metricQueue, metric)
			} else {
				metrics[name] = sortedInsert(metricQueue, metric)
			}
		} else {
			metrics[name] = []*dto.Metric{metric}
		}
	}
	// clear metrics in family because we are keeping them in the queues
	family.Metric = nil

	return &familyAndMetrics{
		family:  family,
		metrics: metrics,
	}
}

func (f *familyAndMetrics) addMetrics(newMetrics []*dto.Metric) {
	// Keep array sorted [t0, t1, t2...] each insert
	for _, metric := range newMetrics {
		metricName := makeLabeledName(metric, f.family.GetName())
		if queue, ok := f.metrics[metricName]; ok {
			if *metric.TimestampMs >= *queue[len(queue)-1].TimestampMs {
				f.metrics[metricName] = append(queue, metric)
			} else {
				queue = sortedInsert(queue, metric)
			}
		} else {
			f.metrics[metricName] = []*dto.Metric{metric}
		}
	}
}

// Returns a prometheus MetricFamily populated with all datapoints, sorted so
// that the earliest datapoint appears first
func (f *familyAndMetrics) popDatapoints() *dto.MetricFamily {
	pullFamily := f.copyFamily()
	for _, queue := range f.metrics {
		if len(queue) == 0 {
			continue
		}
		pullFamily.Metric = append(pullFamily.Metric, queue...)
	}
	return &pullFamily
}

// return a copy of the MetricFamily that can be modified safely
func (f *familyAndMetrics) copyFamily() dto.MetricFamily {
	return *f.family
}

// makeLabeledName builds a unique name from a metric LabelPairs
func makeLabeledName(metric *dto.Metric, metricName string) string {
	labels := metric.GetLabel()
	sort.Slice(labels, func(i, j int) bool {
		return labels[i].GetName() < labels[j].GetName()
	})

	labeledName := strings.Builder{}
	labeledName.WriteString(metricName)
	for _, labelPair := range labels {
		labeledName.WriteString(fmt.Sprintf("_%s_%s", labelPair.GetName(), labelPair.GetValue()))
	}
	return labeledName.String()
}

func familyToString(family *dto.MetricFamily) (string, error) {
	var buf bytes.Buffer
	_, err := expfmt.MetricFamilyToText(&buf, family)
	if err != nil {
		return "", fmt.Errorf("error writing family string: %v", err)
	}
	return buf.String(), nil
}

func WriteInternalMetrics() (string, error) {
	metrics, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		return "", err
	}
	str := strings.Builder{}
	for _, fam := range metrics {
		buf := bytes.Buffer{}
		_, err := expfmt.MetricFamilyToText(&buf, fam)
		if err != nil {
			return "", err
		}
		str.WriteString(buf.String())
	}
	return str.String(), nil
}

func sortedInsert(data []*dto.Metric, el *dto.Metric) []*dto.Metric {
	index := sort.Search(len(data), func(i int) bool { return *data[i].TimestampMs > *el.TimestampMs })
	data = append(data, &dto.Metric{})
	copy(data[index+1:], data[index:])
	data[index] = el
	return data
}
