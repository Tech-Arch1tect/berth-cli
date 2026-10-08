package ws

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"

	"github.com/Tech-Arch1tect/berth-cli/pkg/config"
	"github.com/gorilla/websocket"
)

func Dial(ctx context.Context, cfg *config.Config, path string) (*websocket.Conn, error) {
	u, err := url.Parse(cfg.Server)
	if err != nil {
		return nil, fmt.Errorf("invalid server URL: %w", err)
	}

	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	u.Path = path

	dialer := websocket.DefaultDialer
	if cfg.Insecure {
		insecureDialer := *dialer
		insecureDialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		dialer = &insecureDialer
	}

	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+cfg.APIKey)

	conn, response, err := dialer.DialContext(ctx, u.String(), headers)
	if err != nil {
		if response != nil {
			_ = response.Body.Close()
			return nil, fmt.Errorf("failed to connect to WebSocket (%s): %w", response.Status, err)
		}
		return nil, fmt.Errorf("failed to connect to WebSocket: %w", err)
	}

	return conn, nil
}
