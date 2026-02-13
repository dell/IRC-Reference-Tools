package logging

var Default Logger = New(getLogLevel())

func Info(msg string, keyvals ...any) {
	Default.Info(msg, keyvals...)
}

func Warn(msg string, keyvals ...any) {
	Default.Warn(msg, keyvals...)
}

func Error(msg string, keyvals ...any) {
	Default.Error(msg, keyvals...)
}

func Debug(msg string, keyvals ...any) {
	Default.Debug(msg, keyvals...)
}
