package lumberjack

import (
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"strings"

	"go.opentelemetry.io/otel/log"
)

// ErrorInfo contains comprehensive error information
type ErrorInfo struct {
	Message    string   `json:"message"`
	Type       string   `json:"type"`
	StackTrace []string `json:"stack_trace,omitempty"`
	Cause      string   `json:"cause,omitempty"`      // Root cause if wrapped
	IsWrapped  bool     `json:"is_wrapped"`
	IsPanic    bool     `json:"is_panic,omitempty"`
}

// ExtractErrorInfo extracts comprehensive error information
func ExtractErrorInfo(err error) *ErrorInfo {
	if err == nil {
		return nil
	}

	info := &ErrorInfo{
		Message: err.Error(),
		Type:    reflect.TypeOf(err).String(),
	}

	// Check if error is wrapped (Go 1.13+)
	if unwrapped := errors.Unwrap(err); unwrapped != nil {
		info.IsWrapped = true
		info.Cause = unwrapped.Error()
	}

	// Capture current stack trace
	info.StackTrace = captureCurrentStack(3) // Skip this function and caller

	return info
}

// ExtractPanicInfo extracts information from a recovered panic
func ExtractPanicInfo(panicValue interface{}) *ErrorInfo {
	info := &ErrorInfo{
		IsPanic: true,
		Type:    reflect.TypeOf(panicValue).String(),
	}

	switch v := panicValue.(type) {
	case error:
		info.Message = v.Error()
		// Extract error info for error-type panics
		if errorInfo := ExtractErrorInfo(v); errorInfo != nil {
			info.StackTrace = errorInfo.StackTrace
			info.IsWrapped = errorInfo.IsWrapped
			info.Cause = errorInfo.Cause
		}
	case string:
		info.Message = v
	default:
		info.Message = fmt.Sprintf("%v", v)
	}

	// If no stack trace extracted, capture current stack
	if len(info.StackTrace) == 0 {
		info.StackTrace = captureCurrentStack(3)
	}

	return info
}

// captureCurrentStack captures the current goroutine's stack trace
func captureCurrentStack(skip int) []string {
	const depth = 32
	var pcs [depth]uintptr
	n := runtime.Callers(skip, pcs[:])

	frames := runtime.CallersFrames(pcs[0:n])
	var stackTrace []string

	for {
		frame, more := frames.Next()
		stackTrace = append(stackTrace, fmt.Sprintf("%s:%d %s", 
			frame.File, frame.Line, frame.Function))
		if !more {
			break
		}
	}

	return stackTrace
}

// This function is no longer needed since we're not using pkg/errors

// CapturePanic captures panic information for reporting and re-panics
func CapturePanic(logger *Logger) {
	if r := recover(); r != nil {
		errorInfo := ExtractPanicInfo(r)
		
		// Log the panic with all error information for reporting
		logger.Error("panic occurred",
			"panic_message", errorInfo.Message,
			"panic_type", errorInfo.Type,
			"stack_trace", strings.Join(errorInfo.StackTrace, "\n"),
			"is_panic", true,
		)
		
		// Always re-panic to preserve normal program behavior
		panic(r)
	}
}

// LogError logs an error with comprehensive information
func (l *Logger) LogError(err error, message string, args ...any) {
	if err == nil {
		return
	}

	errorInfo := ExtractErrorInfo(err)
	
	// Add error information to the log attributes
	logArgs := append(args,
		"error_message", errorInfo.Message,
		"error_type", errorInfo.Type,
		"stack_trace", strings.Join(errorInfo.StackTrace, "\n"),
	)

	if errorInfo.IsWrapped {
		logArgs = append(logArgs, "error_cause", errorInfo.Cause)
	}

	l.Error(message, logArgs...)
}

// AddErrorInfoToLogRecord adds error information to OpenTelemetry log attributes
func AddErrorInfoToLogRecord(errorInfo *ErrorInfo) []log.KeyValue {
	if errorInfo == nil {
		return nil
	}

	attrs := []log.KeyValue{
		log.String("error_message", errorInfo.Message),
		log.String("error_type", errorInfo.Type),
	}

	if len(errorInfo.StackTrace) > 0 {
		attrs = append(attrs, log.String("stack_trace", strings.Join(errorInfo.StackTrace, "\n")))
	}

	if errorInfo.IsWrapped && errorInfo.Cause != "" {
		attrs = append(attrs, log.String("error_cause", errorInfo.Cause))
	}

	if errorInfo.IsPanic {
		attrs = append(attrs, log.Bool("is_panic", true))
	}

	return attrs
}