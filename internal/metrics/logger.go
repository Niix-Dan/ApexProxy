package metrics

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type CSVLogger struct {
	Enabled       bool
	file          *os.File
	writer        *csv.Writer
	mu            sync.Mutex
	redactHeaders map[string]bool
}

var CSVLog = &CSVLogger{}

func InitCSV(enabled bool, path string, redactHeaders []string) error {
	if !enabled {
		return nil
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	CSVLog.file = f
	CSVLog.writer = csv.NewWriter(f)
	CSVLog.Enabled = true
	CSVLog.redactHeaders = make(map[string]bool)
	for _, h := range redactHeaders {
		CSVLog.redactHeaders[strings.ToLower(h)] = true
	}

	stat, _ := f.Stat()
	if stat.Size() == 0 {
		CSVLog.writer.Write([]string{"Timestamp", "IP", "Method", "URL", "Status", "Headers"})
		CSVLog.writer.Flush()
	}
	return nil
}

func (c *CSVLogger) LogRequest(req *http.Request, status int) {
	if !c.Enabled {
		return
	}
	headersMap := make(map[string]string)
	for k, v := range req.Header {
		if c.redactHeaders[strings.ToLower(k)] {
			headersMap[k] = "[REDACTED]"
		} else {
			headersMap[k] = strings.Join(v, "; ")
		}
	}
	hJSON, _ := json.Marshal(headersMap)

	record := []string{
		time.Now().Format(time.RFC3339),
		req.RemoteAddr,
		req.Method,
		req.URL.String(),
		fmt.Sprintf("%d", status),
		string(hJSON),
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.writer.Write(record)
	c.writer.Flush()
}
