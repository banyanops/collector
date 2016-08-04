// Copyright (C) 2010, Kyle Lemons <kyle@kylelemons.net>.  All rights reserved.

package log4go

import (
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"path"
	"encoding/json"
)

type kvProperty struct {
	Name  string `xml:"name,attr"`
	Value string `xml:",chardata"`
}

type kvFilter struct {
	Enabled  string        `xml:"enabled,attr"`
	Tag      string        `xml:"tag"`
	Level    string        `xml:"level"`
	Type     string        `xml:"type"`
	Properties []kvProperty `xml:"property"`
}

type Config struct {
	Filters []kvFilter `xml:"filter"`
}

func (log Logger) LoadConfig(filename string) {
	if len(filename) <= 0 {
		return
	}

	// Open the configuration file
	fd, err := os.Open(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "LoadConfig: Error: Could not open %q for reading: %s\n", filename, err)
		os.Exit(1)
	}

	buf, err := ioutil.ReadAll(fd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "LoadConfig: Error: Could not read %q: %s\n", filename, err)
		os.Exit(1)
	}

	log.LoadConfigBuf(filename, buf)
	return
}

func (log Logger) LoadConfigBuf(filename string, buf []byte) {
	ext := path.Ext(filename)
	ext = ext[1:]

	switch ext {
	case "xml":
		log.LoadXMLConfig(filename, buf)
		break
	case "json":
		log.LoadJSONConfig(filename, buf)
		break
	default:
		fmt.Fprintf(os.Stderr, "LoadConfig: Error: Unknown config file type %v. XML or JSON are supported types\n", ext)
	}
}

// Parse Json configuration; see examples/example.json for documentation
func (log Logger) LoadJSONConfig(filename string, contents []byte) {
	log.Close()

	jc := new(Config)
	if err := json.Unmarshal(contents, jc); err != nil {
		fmt.Fprintf(os.Stderr, "LoadConfig: Error: Could not parse Json configuration in %q: %s\n", filename, err)
		os.Exit(1)
	}

	log.ConfigToLogWriter(filename, jc)
}

// Parse XML configuration; see examples/example.xml for documentation
func (log Logger) LoadXMLConfig(filename string, contents []byte) {
	log.Close()

	xc := new(Config)
	if err := xml.Unmarshal(contents, xc); err != nil {
		fmt.Fprintf(os.Stderr, "LoadConfig: Error: Could not parse XML configuration in %q: %s\n", filename, err)
		os.Exit(1)
	}

	log.ConfigToLogWriter(filename, xc)
}

func (log Logger) ConfigToLogWriter(filename string, cfg *Config) {
	for _, kvfilt := range cfg.Filters {
		var lw LogWriter
		var lvl Level
		bad, good, enabled := false, true, false

		// Check required children
		if len(kvfilt.Enabled) == 0 {
			fmt.Fprintf(os.Stderr, "LoadConfig: Error: Required attribute %s for filter missing in %s\n", "enabled", filename)
			bad = true
		} else {
			enabled = kvfilt.Enabled != "false"
		}
		if len(kvfilt.Tag) == 0 {
			fmt.Fprintf(os.Stderr, "LoadConfig: Error: Required child <%s> for filter missing in %s\n", "tag", filename)
			bad = true
		}
		if len(kvfilt.Type) == 0 {
			fmt.Fprintf(os.Stderr, "LoadConfig: Error: Required child <%s> for filter missing in %s\n", "type", filename)
			bad = true
		}
		if len(kvfilt.Level) == 0 {
			fmt.Fprintf(os.Stderr, "LoadConfig: Error: Required child <%s> for filter missing in %s\n", "level", filename)
			bad = true
		}

		switch kvfilt.Level {
		case "FINEST":
			lvl = FINEST
		case "FINE":
			lvl = FINE
		case "DEBUG":
			lvl = DEBUG
		case "TRACE":
			lvl = TRACE
		case "INFO":
			lvl = INFO
		case "WARNING":
			lvl = WARNING
		case "ERROR":
			lvl = ERROR
		case "CRITICAL":
			lvl = CRITICAL
		default:
			fmt.Fprintf(os.Stderr, "LoadConfig: Error: Required child <%s> for filter has unknown value in %s: %s\n", "level", filename, kvfilt.Level)
			bad = true
		}

		// Just so all of the required attributes are errored at the same time if missing
		if bad {
			os.Exit(1)
		}

		switch kvfilt.Type {
		case "console":
			lw, good = propToConsoleLogWriter(filename, kvfilt.Properties, enabled)
		case "file":
			lw, good = propToFileLogWriter(filename, kvfilt.Properties, enabled)
		case "xml":
			lw, good = propToXMLLogWriter(filename, kvfilt.Properties, enabled)
		case "socket":
			lw, good = propToSocketLogWriter(filename, kvfilt.Properties, enabled)
		default:
			fmt.Fprintf(os.Stderr, "LoadConfig: Error: Could not load configuration in %s: unknown filter type \"%s\"\n", filename, kvfilt.Type)
			os.Exit(1)
		}

		// Just so all of the required params are errored at the same time if wrong
		if !good {
			os.Exit(1)
		}

		// If we're disabled (syntax and correctness checks only), don't add to logger
		if !enabled {
			continue
		}

		log[kvfilt.Tag] = NewFilter(lvl, lw)
	}
}

func propToConsoleLogWriter(filename string, props []kvProperty, enabled bool) (*ConsoleLogWriter, bool) {
	color := true
	format := "[%D %T] [%L] (%S) %M"
	// Parse properties
	for _, prop := range props {
		switch prop.Name {
		case "color":
			color = strings.Trim(prop.Value, " \r\n") != "false"
		case "format":
			format = strings.Trim(prop.Value, " \r\n")
		default:
			fmt.Fprintf(os.Stderr, "LoadConfig: Warning: Unknown property \"%s\" for console filter in %s\n", prop.Name, filename)
		}
	}

	// If it's disabled, we're just checking syntax
	if !enabled {
		return nil, true
	}

	clw := NewConsoleLogWriter()
	clw.SetColor(color)
	clw.SetFormat(format)
	return clw, true
}

// Parse a number with K/M/G suffixes based on thousands (1000) or 2^10 (1024)
func strToNumSuffix(str string, mult int) int {
	num := 1
	if len(str) > 1 {
		switch str[len(str)-1] {
		case 'G', 'g':
			num *= mult
			fallthrough
		case 'M', 'm':
			num *= mult
			fallthrough
		case 'K', 'k':
			num *= mult
			str = str[0 : len(str)-1]
		}
	}
	parsed, _ := strconv.Atoi(str)
	return parsed * num
}

func propToFileLogWriter(filename string, props []kvProperty, enabled bool) (*FileLogWriter, bool) {
	file := ""
	format := "[%D %T] [%L] (%S) %M"
	maxlines := 0
	maxsize := 0
	daily := false
	rotate := false
	maxbackup := 999
	maxdays := 0

	// Parse properties
	for _, prop := range props {
		switch prop.Name {
		case "filename":
			file = strings.Trim(prop.Value, " \r\n")
		case "format":
			format = strings.Trim(prop.Value, " \r\n")
		case "maxlines":
			maxlines = strToNumSuffix(strings.Trim(prop.Value, " \r\n"), 1000)
		case "maxsize":
			maxsize = strToNumSuffix(strings.Trim(prop.Value, " \r\n"), 1024)
		case "maxdays":
			maxdays = strToNumSuffix(strings.Trim(prop.Value, " \r\n"), 0)
		case "daily":
			daily = strings.Trim(prop.Value, " \r\n") != "false"
		case "rotate":
			rotate = strings.Trim(prop.Value, " \r\n") != "false"
		case "maxBackup":
			maxbackup = strToNumSuffix(strings.Trim(prop.Value, " \r\n"), 999)
		default:
			fmt.Fprintf(os.Stderr, "LoadConfig: Warning: Unknown property \"%s\" for file filter in %s\n", prop.Name, filename)
		}
	}

	// Check properties
	if len(file) == 0 {
		fmt.Fprintf(os.Stderr, "LoadConfig: Error: Required property \"%s\" for file filter missing in %s\n", "filename", filename)
		return nil, false
	}

	// If it's disabled, we're just checking syntax
	if !enabled {
		return nil, true
	}

	flw := NewFileLogWriter(file)
	if flw == nil {
		return nil, false
	}
	flw.SetFormat(format)
	flw.SetRotate(rotate)
	flw.SetRotateLines(maxlines)
	flw.SetRotateSize(maxsize)
	flw.SetRotateDays(maxdays)
	flw.SetRotateDaily(daily)
	flw.SetRotateBackup(maxbackup)
	return flw, true
}

func propToXMLLogWriter(filename string, props []kvProperty, enabled bool) (*FileLogWriter, bool) {
	file := ""
	maxrecords := 0
	maxsize := 0
	daily := false
	rotate := false

	// Parse properties
	for _, prop := range props {
		switch prop.Name {
		case "filename":
			file = strings.Trim(prop.Value, " \r\n")
		case "maxrecords":
			maxrecords = strToNumSuffix(strings.Trim(prop.Value, " \r\n"), 1000)
		case "maxsize":
			maxsize = strToNumSuffix(strings.Trim(prop.Value, " \r\n"), 1024)
		case "daily":
			daily = strings.Trim(prop.Value, " \r\n") != "false"
		case "rotate":
			rotate = strings.Trim(prop.Value, " \r\n") != "false"
		default:
			fmt.Fprintf(os.Stderr, "LoadConfig: Warning: Unknown property \"%s\" for xml filter in %s\n", prop.Name, filename)
		}
	}

	// Check properties
	if len(file) == 0 {
		fmt.Fprintf(os.Stderr, "LoadConfig: Error: Required property \"%s\" for xml filter missing in %s\n", "filename", filename)
		return nil, false
	}

	// If it's disabled, we're just checking syntax
	if !enabled {
		return nil, true
	}

	xlw := NewXMLLogWriter(file)
	xlw.SetRotate(rotate)
	xlw.SetRotateLines(maxrecords)
	xlw.SetRotateSize(maxsize)
	xlw.SetRotateDaily(daily)
	return xlw, true
}

func propToSocketLogWriter(filename string, props []kvProperty, enabled bool) (*SocketLogWriter, bool) {
	endpoint := ""
	protocol := "udp"

	// Parse properties
	for _, prop := range props {
		switch prop.Name {
		case "endpoint":
			endpoint = strings.Trim(prop.Value, " \r\n")
		case "protocol":
			protocol = strings.Trim(prop.Value, " \r\n")
		default:
			fmt.Fprintf(os.Stderr, "LoadConfig: Warning: Unknown property \"%s\" for file filter in %s\n", prop.Name, filename)
		}
	}

	// Check properties
	if len(endpoint) == 0 {
		fmt.Fprintf(os.Stderr, "LoadConfig: Error: Required property \"%s\" for file filter missing in %s\n", "endpoint", filename)
		return nil, false
	}

	// If it's disabled, we're just checking syntax
	if !enabled {
		return nil, true
	}

	return NewSocketLogWriter(protocol, endpoint), true
}
