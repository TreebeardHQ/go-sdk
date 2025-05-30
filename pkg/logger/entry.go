package logger

import (
	"runtime"
	"strings"
	"time"
)

// LogEntry represents a structured log entry
type LogEntry struct {
	Timestamp   time.Time              `json:"ts"`
	TraceID     string                 `json:"tid"`
	Message     string                 `json:"msg"`
	Level       string                 `json:"lvl"`
	File        string                 `json:"fl"`
	Line        int                    `json:"ln"`
	Traceback   string                 `json:"tb,omitempty"`
	TraceName   string                 `json:"tn,omitempty"`
	Source      string                 `json:"src"`
	Properties  map[string]interface{} `json:"props,omitempty"`
}

// CallerInfo holds information about the caller
type CallerInfo struct {
	File string
	Line int
	Func string
}

// GetCaller returns caller information, skipping frames from this package
func GetCaller(skip int) CallerInfo {
	// Start from the caller of GetCaller + additional skip
	pc, file, line, ok := runtime.Caller(skip + 1)
	if !ok {
		return CallerInfo{}
	}

	// Get function name
	fn := runtime.FuncForPC(pc)
	funcName := ""
	if fn != nil {
		funcName = fn.Name()
		// Extract just the function name from the full path
		if idx := strings.LastIndex(funcName, "."); idx >= 0 {
			funcName = funcName[idx+1:]
		}
	}

	// Extract just the filename from the full path
	if idx := strings.LastIndex(file, "/"); idx >= 0 {
		file = file[idx+1:]
	}

	return CallerInfo{
		File: file,
		Line: line,
		Func: funcName,
	}
}