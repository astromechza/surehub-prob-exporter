package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"

	"github.com/astromechza/surehub-prob-exporter/client"
	"github.com/astromechza/surehub-prob-exporter/poller"
)

const defaultAddress = ":8080"
const defaultHubApi = "https://app.api.surehub.io"

var rootCmd = &cobra.Command{
	Use: "surehub-prom-exporter",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		v, _ := cmd.Flags().GetCount("verbose")
		v = max(0, min(v, 2))
		slog.SetDefault(slog.New(slog.NewTextHandler(cmd.ErrOrStderr(), &slog.HandlerOptions{AddSource: v >= 2, Level: map[int]slog.Level{
			0: slog.LevelInfo,
			1: slog.LevelDebug,
			2: slog.LevelDebug,
		}[v]})))
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		slog.Info("startup", "debug", slog.Default().Enabled(context.Background(), slog.LevelDebug))

		surehubEmail := os.Getenv("SUREHUB_EMAIL")
		if surehubEmail == "" {
			return errors.New("SUREHUB_EMAIL is not set")
		}

		sureHubPassword := os.Getenv("SUREHUB_PASSWORD")
		if sureHubPassword == "" {
			return errors.New("$SUREHUB_PASSWORD is not set")
		}

		c, _ := client.NewClientWithResponses(defaultHubApi, client.WithHTTPClient(&http.Client{Timeout: time.Second * 30}))
		p := poller.Poller{Client: c, Interval: time.Minute, HubEmail: surehubEmail, HubPassword: sureHubPassword}
		if err := p.Start(context.Background()); err != nil {
			return err
		}

		e := echo.New()
		e.HideBanner = true
		e.HidePort = true
		e.Use(middleware.Recover())
		e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
			Skipper: func(c echo.Context) bool {
				return c.Request().RequestURI == "/alive"
			},
			HandleError:     true,
			LogStatus:       true,
			LogURI:          true,
			LogMethod:       true,
			LogResponseSize: true,
			LogUserAgent:    true,
			LogLatency:      true,
			LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
				fields := []any{
					slog.String("method", v.Method),
					slog.String("uri", v.URI),
					slog.Int("status", v.Status),
					slog.Int64("resp_size", v.ResponseSize),
					slog.String("user_agent", v.UserAgent),
					slog.Duration("latency", v.Latency),
				}
				if v.Error != nil {
					fields = append(fields, slog.Any("error", v.Error))
				}
				slog.Info("handled", fields...)
				return nil
			},
		}))
		promHandler := promhttp.Handler()
		e.GET("/metrics", func(c echo.Context) error {
			promHandler.ServeHTTP(c.Response(), c.Request())
			return nil
		})
		e.GET("/alive", func(c echo.Context) error {
			return nil
		})
		e.GET("/ready", func(c echo.Context) error {
			if err := p.UnreadyError(); err != nil {
				return err
			}
			return nil
		})

		slog.Info("starting webserver", "address", defaultAddress)
		return e.Start(defaultAddress)
	},
	SilenceErrors: true,
}

func init() {
	rootCmd.PersistentFlags().CountP("verbose", "v", "Increase log verbosity and detail by specifying this flag one or more times")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
