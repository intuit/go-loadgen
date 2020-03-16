package utility

import easy "github.com/t-tomalak/logrus-easy-formatter"

func GetFormatter(isLineEndsWithNewLine bool) *easy.Formatter {
	var format string

	if isLineEndsWithNewLine {
		format = "%time% %msg%\n"
	} else {
		format = "%time% %msg%"
	}
	var logFormat = &easy.Formatter{
		LogFormat:       format,
		TimestampFormat: "2006-01-02T15:04:05.000-07:00",
	}
	return logFormat
}