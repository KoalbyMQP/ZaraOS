package logger

import (
	"fmt"
	"os"
	"sync"
	"time"
)

const (
	cReset  = "\033[0m"
	cBold   = "\033[1m"
	cRed    = "\033[31m"
	cYellow = "\033[33m"
	cGreen  = "\033[32m"
	cCyan   = "\033[36m"
	cGray   = "\033[90m"
)

type Logger struct {
	mu sync.Mutex
}

func New() *Logger { return &Logger{} }

func (l *Logger) print(levelColor, level, msg string, fields ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	ts := time.Now().Format("15:04:05.000")
	kv := buildFields(fields)
	fmt.Fprintf(os.Stdout, "%s%s%s  %s%-5s%s  %-44s%s\n",
		cGray, ts, cReset,
		levelColor, level, cReset,
		msg, kv,
	)
}

func buildFields(fields []any) string {
	out := ""
	for i := 0; i+1 < len(fields); i += 2 {
		if i > 0 {
			out += "  "
		}
		out += fmt.Sprintf("%s%v%s=%v", cGray, fields[i], cReset, fields[i+1])
	}
	return out
}

func (l *Logger) Info(msg string, fields ...any)  { l.print(cGreen, "INFO", msg, fields...) }
func (l *Logger) Warn(msg string, fields ...any)  { l.print(cYellow, "WARN", msg, fields...) }
func (l *Logger) Error(msg string, fields ...any) { l.print(cRed, "ERROR", msg, fields...) }
func (l *Logger) Debug(msg string, fields ...any) { l.print(cGray, "DEBUG", msg, fields...) }

// Event logs a domain event with distinct cyan styling.
func (l *Logger) Event(name string, fields ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	ts := time.Now().Format("15:04:05.000")
	kv := buildFields(fields)
	label := fmt.Sprintf("[%s]", name)
	fmt.Fprintf(os.Stdout, "%s%s%s  %sEVENT%s  %s%-44s%s%s\n",
		cGray, ts, cReset,
		cCyan+cBold, cReset,
		cCyan+cBold, label, cReset,
		kv,
	)
}

func (l *Logger) Banner(lines ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Println()
	for _, line := range lines {
		fmt.Println(cCyan + cBold + line + cReset)
	}
	fmt.Println()
}
