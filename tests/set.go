package models

import (
	"encoding/json"
	"fmt"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

type PluginSettings struct {
	Path    string                `json:"path"`
	Secrets *SecretPluginSettings `json:"-"`
}

type SecretPluginSettings struct {
	ApiKey string `json:"apiKey"`
}

func LoadPluginSettings(source backend.DataSourceInstanceSettings) (*PluginSettings, error) {
	settings := PluginSettings{}
	err := json.Unmarshal(source.JSONData, &settings)
	if err != nil {
		return nil, fmt.Errorf("could not unmarshal PluginSettings json: %w", err)
	}

	// Handling both values returned from loadSecretPluginSettings
	settings.Secrets, err = loadSecretPluginSettings(source.DecryptedSecureJSONData)
	if err != nil {
		return nil, fmt.Errorf("failed to load secret plugin settings: %w", err)
	}

	return &settings, nil
}

func loadSecretPluginSettings(source map[string]string) (*SecretPluginSettings, error) {
	apiKey, exists := source["apiKey"]
	if !exists || apiKey == "" {
		return nil, fmt.Errorf("apiKey is missing or empty")
	}

	return &SecretPluginSettings{
		ApiKey: apiKey,
	}, nil
}

