package except

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	config "github.com/banyanops/collector/config"
	blog "github.com/ccpaging/log4go"
)

func bup(status ...string) {
	fmt.Print("BANYANUPDATE: ")
	fmt.Println(strings.Join(status, " "))
}

func TestError(t *testing.T) {
	config.BanyanUpdate = bup
	a := 2
	b := "hello %d"
	fmt.Println("Expected output: 2 hello %d")
	Error(a, b)
	fmt.Println()
	fmt.Println("Expected output: hello 2")
	Error(b, a)
	fmt.Println()
	fmt.Println("Expected output: hello %d")
	Error(b)
	fmt.Println()

	e := errors.New("An error message")
	fmt.Println("Expected output: An error message hello %d")
	Error(e, b)
	fmt.Println()

}

func TestWarn(t *testing.T) {
	config.BanyanUpdate = bup
	a := 2
	b := "hello %d"
	fmt.Println("Expected output: 2 hello %d")
	Warn(a, b)
	fmt.Println()
	fmt.Println("Expected output: hello 2")
	Warn(b, a)
	fmt.Println()
	fmt.Println("Expected output: hello %d")
	Warn(b)
	fmt.Println()

	e := errors.New("An error message")
	imageList := "imageList"
	fmt.Println("Expected output: An error message: Error in opening imageList: perhaps a fresh start?")
	Warn(e, ": Error in opening", imageList, ": perhaps a fresh start?")
	fmt.Println()
	blog.Close()
}

/*
func TestFail(t *testing.T) {
	Fail("bye!")
}
*/
