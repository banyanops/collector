package except

import (
	"fmt"
	"testing"

	config "github.com/banyanops/collector/config"
	blog "github.com/ccpaging/log4go"
)

func bup(status ...string) {
	fmt.Print("BANYANUPDATE: ")
	for _, s := range status {
		fmt.Printf("%s ", s)
	}
	fmt.Println()
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
	blog.Close()
}

/*
func TestFail(t *testing.T) {
	Fail("bye!")
}
*/
