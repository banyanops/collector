// distro.go has functions for identifying Linux distribution type and version.
// Currently handles some CentOS, Ubuntu, and Debian versions.
package collector

import (
	"strings"

	blog "github.com/ccpaging/log4go"
)

// DistroMap is a reference that maps each pretty name to the corresponding distribution name.
var DistroMap = map[string]string{
	"Ubuntu 15.04":                               "UBUNTU-vivid",
	"Ubuntu 14.10":                               "UBUNTU-utopic",
	"Ubuntu Utopic Unicorn (development branch)": "UBUNTU-utopic",
	"Ubuntu 14.04.2 LTS":                         "UBUNTU-trusty",
	"Ubuntu 14.04.1 LTS":                         "UBUNTU-trusty",
	"Ubuntu 14.04 LTS":                           "UBUNTU-trusty",
	"Ubuntu 12.04 LTS":                           "UBUNTU-precise",
	"Ubuntu precise (12.04.5 LTS)":               "UBUNTU-precise",
	"Ubuntu precise (12.04.4 LTS)":               "UBUNTU-precise",
	"Ubuntu precise (12.04.3 LTS)":               "UBUNTU-precise",
	"Ubuntu precise (12.04.2 LTS)":               "UBUNTU-precise",
	"Ubuntu precise (12.04.1 LTS)":               "UBUNTU-precise",
	"Ubuntu 10.04.1 LTS":                         "UBUNTU-lucid",
	"Ubuntu 10.04.2 LTS":                         "UBUNTU-lucid",
	"Ubuntu 10.04.3 LTS":                         "UBUNTU-lucid",
	"Ubuntu 10.04.4 LTS":                         "UBUNTU-lucid",
	"Ubuntu 13.10":                               "UBUNTU-saucy",
	"Ubuntu 13.04":                               "UBUNTU-raring",
	"Ubuntu 12.10":                               "UBUNTU-quantal",
	"Ubuntu 11.10":                               "UBUNTU-oneiric",
	"Ubuntu 11.04":                               "UBUNTU-natty",
	"Ubuntu 10.10":                               "UBUNTU-maverick",
	"Ubuntu 10.04":                               "UBUNTU-lucid",
	"CentOS Linux 7 (Core)":                      "REDHAT-7Server",
	"Debian GNU/Linux 7 (wheezy)":                "DEBIAN-wheezy",
	"Debian 6.0.10":                              "DEBIAN-squeeze",
	"Debian GNU/Linux 8 (jessie)":                "DEBIAN-jessie",
}

// getDistroID takes a distribution "pretty name" as input and returns the corresponding
// distribution ID, or "Unknown" if no match can be found.
func getDistroID(distroName string) string {
	if id, ok := DistroMap[distroName]; ok {
		return id
	}

	//Exceptions to the rule: There are many such cases, so bucketing them together
	if strings.HasPrefix(distroName, `CentOS release 5`) {
		return "REDHAT-5Server"
	}
	if strings.HasPrefix(distroName, `CentOS release 6`) {
		return "REDHAT-6Server"
	}
	if strings.HasPrefix(distroName, `Ubuntu Vivid`) {
		return "UBUNTU-vivid"
	}

	blog.Warn("DISTRO ", distroName, " : UNKNOWN")
	return "Unknown"
}
