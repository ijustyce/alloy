package otlphttp_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/exporter/otlphttpexporter"

	"github.com/grafana/alloy/internal/component/otelcol/exporter/otlphttp"
	"github.com/grafana/alloy/syntax"
)

func TestPrecompressLogs(t *testing.T) {
	for _, test := range []struct {
		name      string
		attribute string
		want      bool
	}{
		{name: "default"},
		{name: "enabled", attribute: "precompress_logs = true", want: true},
		{name: "disabled", attribute: "precompress_logs = false"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var args otlphttp.Arguments
			require.NoError(t, syntax.Unmarshal([]byte(test.attribute+`
client {
  endpoint = "http://localhost:4318"
}
`), &args))
			require.Equal(t, test.want, args.PrecompressLogs)
			converted, err := args.Convert()
			require.NoError(t, err)
			cfg := converted.(*otlphttpexporter.Config)
			require.Equal(t, test.want, cfg.PrecompressLogs)
			require.NoError(t, cfg.Validate())
		})
	}
}
