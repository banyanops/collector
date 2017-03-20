// Testing for registry authentication.
package collector

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
)

func TestRegAuth(t *testing.T) {
	fmt.Println("TestRegAuth")
	var e error
	DockerClient, e = NewDockerClient(DOCKERPROTO, DOCKERADDR)
	if e != nil {
		t.Fatal(e)
	}
	expectedUser, expectedPassword, registry, e := dockerAuth()
	if e != nil {
		t.Fatal(e)
	}
	basicAuth, _, _ := RegAuth(registry)
	decoded, err := base64.StdEncoding.DecodeString(basicAuth)
	if err != nil {
		t.Fatal(err, "Unable to decode basicAuth", basicAuth)
	}
	arr := strings.Split(string(decoded), ":")
	user := arr[0]
	password := arr[1]
	if user != expectedUser {
		t.Fatal("user:", user, "expected:", expectedUser)
	}
	if password != expectedPassword {
		t.Fatal("password:", password, "expected:", expectedPassword)
	}
	return
}
