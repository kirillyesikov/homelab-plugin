package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"io"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/datasource"
	"github.com/grafana/grafana-plugin-sdk-go/backend/httpclient"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/kirillyesikov/homelab-plugin/pkg/models"
)

type testDataSource struct {
	httpClient *http.Client
	backend.CallResourceHandler
	settings *models.PluginSettings
}

type Query struct {
	Metric string `json:"metric"`
} 


var (
	registerMetricsOnce sync.Once

	queriesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "grafana_plugin",
			Name:      "queries_total",
			Help:      "Total number of queries.",
		},
		[]string{"query_type"},
	)

	healthCheckTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: "grafana_plugin",
			Name:      "health_checks_total",
			Help:      "Total number of health check calls.",
		},
	)

	healthCheckDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: "grafana_plugin",
			Name:      "health_check_duration_seconds",
			Help:      "Duration of health check requests.",
			Buckets:   prometheus.DefBuckets,
		},
	)
)

func registerMetrics() {
	registerMetricsOnce.Do(func() {
		prometheus.MustRegister(queriesTotal, healthCheckTotal, healthCheckDuration)
	})
}

func newDataSource(ctx context.Context, settings backend.DataSourceInstanceSettings) (instancemgmt.Instance, error) {
	backend.Logger.Info("Initializing new data source...")

	if settings.UID == "" {
		backend.Logger.Error("Data source instance settings are missing")
		return nil, fmt.Errorf("data source instance settings cannot be nil")
	}

	registerMetrics() // Ensure metrics are registered

	opts, err := settings.HTTPClientOptions(ctx)
	if err != nil {
		return nil, err
	}

	client, err := httpclient.New(opts)
	if err != nil {
		return nil, err
	}

	pluginSettings, err := models.LoadPluginSettings(settings)
	if err != nil {
		return nil, fmt.Errorf("failed to load plugin settings: %w", err)
	}

	ds := &testDataSource{
		httpClient: client,
		settings:   pluginSettings,
	}

	backend.Logger.Info("Data source initialized successfully")
	return ds, nil
}

func (ds *testDataSource) Dispose() {}

func (ds *testDataSource) CheckHealth(ctx context.Context, _ *backend.CheckHealthRequest) (*backend.CheckHealthResult, error) {
	backend.Logger.Info("CheckHealth called")
	healthCheckTotal.Inc() // Increment health check count

	start := time.Now()
	defer func() {
		healthCheckDuration.Observe(time.Since(start).Seconds())
	}()

	if ds.settings == nil {
		backend.Logger.Error("CheckHealth failed: Data source settings are nil")
		return &backend.CheckHealthResult{
			Status:  backend.HealthStatusError,
			Message: "Data source settings are not initialized",
		}, nil
	}

	if ds.httpClient == nil {
		backend.Logger.Error("CheckHealth failed: HTTP client is nil")
		return &backend.CheckHealthResult{
			Status:  backend.HealthStatusError,
			Message: "HTTP client is not initialized",
		}, nil
	}

	testURL := "http://localhost:3000/api/health"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, testURL, nil)
	if err != nil {
		return &backend.CheckHealthResult{
			Status:  backend.HealthStatusError,
			Message: "Failed to create health check request",
		}, err
	}

	if ds.settings.Secrets == nil || ds.settings.Secrets.ApiKey == "" {
		backend.Logger.Error("CheckHealth failed: Missing API key")
		return &backend.CheckHealthResult{
			Status:  backend.HealthStatusError,
			Message: "Missing API key in plugin settings",
		}, nil
	}
	req.Header.Set("Authorization", "Bearer "+ds.settings.Secrets.ApiKey)

	resp, err := ds.httpClient.Do(req)
	if err != nil {
		backend.Logger.Error("CheckHealth request failed", "error", err)
		return &backend.CheckHealthResult{
			Status:  backend.HealthStatusError,
			Message: fmt.Sprintf("Request error: %v", err),
		}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &backend.CheckHealthResult{
			Status:  backend.HealthStatusError,
			Message: fmt.Sprintf("Unexpected response: %s", resp.Status),
		}, nil
	}

	return &backend.CheckHealthResult{
		Status:  backend.HealthStatusOk,
		Message: "Datasource is healthy",
	}, nil
}

func startMetricsServer() {
	go func() {
		http.Handle("/metrics", promhttp.Handler()) // Serve metrics
		backend.Logger.Info("Starting metrics server on :2112")
		if err := http.ListenAndServe(":2112", nil); err != nil {
			backend.Logger.Error("Metrics server failed", "error", err)
		}
	}()
}

func (ds *testDataSource) QueryData(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	// Initialize an empty metric name variable
	var metricName string

	// Loop through the queries in the request
	for _, query := range req.Queries {
		// Unmarshal JSON query into a map or struct to access user-defined parameters
		var q Query
		if err := json.Unmarshal(query.JSON, &q); err != nil {
			return nil, fmt.Errorf("failed to unmarshal query JSON: %w", err)
		}

		// If a metric was specified, use it
		if q.Metric != "" {
			metricName = q.Metric
			break
		}
	}

	// If no metric name is provided, return an error
	if metricName == "" {
		return nil, fmt.Errorf("no metric specified in the query")
	}

	// Fetch the metrics data from the Prometheus endpoint
	metricsURL := "http://172.18.0.2:2112/metrics"
	metricsResp, err := ds.httpClient.Get(metricsURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch metrics from endpoint: %w", err)
	}
	defer metricsResp.Body.Close()

	metricsBody, err := io.ReadAll(metricsResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read metrics response: %w", err)
	}

	metricsData := string(metricsBody)
	backend.Logger.Info("Fetched metrics data")

	// Parse the Prometheus metrics and search for the user-defined metric
	var metricValue string
	lines := strings.Split(metricsData, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, metricName) { // Look for the user-defined metric
			parts := strings.Fields(line)
			if len(parts) == 2 {
				metricValue = parts[1]
				break
			}
		}
	}

	// If the metric is not found, return an error
	if metricValue == "" {
		return nil, fmt.Errorf("metric %s not found", metricName)
	}

	// Create a DataFrame to return the metric
	frame := data.NewFrame("metrics",
		data.NewField("metric_name", nil, []string{metricName}),
		data.NewField("metric_value", nil, []float64{toFloat(metricValue)}),
	)

	// Return the response with the metric data
	return &backend.QueryDataResponse{
		Responses: map[string]backend.DataResponse{
			"default": {
				Frames: data.Frames{frame},
			},
		},
	}, nil
}

// Helper function to convert string to float64 safely
func toFloat(value string) float64 {
	if f, err := strconv.ParseFloat(value, 64); err == nil {
		return f
	}
	return 0
}


func main() {
	startMetricsServer() // Start Prometheus metrics server
	err := datasource.Manage("homelab-kirill-datasource", newDataSource, datasource.ManageOpts{})
	if err != nil {
		backend.Logger.Error(err.Error())
	}
}

