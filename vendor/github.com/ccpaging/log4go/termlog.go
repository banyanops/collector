// Copyright (C) 2010, Kyle Lemons <kyle@kylelemons.net>.  All rights reserved.

package log4go

import (
	"fmt"
	"io"
	"os"
	"github.com/daviddengcn/go-colortext"
)

var stdout io.Writer = os.Stdout

// This is the standard writer that prints to standard output.
type ConsoleLogWriter struct {
	iow		io.Writer
	color 	bool	
	format 	string
}

// This creates a new ConsoleLogWriter
func NewConsoleLogWriter() *ConsoleLogWriter {
	c := &ConsoleLogWriter{
		iow:	stdout,
		color:	false,
		format: "[%T %D %Z] [%L] (%S) %M",
	}
	return c
}

// Must be called before the first log message is written.
func (c *ConsoleLogWriter) SetColor(color bool) *ConsoleLogWriter {
	c.color = color
	return c
}

// Set the logging format (chainable).  Must be called before the first log
// message is written.
func (c *ConsoleLogWriter) SetFormat(format string) *ConsoleLogWriter {
	c.format = format
	return c
}

func (c *ConsoleLogWriter) Close() {
}

func (c *ConsoleLogWriter) LogWrite(rec *LogRecord) {
	if c.color {
		switch rec.Level {
			case CRITICAL:
				ct.ChangeColor(ct.Red, true, ct.White, false)
			case ERROR:
				ct.ChangeColor(ct.Red, false, 0, false)
			case WARNING:
				ct.ChangeColor(ct.Yellow, false, 0, false)
			case INFO:
				ct.ChangeColor(ct.Green, false, 0, false)
			case DEBUG:
				ct.ChangeColor(ct.Magenta, false, 0, false)
			case TRACE:
				ct.ChangeColor(ct.Cyan, false, 0, false)
			default:
		}
		defer ct.ResetColor()
	}
	fmt.Fprint(c.iow, FormatLogRecord(c.format, rec))
}
