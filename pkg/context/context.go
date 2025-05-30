package context

import (
	"context"
	"sync"
)

// Key type for context keys to avoid collisions
type Key string

const (
	TraceIDKey   Key = "tb_trace_id"
	TraceNameKey Key = "tb_trace_name"
	PropsKey     Key = "tb_props"
)

// Context manages logging context using Go's context package
type Context struct {
	mu    sync.RWMutex
	props map[string]interface{}
}

// NewContext creates a new logging context
func NewContext() *Context {
	return &Context{
		props: make(map[string]interface{}),
	}
}

// Set adds a key-value pair to the context
func (c *Context) Set(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.props[key] = value
}

// Get retrieves a value from the context
func (c *Context) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	value, exists := c.props[key]
	return value, exists
}

// GetAll returns a copy of all context properties
func (c *Context) GetAll() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	result := make(map[string]interface{})
	for k, v := range c.props {
		result[k] = v
	}
	return result
}

// Clear removes all properties from the context
func (c *Context) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.props = make(map[string]interface{})
}

// WithTraceID adds a trace ID to a Go context
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, TraceIDKey, traceID)
}

// WithTraceName adds a trace name to a Go context
func WithTraceName(ctx context.Context, traceName string) context.Context {
	return context.WithValue(ctx, TraceNameKey, traceName)
}

// WithProps adds properties to a Go context
func WithProps(ctx context.Context, props map[string]interface{}) context.Context {
	return context.WithValue(ctx, PropsKey, props)
}

// GetTraceID retrieves the trace ID from a Go context
func GetTraceID(ctx context.Context) string {
	if value := ctx.Value(TraceIDKey); value != nil {
		if traceID, ok := value.(string); ok {
			return traceID
		}
	}
	return ""
}

// GetTraceName retrieves the trace name from a Go context
func GetTraceName(ctx context.Context) string {
	if value := ctx.Value(TraceNameKey); value != nil {
		if traceName, ok := value.(string); ok {
			return traceName
		}
	}
	return ""
}

// GetProps retrieves properties from a Go context
func GetProps(ctx context.Context) map[string]interface{} {
	if value := ctx.Value(PropsKey); value != nil {
		if props, ok := value.(map[string]interface{}); ok {
			return props
		}
	}
	return make(map[string]interface{})
}