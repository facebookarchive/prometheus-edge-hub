/*
 * Copyright (c) Facebook, Inc. and its affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

package hub

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/stretchr/testify/assert"
)

const (
	timestamp = 1559953047

	sampleReceiveString = `
# HELP http_requests_total The total number of HTTP requests.
# TYPE http_requests_total counter
http_requests_total{method="post",code="200"} 1027 1395066363410
http_requests_total{method="post",code="400"}    3 1395066363021
http_requests_total{method="post",code="400"}    3 1395066363010
http_requests_total{method="post",code="400"}    3 1395066363330
http_requests_total{method="post",code="400"}    3 1395066363000
# HELP cpu_usage The total CPU usage.
# TYPE cpu_usage gauge
cpu_usage{host="A"} 1027 1395066363000
cpu_usage{host="B"}    3 1395066363100
cpu_usage{host="B"}    3 1395066363030
cpu_usage{host="B"}    3 1395066363130
cpu_usage{host="B"}    3 1395066363040
# HELP memory_usage The total memory usage.
# TYPE memory_usage gauge
memory_usage{host="A"} 5 1395066363920
memory_usage{host="A"} 5 1395066363130
memory_usage{host="A"} 5 1395066363430
memory_usage{host="A"} 5 1395066363590
`
)

var (
	testName   = "testName"
	testValue  = "testValue"
	testLabels = []*dto.LabelPair{{Name: &testName, Value: &testValue}}
)

func TestReceiveMetrics(t *testing.T) {
	hub := NewMetricHub(0, 10)
	resp, err := receiveString(hub, sampleReceiveString)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.Code)

	// Check Internal Metrics
	assertPrometheusValue(t, internalMetricHubSize, 14)
	assertPrometheusValue(t, internalMetricHubLimit, 0)
}

func TestReceiveOverLimit(t *testing.T) {
	hub := NewMetricHub(1, 10)
	resp, err := receiveString(hub, sampleReceiveString)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusNotAcceptable, resp.Code)
}

func TestReceiveBadMetrics(t *testing.T) {
	hub := NewMetricHub(0, 10)
	resp, _ := receiveString(hub, "bad metric string")
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func receiveString(hub *MetricHub, receiveString string) (*httptest.ResponseRecorder, error) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(receiveString))
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)

	err := hub.Receive(c)
	return rec, err
}

func TestScrape(t *testing.T) {
	hub := NewMetricHub(0, 10)
	_, err := receiveString(hub, sampleReceiveString)
	assert.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)

	err = hub.Scrape(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	// parse the output to make sure it gives valid response
	var parser expfmt.TextParser
	parsedFamilies, err := parser.TextToMetricFamilies(rec.Body)
	assert.NoError(t, err)
	assert.Equal(t, 3, len(parsedFamilies))

	// make sure all metrics are returned.
	sum := 0
	for _, family := range parsedFamilies {
		sum += len(family.Metric)
	}
	assert.Equal(t, 14, sum)
}

func TestScrapeBadMetrics(t *testing.T) {
	// check that Scrape handles errors
	assertWorkerPoolHandlesError(t)
}

func TestDebugEndpoint(t *testing.T) {
	hub := NewMetricHub(20, 10)
	_, err := receiveString(hub, sampleReceiveString)
	assert.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/debug", nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)

	err = hub.Debug(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	assert.Equal(t, 5, hub.stats.currentCountSeries)
	assert.Equal(t, 3, hub.stats.currentCountFamilies)
	assert.Equal(t, 14, hub.stats.currentCountDatapoints)
	assert.Equal(t, 3, hub.stats.lastReceiveNumFamilies)
}

func TestHubMetrics(t *testing.T) {
	hubSingleFamily(t, 1)
	hubSingleFamily(t, 100)
	hubSingleFamily(t, 10000)

	hubMultipleFamilies(t)
	hubMultipleSeries(t)

	assertTimestampsSortedProperly(t)
}

func hubSingleFamily(t *testing.T, metricsInFamily int) {
	hub := NewMetricHub(0, 10)
	mf := makeFamily(dto.MetricType_GAUGE, "metricA", metricsInFamily, testLabels, timestamp)
	metrics := map[string]*dto.MetricFamily{"metricA": mf}

	hub.hubMetrics(metrics)
	// 1 family, 1 series with multiple datapoints
	assert.Equal(t, len(hub.metricFamiliesByName), 1)
	for _, family := range hub.metricFamiliesByName {
		assert.Equal(t, 1, len(family.metrics))
		for _, metric := range family.metrics {
			assert.Equal(t, metricsInFamily, len(metric))
		}
	}
}

func hubMultipleFamilies(t *testing.T) {
	hub := NewMetricHub(0, 10)
	mf1 := makeFamily(dto.MetricType_GAUGE, "mf1", 5, testLabels, timestamp)
	mf2 := makeFamily(dto.MetricType_GAUGE, "mf2", 10, testLabels, timestamp)
	metrics := map[string]*dto.MetricFamily{"mf1": mf1, "mf2": mf2}

	hub.hubMetrics(metrics)
	// 2 families each with 1 series
	assert.Equal(t, len(hub.metricFamiliesByName), 2)
	for familyName, family := range hub.metricFamiliesByName {
		if strings.HasPrefix(familyName, "mf1") {
			assert.Equal(t, 1, len(family.metrics))
			for _, metric := range family.metrics {
				assert.Equal(t, 5, len(metric))
			}
		} else {
			assert.Equal(t, 1, len(family.metrics))
			for _, metric := range family.metrics {
				assert.Equal(t, 10, len(metric))
			}
		}
	}
}

func hubMultipleSeries(t *testing.T) {
	hub := NewMetricHub(0, 10)
	mf1 := makeFamily(dto.MetricType_GAUGE, "mf1", 1, testLabels, timestamp)
	mf2 := makeFamily(dto.MetricType_GAUGE, "mf1", 1, []*dto.LabelPair{}, timestamp)
	mf1Map := map[string]*dto.MetricFamily{"mf1": mf1}
	mf2Map := map[string]*dto.MetricFamily{"mf1": mf2}

	hub.hubMetrics(mf1Map)
	hub.hubMetrics(mf2Map)
	// 1 family with 2 unique series
	assert.Equal(t, len(hub.metricFamiliesByName), 1)
	for _, family := range hub.metricFamiliesByName {
		assert.Equal(t, 2, len(family.metrics))
	}
}

func assertTimestampsSortedProperly(t *testing.T) {
	hub := NewMetricHub(0, 10)
	counterValues := []float64{123, 234, 456}
	counterTimes := []int64{1, 2, 3}
	counter1 := dto.Counter{
		Value: &counterValues[0],
	}
	counter2 := dto.Counter{
		Value: &counterValues[1],
	}
	counter3 := dto.Counter{
		Value: &counterValues[2],
	}
	familyName := "mf1"
	mf := dto.MetricFamily{
		Name: &familyName,
		Metric: []*dto.Metric{{
			Counter:     &counter3,
			TimestampMs: &counterTimes[2],
		},
			{
				Counter:     &counter1,
				TimestampMs: &counterTimes[0],
			},
			{
				Counter:     &counter2,
				TimestampMs: &counterTimes[1],
			},
		},
	}

	metrics := map[string]*dto.MetricFamily{"mf": &mf}
	hub.hubMetrics(metrics)

	expectedExpositionText := `# TYPE mf1 counter
mf1 123 1
mf1 234 2
mf1 456 3
`
	assert.Equal(t, expectedExpositionText, hub.exposeMetrics(hub.metricFamiliesByName, 1))
}

func assertWorkerPoolHandlesError(t *testing.T) {
	hub := NewMetricHub(0, 10)
	counterValues := []float64{123, 234, 456}
	counterTimes := []int64{1, 2, 3}
	counter1 := dto.Counter{
		Value: &counterValues[0],
	}
	counter2 := dto.Counter{
		Value: &counterValues[1],
	}
	counter3 := dto.Counter{
		Value: &counterValues[2],
	}
	familyName := "mf1"
	blankFamilyName := "" // for error

	mf := dto.MetricFamily{
		Name: &familyName,
		Metric: []*dto.Metric{{
			Counter:     &counter3,
			TimestampMs: &counterTimes[2],
		},
			{
				Counter:     &counter1,
				TimestampMs: &counterTimes[0],
			},
			{
				Counter:     &counter2,
				TimestampMs: &counterTimes[1],
			},
		},
	}

	errorFamily := dto.MetricFamily{
		Name: &blankFamilyName,
		Metric: []*dto.Metric{{
			Counter:     &counter3,
			TimestampMs: &counterTimes[2],
		},
			{
				Counter:     &counter1,
				TimestampMs: &counterTimes[0],
			},
			{
				Counter:     &counter2,
				TimestampMs: &counterTimes[1],
			},
		},
	}

	metrics := map[string]*dto.MetricFamily{"mf": &mf, "errorFamily": &errorFamily}
	hub.hubMetrics(metrics)

	expectedExpositionText := `# TYPE mf1 counter
mf1 123 1
mf1 234 2
mf1 456 3
`
	assert.Equal(t, expectedExpositionText, hub.exposeMetrics(hub.metricFamiliesByName, 5))
}

func getGaugeValue(gauge prometheus.Gauge) float64 {
	var dtoMetric dto.Metric
	_ = gauge.Write(&dtoMetric)
	return *dtoMetric.Gauge.Value
}

func makeFamily(familyType dto.MetricType, familyName string, numMetrics int, labels []*dto.LabelPair, timestamp int64) *dto.MetricFamily {
	metrics := make([]*dto.Metric, 0)
	for i := 0; i < numMetrics; i++ {
		met := prometheus.NewGauge(prometheus.GaugeOpts{Name: familyName, Help: familyName})
		met.Set(float64(i))
		var dtoMetric dto.Metric
		_ = met.Write(&dtoMetric)

		dtoMetric.Label = append(dtoMetric.Label, labels...)
		dtoMetric.TimestampMs = &timestamp
		metrics = append(metrics, &dtoMetric)
	}

	return &dto.MetricFamily{
		Name:   &familyName,
		Help:   &familyName,
		Type:   &familyType,
		Metric: metrics,
	}
}

func assertPrometheusValue(t *testing.T, name string, expectedValue float64) {
	metrics, err := prometheus.DefaultGatherer.Gather()
	assert.NoError(t, err)
	for _, met := range metrics {
		if met.GetName() == name {
			assert.Equal(t, expectedValue, met.GetMetric()[0].Gauge.GetValue())
		}
	}
}
