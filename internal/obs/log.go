package obs

import (
	"encoding/json"
	"log"
	"os"
	"sync"
)

var (
	loggerOnce sync.Once
	logger     *log.Logger
)

// Logger returns the shared structured logger used across the service.
func Logger() *log.Logger {
	loggerOnce.Do(func() {
		logger = log.New(os.Stdout, "", 0)
	})
	return logger
}

// LogRequest emits a structured JSON log line with common HTTP fields.
func LogRequest(entry map[string]any) {
	data, err := json.Marshal(entry)
	if err != nil {
		Logger().Println(`{"ts":"error","level":"error","msg":"log marshal failed"}`)
		return
	}
	Logger().Println(string(data))
}
