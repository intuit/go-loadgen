package utility

import (
	"github.com/intuit/go-loadgen/constants"
	easy "github.com/t-tomalak/logrus-easy-formatter"
)

type LogProperties struct {
	IsLineEndsWithNewLine bool
	CustomTimestampFormat string
	DisableTimestamp      bool
	Tags                  string
	LogFormat             *easy.Formatter
}

func GetFormatter(logProps *LogProperties) *easy.Formatter {
	var format string

	if !logProps.DisableTimestamp {
		format += "%time%" + " "
	}

	if logProps.Tags != "" {
		format += logProps.Tags + " "
	}

	if logProps.IsLineEndsWithNewLine {
		format += "%msg%\n"
	} else {
		format += "%msg%"
	}

	var timestampFormat = ""
	if logProps.CustomTimestampFormat != "" {
		timestampFormat = logProps.CustomTimestampFormat
	} else {
		timestampFormat = constants.DefaultLogTimestampFormat

	}
	var logFormat = &easy.Formatter{
		LogFormat:       format,
		TimestampFormat: timestampFormat,
	}
	return logFormat
}
