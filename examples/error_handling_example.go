package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	lumberjack "github.com/TreebeardHQ/go-sdk"
)

// Custom error types
var (
	ErrNotFound     = errors.New("item not found")
	ErrInvalidInput = errors.New("invalid input")
)

// BusinessError represents a custom error with additional context
type BusinessError struct {
	Code    string
	Message string
	Cause   error
}

func (e *BusinessError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s (caused by: %s)", e.Code, e.Message, e.Cause.Error())
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *BusinessError) Unwrap() error {
	return e.Cause
}

// Example functions that demonstrate error patterns
func fetchUserData(userID string) (*UserData, error) {
	if userID == "" {
		return nil, ErrInvalidInput
	}
	
	if userID == "404" {
		return nil, ErrNotFound
	}
	
	if userID == "error" {
		// Simulate a wrapped error
		dbErr := errors.New("database connection failed")
		return nil, &BusinessError{
			Code:    "DB_ERROR",
			Message: "failed to fetch user data",
			Cause:   dbErr,
		}
	}
	
	if userID == "panic" {
		panic("simulated panic in fetchUserData")
	}
	
	return &UserData{ID: userID, Name: "John Doe"}, nil
}

type UserData struct {
	ID   string
	Name string
}

func processUserRequest(logger *lumberjack.Logger, userID string) {
	// Example of panic capture pattern
	defer lumberjack.CapturePanic(logger)
	
	// Example of comprehensive error logging
	userData, err := fetchUserData(userID)
	if err != nil {
		// Use the enhanced LogError method for comprehensive error info
		logger.LogError(err, "failed to process user request", 
			"user_id", userID,
			"operation", "fetchUserData",
		)
		return
	}
	
	logger.Info("user request processed successfully",
		"user_id", userData.ID,
		"user_name", userData.Name,
	)
}

// Example of different error handling patterns
func demonstrateErrorPatterns() {
	config := lumberjack.NewConfig().
		WithProjectName("error-handling-example").
		WithDebug(true)

	sdk := lumberjack.Init(config)
	defer sdk.Shutdown(context.Background())

	logger := sdk.Logger()

	fmt.Println("=== Error Handling Patterns Demo ===\n")

	// 1. Handle valid request
	fmt.Println("1. Processing valid user:")
	processUserRequest(logger, "user123")
	time.Sleep(10 * time.Millisecond) // Allow logs to flush

	// 2. Handle invalid input error
	fmt.Println("\n2. Processing invalid input:")
	processUserRequest(logger, "")
	time.Sleep(10 * time.Millisecond)

	// 3. Handle not found error
	fmt.Println("\n3. Processing not found error:")
	processUserRequest(logger, "404")
	time.Sleep(10 * time.Millisecond)

	// 4. Handle wrapped business error
	fmt.Println("\n4. Processing wrapped business error:")
	processUserRequest(logger, "error")
	time.Sleep(10 * time.Millisecond)

	// 5. Handle panic (will be captured and logged, then re-panic)
	fmt.Println("\n5. Processing panic (will crash after logging):")
	func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("Recovered from panic: %v\n", r)
			}
		}()
		processUserRequest(logger, "panic")
	}()

	fmt.Println("\n=== Demo completed ===")
}

func main() {
	demonstrateErrorPatterns()
}