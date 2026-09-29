package otlphttp_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/config/configcompression"
	"go.opentelemetry.io/collector/config/configoptional"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/collector/exporter/exportertest"
	"go.opentelemetry.io/collector/exporter/otlphttpexporter"

	"github.com/grafana/alloy/internal/component/otelcol/auth"
	"github.com/grafana/alloy/internal/component/otelcol/exporter/otlphttp"
	"github.com/grafana/alloy/syntax"
)

func TestFallbackConfig(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, block string
		configured  bool
	}{
		{name: "disabled"},
		{name: "defaults", block: `fallback_client { endpoint = "https://backup.local/base" }`, configured: true},
		{name: "explicit", block: `fallback_client {
			endpoint = "https://backup.local/base"
			timeout = "15s"
			compression = "none"
			proxy_url = "http://proxy.local:3128"
			headers = { "X-Backend" = "backup" }
		}`, configured: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var args otlphttp.Arguments
			require.NoError(t, syntax.Unmarshal([]byte(`client { endpoint = "http://primary.local" }`+"\n"+tc.block), &args))
			converted, err := args.Convert()
			require.NoError(t, err)
			cfg := converted.(*otlphttpexporter.Config)
			if !tc.configured {
				require.Nil(t, args.FallbackClient)
				require.Nil(t, cfg.FallbackClient)
				return
			}
			require.NotNil(t, cfg.FallbackClient)
			require.Equal(t, "https://backup.local/base", cfg.FallbackClient.Endpoint)
			require.Equal(t, "http://primary.local", cfg.ClientConfig.Endpoint)
			require.NoError(t, cfg.Validate())
			if tc.name == "defaults" {
				require.Equal(t, 30*time.Second, cfg.FallbackClient.Timeout)
				require.Equal(t, configcompression.TypeGzip, cfg.FallbackClient.Compression)
				require.Equal(t, 100, cfg.FallbackClient.MaxIdleConns)
			} else {
				require.Equal(t, 15*time.Second, cfg.FallbackClient.Timeout)
				require.Equal(t, configcompression.Type("none"), cfg.FallbackClient.Compression)
				require.Equal(t, "http://proxy.local:3128", cfg.FallbackClient.ProxyURL)
				value, ok := cfg.FallbackClient.Headers.Get("X-Backend")
				require.True(t, ok)
				require.Equal(t, "backup", string(value))
			}
		})
	}
	var args otlphttp.Arguments
	require.Error(t, syntax.Unmarshal([]byte(`client { endpoint = "http://primary.local" }
fallback_client { endpoint = "" }`), &args))
}

func TestFallbackAuthExtensions(t *testing.T) {
	t.Parallel()
	var args otlphttp.Arguments
	require.NoError(t, syntax.Unmarshal([]byte(`client { endpoint = "http://primary.local" }
fallback_client { endpoint = "http://backup.local" }`), &args))
	primaryID := component.MustNewIDWithName("testauth", "primary")
	fallbackID := component.MustNewIDWithName("testauth", "fallback")
	args.Client.Authentication = auth.NewHandler("primary")
	args.FallbackClient.Authentication = auth.NewHandler("fallback")
	require.NoError(t, args.Client.Authentication.AddExtension(auth.Client, &auth.ExtensionHandler{ID: primaryID}))
	require.NoError(t, args.FallbackClient.Authentication.AddExtension(auth.Client, &auth.ExtensionHandler{ID: fallbackID}))
	converted, err := args.Convert()
	require.NoError(t, err)
	cfg := converted.(*otlphttpexporter.Config)
	require.Equal(t, primaryID, cfg.ClientConfig.Auth.Get().AuthenticatorID)
	require.Equal(t, fallbackID, cfg.FallbackClient.Auth.Get().AuthenticatorID)
	extensions := args.Extensions()
	require.Contains(t, extensions, primaryID)
	require.Contains(t, extensions, fallbackID)
	args.FallbackClient.Authentication = auth.NewHandler("unavailable")
	_, err = args.Convert()
	require.ErrorContains(t, err, "fallback_client")
}

func TestFallbackRoutesToBackup(t *testing.T) {
	t.Parallel()
	var primaryCalls atomic.Int32
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		primaryCalls.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		<-r.Context().Done()
	}))
	defer primary.Close()
	paths := make(chan string, 2)
	backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.NotEmpty(t, body)
		require.Equal(t, "backup", r.Header.Get("X-Backend"))
		paths <- r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer backup.Close()
	var args otlphttp.Arguments
	require.NoError(t, syntax.Unmarshal([]byte(fmt.Sprintf(`
client {
  endpoint = %q
  timeout = "100ms"
  compression = "none"
  headers = { "X-Backend" = "primary" }
}
traces_endpoint = %q
fallback_client {
  endpoint = %q
  compression = "none"
  headers = { "X-Backend" = "backup" }
}
`, primary.URL, primary.URL+"/custom-traces", backup.URL+"/tenant")), &args))
	converted, err := args.Convert()
	require.NoError(t, err)
	cfg := converted.(*otlphttpexporter.Config)
	cfg.QueueConfig = configoptional.None[exporterhelper.QueueBatchConfig]()
	cfg.RetryConfig.Enabled = false
	factory := otlphttpexporter.NewFactory()
	exp, err := factory.CreateTraces(t.Context(), exportertest.NewNopSettings(factory.Type()), cfg)
	require.NoError(t, err)
	require.NoError(t, exp.Start(t.Context(), componenttest.NewNopHost()))
	defer func() { require.NoError(t, exp.Shutdown(t.Context())) }()
	for range 2 {
		require.NoError(t, exp.ConsumeTraces(t.Context(), createTestTraces()))
		select {
		case path := <-paths:
			require.Equal(t, "/tenant/v1/traces", path)
		case <-time.After(5 * time.Second):
			t.Fatal("fallback did not receive the request")
		}
	}
	require.Equal(t, int32(1), primaryCalls.Load())
}
