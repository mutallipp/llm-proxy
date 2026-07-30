package api

import (
	"github.com/mutallipp/llm-proxy/internal/log"
)

var logger *log.Logger

func initLogger(l *log.Logger) {
	logger = l.WithName("api")
}
