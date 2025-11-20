package utils

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
)

// LogLevel represents the logging level
type LogLevel int

const (
	// DEBUG level - detailed debug information
	DEBUG LogLevel = iota
	// INFO level - general informational messages
	INFO
	// WARN level - warning messages
	WARN
	// ERROR level - error messages
	ERROR
)

var (
	logLevelNames = map[LogLevel]string{
		DEBUG: "DEBUG",
		INFO:  "INFO",
		WARN:  "WARN",
		ERROR: "ERROR",
	}

	logLevelColors = map[LogLevel]*color.Color{
		DEBUG: color.New(color.FgCyan),
		INFO:  color.New(color.FgGreen),
		WARN:  color.New(color.FgYellow),
		ERROR: color.New(color.FgRed),
	}
)

// Logger is a structured logger with color support
type Logger struct {
	level      LogLevel
	mu         sync.Mutex
	fileWriter io.Writer
	logFile    *os.File
	prefix     string
}

var (
	defaultLogger *Logger
	loggerOnce    sync.Once
)

// GetLogger returns the singleton logger instance
func GetLogger() *Logger {
	loggerOnce.Do(func() {
		defaultLogger = &Logger{
			level:  INFO,
			prefix: "",
		}
	})
	return defaultLogger
}

// SetLevel sets the logging level
func (l *Logger) SetLevel(level LogLevel) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// SetLevelFromString sets the logging level from a string
func (l *Logger) SetLevelFromString(levelStr string) {
	levelStr = strings.ToUpper(levelStr)
	switch levelStr {
	case "DEBUG":
		l.SetLevel(DEBUG)
	case "INFO":
		l.SetLevel(INFO)
	case "WARN":
		l.SetLevel(WARN)
	case "ERROR":
		l.SetLevel(ERROR)
	default:
		l.SetLevel(INFO)
	}
}

// SetPrefix sets a prefix for all log messages
func (l *Logger) SetPrefix(prefix string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.prefix = prefix
}

// SetLogFile sets a file to write logs to (in addition to stdout)
func (l *Logger) SetLogFile(filename string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Close previous log file if exists
	if l.logFile != nil {
		l.logFile.Close()
	}

	// Create log directory if it doesn't exist
	logDir := filepath.Dir(filename)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// Open log file
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	l.logFile = file
	l.fileWriter = file
	return nil
}

// Close closes the log file if it's open
func (l *Logger) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.logFile != nil {
		l.logFile.Close()
		l.logFile = nil
		l.fileWriter = nil
	}
}

// log is the internal logging function
func (l *Logger) log(level LogLevel, format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if level < l.level {
		return
	}

	// Get caller information
	_, file, line, ok := runtime.Caller(2)
	if ok {
		file = filepath.Base(file)
	} else {
		file = "unknown"
		line = 0
	}

	// Format timestamp
	timestamp := time.Now().Format("2006-01-02 15:04:05.000")

	// Format message
	message := fmt.Sprintf(format, args...)

	// Build log entry
	levelName := logLevelNames[level]
	caller := fmt.Sprintf("%s:%d", file, line)

	// Console output with colors
	levelColor := logLevelColors[level]
	fmt.Printf("%s [%s] %s - ",
		color.New(color.FgWhite).Sprint(timestamp),
		levelColor.Sprint(levelName),
		color.New(color.FgWhite).Sprint(caller),
	)
	
	if l.prefix != "" {
		fmt.Printf("[%s] ", color.New(color.FgMagenta).Sprint(l.prefix))
	}
	
	fmt.Println(message)

	// File output (without colors)
	if l.fileWriter != nil {
		logEntry := fmt.Sprintf("%s [%s] %s", timestamp, levelName, caller)
		if l.prefix != "" {
			logEntry += fmt.Sprintf(" [%s]", l.prefix)
		}
		logEntry += fmt.Sprintf(" - %s\n", message)
		l.fileWriter.Write([]byte(logEntry))
	}
}

// Debug logs a debug message
func (l *Logger) Debug(format string, args ...interface{}) {
	l.log(DEBUG, format, args...)
}

// Info logs an info message
func (l *Logger) Info(format string, args ...interface{}) {
	l.log(INFO, format, args...)
}

// Warn logs a warning message
func (l *Logger) Warn(format string, args ...interface{}) {
	l.log(WARN, format, args...)
}

// Error logs an error message
func (l *Logger) Error(format string, args ...interface{}) {
	l.log(ERROR, format, args...)
}

// LogRequest logs HTTP request details
func (l *Logger) LogRequest(method, path string, headers map[string]string) {
	l.Info("→ Request: %s %s", method, path)
	if len(headers) > 0 {
		l.Debug("  Headers: %v", headers)
	}
}

// LogResponse logs HTTP response details
func (l *Logger) LogResponse(statusCode int, duration time.Duration) {
	if statusCode >= 200 && statusCode < 300 {
		l.Info("← Response: %d (took %v)", statusCode, duration)
	} else if statusCode >= 400 {
		l.Error("← Response: %d (took %v)", statusCode, duration)
	} else {
		l.Warn("← Response: %d (took %v)", statusCode, duration)
	}
}

// LogJSON logs a JSON object (for debugging)
func (l *Logger) LogJSON(label string, data interface{}) {
	l.Debug("%s: %+v", label, data)
}

// Package-level convenience functions
func Debug(format string, args ...interface{}) {
	GetLogger().Debug(format, args...)
}

func Info(format string, args ...interface{}) {
	GetLogger().Info(format, args...)
}

func Warn(format string, args ...interface{}) {
	GetLogger().Warn(format, args...)
}

func Error(format string, args ...interface{}) {
	GetLogger().Error(format, args...)
}
