package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/datasource"
	"github.com/grafana/grafana-plugin-sdk-go/backend/httpclient"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/backend/resource/httpadapter"

	"github.com/kirillyesikov/homelab-plugin/pkg/models"
)

type testDataSource struct {
	httpClient *http.Client
	backend.CallResourceHandler
	settings *models.PluginSettings
}

var registerMetricsOnce sync.Once

var queriesTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Namespace: "grafana_plugin",
		Name:      "queries_total",
		Help:      "Total number of queries.",
	},
	[]string{"query_type"},
)

func newDataSource(ctx context.Context, settings backend.DataSourceInstanceSettings) (instancemgmt.Instance, error) {
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

	mux := http.NewServeMux()
	mux.HandleFunc("/test", ds.handleTest)
	mux.Handle("/metrics", promhttp.Handler())
	prometheus.MustRegister(queriesTotal)

	registerMetricsOnce.Do(func() {
		prometheus.MustRegister(queriesTotal)
	})

	ds.CallResourceHandler = httpadapter.New(mux)

	return ds, nil
}

func (ds *testDataSource) Dispose() {}

func (ds *testDataSource) CheckHealth(_ context.Context, _ *backend.CheckHealthRequest) (*backend.CheckHealthResult, error) {
	if ds.httpClient == nil {
		return &backend.CheckHealthResult{
			Status:  backend.HealthStatusError,
			Message: "httpClient is nil",
		}, nil
	}

	req, err := http.NewRequest("GET", "http://localhost:3000/api/plugins/homelab-kirill-datasource/metrics", nil)
	if err != nil {
		return &backend.CheckHealthResult{
			Status:  backend.HealthStatusError,
			Message: "Failed to create request",
		}, err
	}

	if ds.settings.Secrets != nil && ds.settings.Secrets.ApiKey != "" {
		req.Header.Set("Authorization", "Bearer "+ds.settings.Secrets.ApiKey)
	} else {
		return &backend.CheckHealthResult{
			Status:  backend.HealthStatusError,
			Message: "API Key is missing",
		}, nil
	}

	resp, err := ds.httpClient.Do(req)
	if err != nil {
		return &backend.CheckHealthResult{
			Status:  backend.HealthStatusError,
			Message: fmt.Sprintf("Failed to reach Grafana API: %v", err),
		}, err
	}
	defer resp.Body.Close()

	fmt.Println("Health Check Response Status:", resp.Status)
	body, _ := ioutil.ReadAll(resp.Body)
	fmt.Println("Health Check Response Body:", string(body))

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

	resp, err := ds.httpClient.Get("http://localhost:3000/api/plugins/homelab-kirill-datasource/metrics")
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
