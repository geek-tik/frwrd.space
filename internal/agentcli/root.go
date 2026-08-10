package agentcli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/forward/forward/internal/agent"
	"github.com/forward/forward/internal/config"
)

var rootCmd = &cobra.Command{
	Use:   "forward",
	Short: "Forward — проброс локальных портов в интернет",
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(authCmd)
	rootCmd.AddCommand(httpCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Показать версию",
	Run: func(_ *cobra.Command, _ []string) {
		fmt.Println("forward 0.1.0-dev")
	},
}

var httpCmd = &cobra.Command{
	Use:   "http <port>",
	Short: "Запустить HTTP-туннель на локальный порт",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		cfg := config.LoadAgent()
		if cfg.Token == "" {
			return fmt.Errorf("FORWARD_API_TOKEN не задан; создайте токен в dashboard")
		}

		port, err := agent.ParsePort(args[0])
		if err != nil {
			return err
		}

		return agent.RunHTTP(context.Background(), cfg, port)
	},
}
