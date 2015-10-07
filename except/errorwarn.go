package except

import (
	"fmt"
	"strings"

	config "github.com/banyanops/collector/config"
	blog "github.com/ccpaging/log4go"
)

const ()

// Error logs an error message and generates a config.BanyanUpdate.
func Error(arg0 interface{}, args ...interface{}) {
	if len(args) == 0 {
		blog.Error(arg0)
		s := fmt.Sprintf("ERROR %v", arg0)
		config.BanyanUpdate(s)
	} else {
		var s string
		switch arg0.(type) {
		case string:
			blog.Error(arg0.(string), args...)
			s = fmt.Sprintf("ERROR %s", arg0.(string))
			s = fmt.Sprintf(s, args...)
		default:
			blog.Error(arg0, args...)
			s = fmt.Sprintf("ERROR %v", arg0)
			arr := []interface{}{s}
			arr = append(arr, args...)
			s = fmt.Sprintln(arr...)
		}
		s = strings.TrimRight(s, "\n")
		config.BanyanUpdate(s)
	}
}

// Warn logs a warning message and generates a config.BanyanUpdate.
func Warn(arg0 interface{}, args ...interface{}) {
	if len(args) == 0 {
		blog.Warn(arg0)
		s := fmt.Sprintf("WARN %v", arg0)
		config.BanyanUpdate(s)
	} else {
		var s string
		switch arg0.(type) {
		case string:
			blog.Warn(arg0.(string), args...)
			s = fmt.Sprintf("WARN %s", arg0.(string))
			s = fmt.Sprintf(s, args...)
		default:
			blog.Warn(arg0, args...)
			s = fmt.Sprintf("WARN %v", arg0)
			arr := []interface{}{s}
			arr = append(arr, args...)
			s = fmt.Sprintln(arr...)
		}
		s = strings.TrimRight(s, "\n")
		config.BanyanUpdate(s)
	}
}
