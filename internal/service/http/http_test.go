package http

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/phayes/freeport"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/config"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/grafana/alloy/internal/component"
	"github.com/grafana/alloy/internal/featuregate"
	"github.com/grafana/alloy/internal/runtime/componenttest"
	"github.com/grafana/alloy/internal/service"
	"github.com/grafana/alloy/internal/service/remotecfg"
	"github.com/grafana/alloy/internal/util"
	"github.com/grafana/alloy/syntax"
)

func TestHTTP(t *testing.T) {
	ctx := componenttest.TestContext(t)

	env, err := newTestEnvironment(t)
	require.NoError(t, err)
	require.NoError(t, env.ApplyConfig(`/* empty */`))

	go func() {
		require.NoError(t, env.Run(ctx))
	}()

	util.Eventually(t, func(t require.TestingT) {
		cli, err := config.NewClientFromConfig(config.HTTPClientConfig{}, "test")
		require.NoError(t, err)

		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s/-/ready", env.ListenAddr()), nil)
		require.NoError(t, err)

		resp, err := cli.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		buf, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, "Alloy is ready.\n", string(buf))

		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	util.Eventually(t, func(t require.TestingT) {
		cli, err := config.NewClientFromConfig(config.HTTPClientConfig{}, "test")
		require.NoError(t, err)

		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s/-/healthy", env.ListenAddr()), nil)
		require.NoError(t, err)

		resp, err := cli.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		buf, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, "All Alloy components are healthy.\n", string(buf))

		require.Equal(t, http.StatusOK, resp.StatusCode)
	})
}

func TestTLS(t *testing.T) {
	ctx := componenttest.TestContext(t)

	env, err := newTestEnvironment(t)
	require.NoError(t, err)
	require.NoError(t, env.ApplyConfig(`
		tls {
			cert_file = "testdata/test-cert.crt"
			key_file = "testdata/test-key.key"
		}
	`))

	go func() {
		require.NoError(t, env.Run(ctx))
	}()

	util.Eventually(t, func(t require.TestingT) {
		cli, err := config.NewClientFromConfig(config.HTTPClientConfig{
			TLSConfig: config.TLSConfig{
				CAFile: "testdata/test-cert.crt",
			},
		}, "test")
		require.NoError(t, err)

		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("https://%s/-/ready", env.ListenAddr()), nil)
		require.NoError(t, err)

		resp, err := cli.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusOK, resp.StatusCode)
	})
}

func Test_Toggle_TLS(t *testing.T) {
	ctx := componenttest.TestContext(t)

	env, err := newTestEnvironment(t)
	require.NoError(t, err)

	go func() {
		require.NoError(t, env.Run(ctx))
	}()

	{
		// Start with plain HTTP.
		require.NoError(t, env.ApplyConfig(`/* empty */`))
		util.Eventually(t, func(t require.TestingT) {
			cli, err := config.NewClientFromConfig(config.HTTPClientConfig{}, "test")
			require.NoError(t, err)

			req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s/-/ready", env.ListenAddr()), nil)
			require.NoError(t, err)

			resp, err := cli.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			require.Equal(t, http.StatusOK, resp.StatusCode)
		})
	}

	{
		// Toggle TLS.
		require.NoError(t, env.ApplyConfig(`
			tls {
				cert_file = "testdata/test-cert.crt"
				key_file = "testdata/test-key.key"
			}
		`))

		util.Eventually(t, func(t require.TestingT) {
			cli, err := config.NewClientFromConfig(config.HTTPClientConfig{
				TLSConfig: config.TLSConfig{
					CAFile: "testdata/test-cert.crt",
				},
			}, "test")
			require.NoError(t, err)

			req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("https://%s/-/ready", env.ListenAddr()), nil)
			require.NoError(t, err)

			resp, err := cli.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			require.Equal(t, http.StatusOK, resp.StatusCode)
		})
	}

	{
		// Disable TLS.
		require.NoError(t, env.ApplyConfig(`/* empty */`))
		util.Eventually(t, func(t require.TestingT) {
			cli, err := config.NewClientFromConfig(config.HTTPClientConfig{}, "test")
			require.NoError(t, err)

			req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s/-/ready", env.ListenAddr()), nil)
			require.NoError(t, err)

			resp, err := cli.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			require.Equal(t, http.StatusOK, resp.StatusCode)
		})
	}
}

func TestAuth(t *testing.T) {
	ctx := componenttest.TestContext(t)

	env, err := newTestEnvironment(t)
	require.NoError(t, err)
	require.NoError(t, env.ApplyConfig(`
		auth {
			basic {
				username = "username"
				password = "password"
			}
			filter {
				paths = ["/"]
			}
		}
	`))

	go func() {
		require.NoError(t, env.Run(ctx))
	}()

	util.Eventually(t, func(t require.TestingT) {
		cli, err := config.NewClientFromConfig(config.HTTPClientConfig{}, "test")
		require.NoError(t, err)

		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s/-/ready", env.ListenAddr()), nil)
		require.NoError(t, err)

		resp, err := cli.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}

func Test_Toggle_Auth(t *testing.T) {
	ctx := componenttest.TestContext(t)

	env, err := newTestEnvironment(t)
	require.NoError(t, err)

	go func() {
		require.NoError(t, env.Run(ctx))
	}()

	request := func(t require.TestingT, cfg config.HTTPClientConfig) *http.Response {
		cli, err := config.NewClientFromConfig(cfg, "test")
		require.NoError(t, err)

		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s/-/ready", env.ListenAddr()), nil)
		require.NoError(t, err)

		resp, err := cli.Do(req)
		require.NoError(t, err)
		return resp
	}

	{
		// Start without auth.
		require.NoError(t, env.ApplyConfig(`/* empty */`))
		util.Eventually(t, func(t require.TestingT) {
			resp := request(t, config.HTTPClientConfig{})
			require.NoError(t, resp.Body.Close())
			require.Equal(t, http.StatusOK, resp.StatusCode)
		})
	}

	{
		// Toggle Auth.
		require.NoError(t, env.ApplyConfig(`
			auth {
				basic {
					username = "username"
					password = "password"
				}
				filter {
					paths = ["/"]
				}
			}
		`))

		util.Eventually(t, func(t require.TestingT) {
			resp := request(t, config.HTTPClientConfig{})
			require.NoError(t, resp.Body.Close())
			require.Equal(t, http.StatusUnauthorized, resp.StatusCode)

			resp = request(t, config.HTTPClientConfig{BasicAuth: &config.BasicAuth{
				Username: "user",
				Password: "password",
			}})
			require.NoError(t, resp.Body.Close())
			require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		})
	}

	{
		// Disable Auth.
		require.NoError(t, env.ApplyConfig(``))
		util.Eventually(t, func(t require.TestingT) {
			resp := request(t, config.HTTPClientConfig{})
			require.NoError(t, resp.Body.Close())
			require.Equal(t, http.StatusOK, resp.StatusCode)
		})
	}
}

func TestUnhealthy(t *testing.T) {
	ctx := componenttest.TestContext(t)

	env, err := newTestEnvironment(t)
	require.NoError(t, err)

	env.components = []*component.Info{
		{
			ID: component.ID{
				ModuleID: "",
				LocalID:  "testCompId",
			},
			Label:         "testCompLabel",
			ComponentName: "testCompName",
			Health: component.Health{
				Health: component.HealthTypeUnhealthy,
			},
		},
	}
	require.NoError(t, env.ApplyConfig(""))

	go func() {
		require.NoError(t, env.Run(ctx))
	}()

	util.Eventually(t, func(t require.TestingT) {
		cli, err := config.NewClientFromConfig(config.HTTPClientConfig{}, "test")
		require.NoError(t, err)

		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("http://%s/-/healthy", env.ListenAddr()), nil)
		require.NoError(t, err)

		resp, err := cli.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		buf, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, "unhealthy components: testCompName\n", string(buf))

		require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})
}

func TestPrestop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env, err := newTestEnvironment(t)
	require.NoError(t, err)
	require.NoError(t, env.ApplyConfig(""))
	var calls atomic.Int32
	env.svc.opts.ShutdownFunc = func() { calls.Add(1) }
	env.svc.authenticator = basicAuthenticator("operator", "secret")
	done := make(chan error, 1)
	go func() { done <- env.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Error("HTTP service did not stop")
		}
	})
	client := &http.Client{Timeout: 5 * time.Second}
	util.Eventually(t, func(t require.TestingT) {
		resp, err := client.Get("http://" + env.ListenAddr() + "/-/ready")
		require.NoError(t, err)
		resp.Body.Close()
	})
	for _, test := range []struct {
		name   string
		method string
		auth   bool
		status int
		calls  int32
	}{
		{"unauthorized", http.MethodGet, false, http.StatusUnauthorized, 0},
		{"post rejected", http.MethodPost, true, http.StatusMethodNotAllowed, 0},
		{"shutdown", http.MethodGet, true, http.StatusOK, 1},
		{"repeated shutdown", http.MethodGet, true, http.StatusOK, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			req, err := http.NewRequest(test.method, "http://"+env.ListenAddr()+"/prestop", nil)
			require.NoError(t, err)
			if test.auth {
				req.SetBasicAuth("operator", "secret")
			}
			resp, err := client.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			_, err = io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, test.status, resp.StatusCode)
			require.Equal(t, test.calls, calls.Load())
		})
	}
}

func TestPrestopWaitsForShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env, err := newTestEnvironment(t)
	require.NoError(t, err)
	require.NoError(t, env.ApplyConfig(""))
	shutdownDone := make(chan struct{})
	finishShutdown := sync.OnceFunc(func() { close(shutdownDone) })
	t.Cleanup(finishShutdown)
	env.svc.opts.ShutdownFunc = cancel
	env.svc.opts.ShutdownDone = shutdownDone
	serviceDone := make(chan error, 1)
	go func() { serviceDone <- env.Run(ctx) }()
	client := &http.Client{Timeout: 5 * time.Second}
	util.Eventually(t, func(t require.TestingT) {
		resp, err := client.Get("http://" + env.ListenAddr() + "/-/ready")
		require.NoError(t, err)
		resp.Body.Close()
	})
	responseDone := make(chan struct{})
	var response *http.Response
	var responseErr error
	go func() {
		response, responseErr = client.Get("http://" + env.ListenAddr() + "/prestop")
		close(responseDone)
	}()
	select {
	case err := <-serviceDone:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("HTTP service shutdown blocked on prestop handler")
	}
	select {
	case <-responseDone:
		t.Fatal("prestop returned before shutdown completed")
	case <-time.After(100 * time.Millisecond):
	}
	finishShutdown()
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer waitCancel()
	require.NoError(t, env.svc.WaitForShutdown(waitCtx))
	select {
	case <-responseDone:
	case <-waitCtx.Done():
		t.Fatal("prestop did not return after shutdown completed")
	}
	require.NoError(t, responseErr)
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.Equal(t, "graceful shutdown complete\n", string(body))
}

type testEnvironment struct {
	svc        *Service
	addr       string
	components []*component.Info
}

func newTestEnvironment(t *testing.T) (*testEnvironment, error) {
	port, err := freeport.GetFreePort()
	if err != nil {
		return nil, err
	}

	svc := New(Options{
		Logger:   util.TestAlloyLogger(t),
		Tracer:   noop.NewTracerProvider(),
		Gatherer: prometheus.NewRegistry(),

		ReadyFunc:  func() bool { return true },
		ReloadFunc: func() error { return nil },

		HTTPListenAddr:   fmt.Sprintf("127.0.0.1:%d", port),
		MemoryListenAddr: "alloy.internal:12345",
		EnablePProf:      true,
	})

	return &testEnvironment{
		svc:  svc,
		addr: fmt.Sprintf("127.0.0.1:%d", port),
	}, nil
}

func (env *testEnvironment) ApplyConfig(config string) error {
	var args Arguments
	if err := syntax.Unmarshal([]byte(config), &args); err != nil {
		return err
	}
	return env.svc.Update(args)
}

func (env *testEnvironment) Run(ctx context.Context) error {
	return env.svc.Run(ctx, fakeHost{
		components: env.components,
	})
}

func (env *testEnvironment) ListenAddr() string { return env.addr }

type fakeHost struct {
	components []*component.Info
}

var _ service.Host = (fakeHost{})

func (fakeHost) GetComponent(id component.ID, opts component.InfoOptions) (*component.Info, error) {
	return nil, fmt.Errorf("no such component %s", id)
}

func (f fakeHost) ListComponents(moduleID string, opts component.InfoOptions) ([]*component.Info, error) {
	if f.components != nil {
		return f.components, nil
	}
	if moduleID == "" {
		return nil, nil
	}
	return nil, fmt.Errorf("no such module %q", moduleID)
}

func (fakeHost) GetServiceConsumers(serviceName string) []service.Consumer { return nil }

func (fakeHost) NewController(id string) (service.Controller, error) { return nil, nil }

func (fakeHost) GetService(svc string) (service.Service, bool) {
	if svc == remotecfg.ServiceName {
		return fakeRemotecfg{}, true
	}
	return nil, false
}

type fakeRemotecfg struct{}

func (f fakeRemotecfg) Definition() service.Definition {
	return service.Definition{
		Name:       remotecfg.ServiceName,
		ConfigType: remotecfg.Arguments{},
		DependsOn:  nil, // remotecfg has no dependencies.
		Stability:  featuregate.StabilityPublicPreview,
	}
}
func (f fakeRemotecfg) Run(ctx context.Context, host service.Host) error { return nil }
func (f fakeRemotecfg) Update(newConfig any) error                       { return nil }
func (f fakeRemotecfg) Data() any                                        { return remotecfg.Data{} }
