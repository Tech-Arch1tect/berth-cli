package ws

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"

	"github.com/Tech-Arch1tect/berth-cli/pkg/config"
	"github.com/gorilla/websocket"
)

func Dial(cfg *config.Config, path string) (*websocket.Conn, error) {
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
		dialer = &websocket.Dialer{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+cfg.APIKey)

	conn, _, err := dialer.Dial(u.String(), headers)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to WebSocket: %w", err)
	}

	return conn, nil
}
