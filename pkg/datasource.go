package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"github.com/kirillyesikov/homelab-plugin"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/datasource"
	"github.com/grafana/grafana-plugin-sdk-go/backend/httpclient"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/backend/resource/httpadapter"
)

type testDataSource struct {
	httpClient *http.Client
	backend.CallResourceHandler
	settings *backend.DataSourceInstanceSettings
}

func newDataSource(ctx context.Context, settings backend.DataSourceInstanceSettings) (instancemgmt.Instance, error) {
	opts, err := settings.HTTPClientOptions(ctx)
	if err != nil {
		return nil, err
	}

	client, err := httpclient.New(opts)
	if err != nil {
		return nil, err
	}

	ds := &testDataSource{
		httpClient: client,
		settings:   &settings,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/test", ds.handleTest)
	ds.CallResourceHandler = httpadapter.New(mux)

	return ds, nil
}

func (ds *testDataSource) Dispose() {
	// Cleanup
}

func (ds *testDataSource) CheckHealth(_ context.Context, _ *backend.CheckHealthRequest) (*backend.CheckHealthResult, error) {
	if ds.httpClient == nil {
		return &backend.CheckHealthResult{
			Status:  backend.HealthStatusError,
			Message: "httpClient is nil",
		}, nil
	}

	// Create a new HTTP request
	req, err := http.NewRequest("GET", "http://localhost:3000/api/health", nil)
	if err != nil {
		return &backend.CheckHealthResult{
			Status:  backend.HealthStatusError,
			Message: "Failed to create request",
		}, err
	}

	// Add API Key to Authorization header
	if ds.settings.Secrets != nil && ds.settings.Secrets.ApiKey != "" {
		req.Header.Set("Authorization", "Bearer "+ds.settings.Secrets.ApiKey)
	} else {
		return &backend.CheckHealthResult{
			Status:  backend.HealthStatusError,
			Message: "API Key is missing",
		}, nil
	}

	// Send the request
	resp, err := ds.httpClient.Do(req)
	if err != nil {
		return &backend.CheckHealthResult{
			Status:  backend.HealthStatusError,
			Message: "Failed to reach Grafana API",
		}, err
	}
	defer resp.Body.Close()

	// Debugging: Print response status
	fmt.Println("Health Check Response Status:", resp.Status)

	// Check if response is 200 OK
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

func (ds *testDataSource) QueryData(_ context.Context, _ *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	if ds.httpClient == nil {
		return &backend.QueryDataResponse{
			Responses: map[string]backend.DataResponse{
				"error": {
					Error: fmt.Errorf("datasource instance not found"),
				},
			},
		}, nil
	}

	req, err := http.NewRequest("GET", "http://localhost:3000/api/health", nil)
	if err != nil {
		return &backend.QueryDataResponse{
			Responses: map[string]backend.DataResponse{
				"error": {
					Error: err,
				},
			},
		}, nil
	}

	// Add API Key to Authorization header
	if ds.settings.Secrets != nil && ds.settings.Secrets.ApiKey != "" {
		req.Header.Set("Authorization", "Bearer "+ds.settings.Secrets.ApiKey)
	}

	resp, err := ds.httpClient.Get("http://localhost:3000/api/health")
	if err != nil {
		return &backend.QueryDataResponse{
			Responses: map[string]backend.DataResponse{
				"error": {
					Error: err,
				},
			},
		}, nil
	}
	defer resp.Body.Close()

	fmt.Println("Response Status:", resp.Status)

	return &backend.QueryDataResponse{}, nil
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
