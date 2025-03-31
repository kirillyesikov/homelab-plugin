package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/datasource"
	"github.com/grafana/grafana-plugin-sdk-go/backend/httpclient"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	

	"github.com/kirillyesikov/homelab-plugin/pkg/models"
)

type testDataSource struct {
	httpClient *http.Client
	backend.CallResourceHandler
	settings *models.PluginSettings
}

var registerMetricsOnce sync.Once

func registerMetrics() {
	registerMetricsOnce.Do(func() {
		prometheus.MustRegister(queriesTotal)
	})
}

var queriesTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: "grafana_plugin",
		Name:      "queries_total",
		Help:      "Total number of queries.",
	},
	[]string{"query_type"},
)

func newDataSource(ctx context.Context, settings backend.DataSourceInstanceSettings) (instancemgmt.Instance, error) {
	backend.Logger.Info("Initializing new data source...")


	if settings.UID == "" {
		backend.Logger.Error("Data source instance settings are missing")
		return nil, fmt.Errorf("data source instance settings cannot be nil")
	}

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


	body, _ := io.ReadAll(resp.Body)
	backend.Logger.Info("CheckHealth response", "status", resp.Status, "body", string(body))


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


func (ds *testDataSource) QueryData(_ context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	for _, q := range req.Queries {
		queriesTotal.WithLabelValues(q.QueryType).Inc()
	}

	return &backend.QueryDataResponse{
		Responses: map[string]backend.DataResponse{
			"default": {},
		},
	}, nil
}

func (ds *testDataSource) handleTest(rw http.ResponseWriter, r *http.Request) {
	if ds.httpClient == nil {
		http.Error(rw, "httpClient is nil", http.StatusInternalServerError)
		return
	}

	resp, err := ds.httpClient.Get("http://localhost:3000/api/health")
	if err != nil {
		http.Error(rw, "Failed to reach Grafana API: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	rw.WriteHeader(http.StatusOK)
	rw.Write([]byte("Datasource is healthy"))
}

func main() {
	err := datasource.Manage("homelab-kirill-datasource", newDataSource, datasource.ManageOpts{})
	if err != nil {
		backend.Logger.Error(err.Error())
		os.Exit(1)
	}
}

