package http

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zapcore"

	"github.com/grafana/alloy/internal/runtime/logging"
	"github.com/grafana/alloy/internal/slogadapter"
)

func TestLogLevelHandler(t *testing.T) {
	logger, err := logging.New(io.Discard, logging.Options{
		Level:  logging.LevelInfo,
		Format: logging.FormatLogfmt,
	})
	require.NoError(t, err)

	handler := newLogLevelHandler(logger, 10*time.Millisecond)
	t.Cleanup(handler.close)
	zapLogger := slogadapter.NewZap(logger.Slog())
	require.False(t, zapLogger.Core().Enabled(zapcore.DebugLevel))
	require.NoError(t, logger.Update(logging.Options{
		Level:  logging.LevelError,
		Format: logging.FormatLogfmt,
	}))

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, logLevelPath+"?level=debug", nil))
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "debug\n", response.Body.String())
	require.Equal(t, logging.LevelDebug, logger.Level())
	require.True(t, zapLogger.Core().Enabled(zapcore.DebugLevel))

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, logLevelPath+"?level=invalid", nil))
	require.Equal(t, "debug\n", response.Body.String())

	require.Eventually(t, func() bool {
		return logger.Level() == logging.LevelInfo
	}, time.Second, time.Millisecond)

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, logLevelPath+"?level=debug", nil))
	require.Equal(t, http.StatusMethodNotAllowed, response.Code)
}

func TestLogLevelHandlerResetsFromLatestUpdate(t *testing.T) {
	logger, err := logging.New(io.Discard, logging.Options{
		Level:  logging.LevelInfo,
		Format: logging.FormatLogfmt,
	})
	require.NoError(t, err)

	handler := newLogLevelHandler(logger, 20*time.Millisecond)
	t.Cleanup(handler.close)
	handler.setLevel(logging.LevelDebug)
	time.Sleep(10 * time.Millisecond)
	handler.setLevel(logging.LevelWarn)

	time.Sleep(15 * time.Millisecond)
	require.Equal(t, logging.LevelWarn, logger.Level())
	require.Eventually(t, func() bool {
		return logger.Level() == logging.LevelInfo
	}, time.Second, time.Millisecond)
}
