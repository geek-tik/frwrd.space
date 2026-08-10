package agentcli

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"github.com/forward/forward/internal/config"
)

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Проверка API token",
}

var authVerifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Проверить FORWARD_API_TOKEN на сервере",
	RunE: func(_ *cobra.Command, _ []string) error {
		cfg := config.LoadAgent()
		if cfg.Token == "" {
			return fmt.Errorf("FORWARD_API_TOKEN не задан")
		}

		url := wsToHTTP(cfg.ServerURL) + "/agent/connect"
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+cfg.Token)

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("запрос к серверу: %w", err)
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("сервер вернул %d: %s", resp.StatusCode, string(body))
		}

		fmt.Printf("API token действителен\n%s\n", string(body))
		return nil
	},
}

func wsToHTTP(serverURL string) string {
	switch {
	case len(serverURL) >= 5 && serverURL[:5] == "ws://":
		return "http://" + serverURL[5:]
	case len(serverURL) >= 6 && serverURL[:6] == "wss://":
		return "https://" + serverURL[6:]
	default:
		return serverURL
	}
}

func init() {
	authCmd.AddCommand(authVerifyCmd)
}
