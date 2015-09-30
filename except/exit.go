// Package except defines functions that log a warning or error message, and for fatal errors will quit program execution with an exit status.
package except

import (
	"os"

	blog "github.com/ccpaging/log4go"
)

const (
	// ErrorExitStatus is the default exit status in an error condition.
	ErrorExitStatus = 4
)

// Fail prints an error message at blog.ERROR level and then quits with exit status ErrorExitStatus.
func Fail(arg0 interface{}, args ...interface{}) {
	if len(args) == 0 {
		blog.Error(arg0)
	} else {
		blog.Error(arg0, args...)
	}
	blog.Close()
	os.Exit(ErrorExitStatus)
}
