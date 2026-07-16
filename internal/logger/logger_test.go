package logger

import (
	"testing"

	"github.com/rs/zerolog"
)

func TestSetLogLevel(t *testing.T) {
	tests := []struct {
		name  string
		level string
		want  zerolog.Level
	}{
		{name: "info", level: "INFO", want: zerolog.InfoLevel},
		{name: "debug", level: "DEBUG", want: zerolog.DebugLevel},
		{name: "warn", level: "WARN", want: zerolog.WarnLevel},
		{name: "error", level: "ERROR", want: zerolog.ErrorLevel},
		{name: "trace", level: "TRACE", want: zerolog.TraceLevel},
		{name: "unknown disables", level: "NOPE", want: zerolog.Disabled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := &DefaultLogger{}
			l.SetLogLevel(tt.level)

			if l.level != tt.want {
				t.Errorf("SetLogLevel(%q) level = %v, want %v", tt.level, l.level, tt.want)
			}
			// ERROR and WARN must also lower the global level, not just the field,
			// or filtering never takes effect (the bug this guards).
			if tt.want == zerolog.ErrorLevel || tt.want == zerolog.WarnLevel {
				if got := zerolog.GlobalLevel(); got != tt.want {
					t.Errorf("SetLogLevel(%q) global level = %v, want %v", tt.level, got, tt.want)
				}
			}
		})
	}
}
