package except

import (
	"fmt"
	"testing"

	blog "github.com/ccpaging/log4go"
)

func TestError(t *testing.T) {
	a := 2
	b := "hello %d"
	fmt.Println("Expected output: 2 hello %d")
	Error(a, b)
	fmt.Println("Expected output: hello 2")
	Error(b, a)
	fmt.Println("Expected output: hello %d")
	Error(b)
}

func TestWarn(t *testing.T) {
	a := 2
	b := "hello %d"
	fmt.Println("Expected output: 2 hello %d")
	Warn(a, b)
	fmt.Println("Expected output: hello 2")
	Warn(b, a)
	fmt.Println("Expected output: hello %d")
	Warn(b)
	blog.Close()
}

/*
func TestFail(t *testing.T) {
	Fail("bye!")
}
*/
