// Testing for registry authentication.
package main

import (
	"fmt"
	"testing"
)

func TestRegAuth(t *testing.T) {
	fmt.Println("TestRegAuth")
	expectedUser, expectedPassword, registry, e := dockerAuth()
	if e != nil {
		t.Fatal(e)
	}
	user, password, _, _ := RegAuth(registry)
	if user != expectedUser {
		t.Fatal("user:", user, "expected:", expectedUser)
	}
	if password != expectedPassword {
		t.Fatal("password:", password, "expected:", expectedPassword)
	}
	return
}
