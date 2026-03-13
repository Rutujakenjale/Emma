package logging

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"
	"time"
)

type Level int

const (
	DEBUG Level = iota
	INFO
	WARN
	ERROR
)

var minLevel = DEBUG

func Init(levelStr string) {
	levelStr = strings.ToLower(strings.TrimSpace(levelStr))
	switch levelStr {
	case "debug":
		minLevel = DEBUG
	case "info":
		minLevel = INFO
	case "warn", "warning":
		minLevel = WARN
	case "error":
		minLevel = ERROR
	default:
		minLevel = DEBUG
	}
	// ensure output goes to stderr so it plays nicely with containers
	log.SetOutput(os.Stderr)
	log.SetFlags(0) // we print timestamp ourselves
}

func callerInfo(skip int) string {
	_, file, line, ok := runtime.Caller(skip)
	if !ok {
		return "unknown:0"
	}
	return fmt.Sprintf("%s:%d", file, line)
}

func header(skip int) string {
	ts := time.Now().UTC().Format(time.RFC3339Nano)
	return fmt.Sprintf("%s %s", ts, callerInfo(skip+1))
}

func Debugf(format string, args ...interface{}) {
	if minLevel <= DEBUG {
		log.Printf("DEBUG %s %s", header(2), fmt.Sprintf(format, args...))
	}
}

func Infof(format string, args ...interface{}) {
	if minLevel <= INFO {
		log.Printf("INFO  %s %s", header(2), fmt.Sprintf(format, args...))
	}
}

func Warnf(format string, args ...interface{}) {
	if minLevel <= WARN {
		log.Printf("WARN  %s %s", header(2), fmt.Sprintf(format, args...))
	}
}

func Errorf(format string, args ...interface{}) {
	if minLevel <= ERROR {
		log.Printf("ERROR %s %s", header(2), fmt.Sprintf(format, args...))
	}
}

func Fatalf(format string, args ...interface{}) {
	log.Printf("FATAL %s %s", header(2), fmt.Sprintf(format, args...))
	os.Exit(1)
}
