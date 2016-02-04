package collector

import (
	"fmt"
	"testing"
)

func TestParseDistro(t *testing.T) {
	fmt.Println("TestParseDistro")
	var tests = []struct {
		pretty   string
		codename string
	}{
		{"Ubuntu 14.04.3 LTS", "UBUNTU-trusty"},
		{"CentOS Linux 7 (Core)", "REDHAT-7Server"},
		{"CentOS release 5.11 (Final)", "REDHAT-5Server-5.11"},
		{"CentOS release 6.7 (Final)", "REDHAT-6Server-6.7"},
		{"CentOS release 6.6 (Final)", "REDHAT-6Server-6.6"},
		{"CentOS Linux 7 (Core)", "REDHAT-7Server"},
	}
	for _, trial := range tests {
		distro := getDistroID(trial.pretty)
		if distro != trial.codename {
			t.Fatal("input:", trial.pretty, "output", distro, "expected:", trial.codename)
		}
		fmt.Println("Found distro: ", distro)
	}
	return
}
