package client

import (
	"context"
	"crypto/tls"
	"net/http"
	"time"

	"github.com/Tech-Arch1tect/berth-cli/pkg/config"
	berth "github.com/tech-arch1tect/berth-go-api-client"
)

type Client struct {
	API *berth.APIClient
	Ctx context.Context
}

func New(cfg *config.Config) *Client {
	apiCfg := berth.NewConfiguration()

	apiCfg.Servers = berth.ServerConfigurations{
		{URL: cfg.Server},
	}

	apiCfg.Debug = cfg.Verbose

	apiCfg.HTTPClient = &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: cfg.Insecure,
			},
		},
	}

	apiClient := berth.NewAPIClient(apiCfg)
	ctx := context.WithValue(context.Background(), berth.ContextAccessToken, cfg.APIKey)

	return &Client{
		API: apiClient,
		Ctx: ctx,
	}
}
