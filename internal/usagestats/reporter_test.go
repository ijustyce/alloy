package usagestats

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/grafana/alloy/internal/alloyseed"
	"github.com/grafana/alloy/internal/util"
)

// TestReporter checks that the reporter sends the components tracked for each
// engine. Components are supplied directly; turning an OTel config into
// component types is covered separately by TestExtractOtelComponents.
func TestReporter(t *testing.T) {
	// Report on every tick so the test doesn't wait for the production cadence.
	// Restore afterward so the mutated globals don't leak into other tests.
	origCheck, origInterval := reportCheckInterval, reportInterval
	t.Cleanup(func() { reportCheckInterval, reportInterval = origCheck, origInterval })
	reportCheckInterval = time.Millisecond
	reportInterval = time.Millisecond

	tests := map[string]struct {
		setup func(tr *Tracker)
		want  map[string]any
	}{
		"empty tracker reports no metrics": {
			setup: func(*Tracker) {},
			want:  map[string]any{},
		},
		"default engine reports enabled components": {
			setup: func(tr *Tracker) {
				tr.SetEnabledComponentsFunc(func() []string {
					return []string{"prometheus.scrape", "loki.write"}
				})
			},
			want: map[string]any{
				"enabled-components": []string{"prometheus.scrape", "loki.write"},
			},
		},
		"otel engine reports otel components": {
			setup: func(tr *Tracker) {
				tr.SetOTelComponentsFunc(func() map[string][]string {
					return map[string][]string{"receivers": {"otlp"}, "processors": {"batch"}}
				})
			},
			want: map[string]any{
				"otel-components": map[string][]string{"receivers": {"otlp"}, "processors": {"batch"}},
			},
		},
		"otel engine also reports embedded alloyengine components": {
			setup: func(tr *Tracker) {
				tr.SetOTelComponentsFunc(func() map[string][]string {
					return map[string][]string{"receivers": {"otlp"}}
				})
				tr.SetAlloyEngineComponentsFunc(func() []string {
					return []string{"prometheus.scrape", "loki.write"}
				})
			},
			want: map[string]any{
				"otel-components":        map[string][]string{"receivers": {"otlp"}},
				"alloyengine-components": []string{"prometheus.scrape", "loki.write"},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tr := &Tracker{}
			tc.setup(tr)

			logger := util.TestAlloyLogger(t).Slog()
			alloyseed.Init(t.TempDir(), logger)

			// Capture the metrics the reporter would have sent, without any network.
			sent := make(chan map[string]any, 1)
			rep := &reporter{
				logger: logger,
				send: func(_ context.Context, _ *alloyseed.Seed, _ time.Time, metrics map[string]any) error {
					select {
					case sent <- metrics:
					default:
					}
					return nil
				},
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go func() { _ = rep.run(ctx, tr.Metrics) }()

			select {
			case metrics := <-sent:
				require.Equal(t, tc.want, metrics)
			case <-time.After(5 * time.Second):
				t.Fatal("timed out waiting for a usage report")
			}
		})
	}
}

// TestExtractOtelComponents covers turning an OTel Collector config (as parsed
// from YAML) into component types grouped by kind: ids collapse to their type
// (otlp/2 -> otlp) and dedupe within a kind, non-component sections are ignored,
// and malformed sections and empty ids are skipped.
func TestExtractOtelComponents(t *testing.T) {
	const groupedConfig = `
receivers:
  otlp:
  otlp/2:
  prometheus/scrape:
processors:
  batch:
exporters:
  debug:
  otlp:
service:
  pipelines:
    metrics:
`
	const malformedConfig = `
receivers: not-a-map
processors:
exporters:
  "":
  "/foo":
  otlp:
`
	tests := map[string]struct {
		config string
		want   map[string][]string
	}{
		"empty config": {
			config: "",
			want:   map[string][]string{},
		},
		"groups by kind and collapses ids to types": {
			config: groupedConfig,
			want: map[string][]string{
				"receivers":  {"otlp", "prometheus"},
				"processors": {"batch"},
				"exporters":  {"debug", "otlp"},
			},
		},
		"skips malformed sections and empty ids": {
			config: malformedConfig,
			want: map[string][]string{
				"exporters": {"otlp"},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var conf map[string]any
			require.NoError(t, yaml.Unmarshal([]byte(tc.config), &conf))
			require.Equal(t, tc.want, ExtractOtelComponents(conf))
		})
	}
}

func Test_NextReport(t *testing.T) {
	fixtures := map[string]struct {
		interval  time.Duration
		createdAt time.Time
		now       time.Time

		next time.Time
	}{
		"createdAt aligned with interval and now": {
			interval:  1 * time.Hour,
			createdAt: time.Unix(0, time.Hour.Nanoseconds()),
			now:       time.Unix(0, 2*time.Hour.Nanoseconds()),
			next:      time.Unix(0, 2*time.Hour.Nanoseconds()),
		},
		"createdAt aligned with interval": {
			interval:  1 * time.Hour,
			createdAt: time.Unix(0, time.Hour.Nanoseconds()),
			now:       time.Unix(0, 2*time.Hour.Nanoseconds()+1),
			next:      time.Unix(0, 3*time.Hour.Nanoseconds()),
		},
		"createdAt not aligned": {
			interval:  1 * time.Hour,
			createdAt: time.Unix(0, time.Hour.Nanoseconds()+18*time.Minute.Nanoseconds()+20*time.Millisecond.Nanoseconds()),
			now:       time.Unix(0, 2*time.Hour.Nanoseconds()+1),
			next:      time.Unix(0, 2*time.Hour.Nanoseconds()+18*time.Minute.Nanoseconds()+20*time.Millisecond.Nanoseconds()),
		},
	}
	for name, f := range fixtures {
		t.Run(name, func(t *testing.T) {
			next := nextReport(f.interval, f.createdAt, f.now)
			require.Equal(t, f.next, next)
		})
	}
}
