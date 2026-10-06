package productlog

import "time"

func init() {
	for _, operation := range []string{"geometry", "hint", "mark", "clear", "mark_native", "clear_native", "clear_retry"} {
		for _, result := range []string{"normal", "fullscreen", "accepted", "failed", "timeout", "superseded"} {
			for _, bucket := range []string{"lt1ms", "lt10ms", "lt100ms", "lt1s", "ge1s"} {
				allowed["source_presentation_"+operation+"_"+result+"_"+bucket] = true
			}
		}
	}
}

func (l *Logger) Presentation(operation, result string, elapsed time.Duration) {
	bucket := "ge1s"
	switch {
	case elapsed < time.Millisecond:
		bucket = "lt1ms"
	case elapsed < 10*time.Millisecond:
		bucket = "lt10ms"
	case elapsed < 100*time.Millisecond:
		bucket = "lt100ms"
	case elapsed < time.Second:
		bucket = "lt1s"
	}
	l.Event("source_presentation_" + operation + "_" + result + "_" + bucket)
}
