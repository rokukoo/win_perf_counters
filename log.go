package win_perf_counters

import (
	"log"
)

type Logger struct {
	Name  string // Name is the plugin name, will be printed in the `[]`.
	Quiet bool
}

// We always want to output at debug level during testing to find issues easier
// func (Logger) Level() telegraf.LogLevel {
// 	return telegraf.Debug
// }

// Adding attributes is not supported by the test-logger
func (Logger) AddAttribute(string, interface{}) {}

func (l Logger) Errorf(format string, args ...interface{}) {
	log.Printf("[ERROR] ["+l.Name+"] "+format, args...)
}

func (l Logger) Error(args ...interface{}) {
	log.Print(append([]interface{}{"[ERROR] [" + l.Name + "] "}, args...)...)
}

func (l Logger) Warnf(format string, args ...interface{}) {
	log.Printf("[WARN] ["+l.Name+"] "+format, args...)
}

func (l Logger) Warn(args ...interface{}) {
	log.Print(append([]interface{}{"[WARN] [" + l.Name + "] "}, args...)...)
}

func (l Logger) Infof(format string, args ...interface{}) {
	if !l.Quiet {
		log.Printf("[INFO] ["+l.Name+"] "+format, args...)
	}
}

func (l Logger) Info(args ...interface{}) {
	if !l.Quiet {
		log.Print(append([]interface{}{"[INFO] [" + l.Name + "] "}, args...)...)
	}
}

func (l Logger) Debugf(format string, args ...interface{}) {
	if !l.Quiet {
		log.Printf("[DEBUG] ["+l.Name+"] "+format, args...)
	}
}

func (l Logger) Debug(args ...interface{}) {
	if !l.Quiet {
		log.Print(append([]interface{}{"[DEBUG] [" + l.Name + "] "}, args...)...)
	}
}

func (l Logger) Tracef(format string, args ...interface{}) {
	if !l.Quiet {
		log.Printf("[TRACE] ["+l.Name+"] "+format, args...)
	}
}

// Trace logs a trace message, patterned after log.Print.
func (l Logger) Trace(args ...interface{}) {
	if !l.Quiet {
		log.Print(append([]interface{}{"[TRACE] [" + l.Name + "] "}, args...)...)
	}
}