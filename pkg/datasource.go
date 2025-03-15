package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/datasource"
	"github.com/grafana/grafana-plugin-sdk-go/backend/httpclient"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/backend/resource/httpadapter"
)

type testDataSource struct {
	httpClient *http.Client
	backend.CallResourceHandler
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

	resp, err := ds.httpClient.Get("http://localhost:3000/api/health")
	if err != nil {
		return &backend.CheckHealthResult{
			Status:  backend.HealthStatusError,
			Message: "Failed to reach Grafana API",
		}, err
	}
	defer resp.Body.Close()

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
