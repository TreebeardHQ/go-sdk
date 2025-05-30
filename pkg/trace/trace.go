package trace

import (
	"crypto/rand"
	"fmt"
	"runtime"
	"strings"
)

// GenerateTraceID generates a unique trace ID
func GenerateTraceID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to timestamp-based ID if random fails
		return fmt.Sprintf("T%d", runtime.NumGoroutine())
	}
	return fmt.Sprintf("T%x", bytes)
}

// GetStackTrace returns the current stack trace as a string
func GetStackTrace(skip int) string {
	var lines []string
	
	// Get stack trace starting from the caller
	for i := skip + 1; ; i++ {
		pc, file, line, ok := runtime.Caller(i)
		if !ok {
			break
		}

		// Get function name
		fn := runtime.FuncForPC(pc)
		funcName := "unknown"
		if fn != nil {
			funcName = fn.Name()
		}

		// Format: function (file:line)
		stackLine := fmt.Sprintf("%s (%s:%d)", funcName, file, line)
		lines = append(lines, stackLine)
	}

	return strings.Join(lines, "\n")
}

// IsSDKFrame checks if a frame is from the Treebeard SDK
func IsSDKFrame(file string) bool {
	return strings.Contains(file, "github.com/treebeard/go-sdk") ||
		   strings.Contains(file, "treebeard")
}

// GetFirstNonSDKCaller returns the first caller outside the SDK
func GetFirstNonSDKCaller(skip int) (string, int, string) {
	for i := skip + 1; ; i++ {
		pc, file, line, ok := runtime.Caller(i)
		if !ok {
			return "", 0, ""
		}

		if !IsSDKFrame(file) {
			fn := runtime.FuncForPC(pc)
			funcName := ""
			if fn != nil {
				funcName = fn.Name()
				// Extract just the function name
				if idx := strings.LastIndex(funcName, "."); idx >= 0 {
					funcName = funcName[idx+1:]
				}
			}

			// Extract just the filename
			if idx := strings.LastIndex(file, "/"); idx >= 0 {
				file = file[idx+1:]
			}

			return file, line, funcName
		}
	}
}