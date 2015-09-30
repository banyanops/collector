package except

import (
	"fmt"

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
		blog.Error(arg0, args...)
		s := fmt.Sprintf("ERROR %v", arg0)
		s = fmt.Sprintf(s, args...)
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
		blog.Warn(arg0, args...)
		s := fmt.Sprintf("WARN %v", arg0)
		s = fmt.Sprintf(s, args...)
		config.BanyanUpdate(s)
	}
}
