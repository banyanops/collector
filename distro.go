// distro.go has functions for identifying Linux distribution type and version.
// Currently handles some CentOS, Ubuntu, and Debian versions.
package collector

import (
	"regexp"
	"strings"

	except "github.com/banyanops/collector/except"
)

// DistroMap is a reference that maps each pretty name to the corresponding distribution name.
var DistroMap = map[string]string{
	"Ubuntu 16.10":                               "UBUNTU-yakkety",
	"Ubuntu 16.04":                               "UBUNTU-xenial",
	"Ubuntu 15.10":                               "UBUNTU-wily",
	"Ubuntu 15.04":                               "UBUNTU-vivid",
	"Ubuntu 14.10":                               "UBUNTU-utopic",
	"Ubuntu Utopic Unicorn (development branch)": "UBUNTU-utopic",
	"Ubuntu 13.10":                               "UBUNTU-saucy",
	"Ubuntu 13.04":                               "UBUNTU-raring",
	"Ubuntu 12.10":                               "UBUNTU-quantal",
	"Ubuntu 11.10":                               "UBUNTU-oneiric",
	"Ubuntu 11.04":                               "UBUNTU-natty",
	"Ubuntu 10.10":                               "UBUNTU-maverick",
	"CentOS Linux 7 (Core)":                      "REDHAT-7Server",
	"Debian GNU/Linux 7 (wheezy)":                "DEBIAN-wheezy",
	"Debian 6.0.10":                              "DEBIAN-squeeze",
	"Debian GNU/Linux 8 (jessie)":                "DEBIAN-jessie",
	"Debian GNU/Linux jessie/sid":                "DEBIAN-jessie",
	"Debian GNU/Linux stretch/sid":               "DEBIAN-stretch-sid",
}

// distroRegexp is a map of compiled regular expressions indexed by names.
var distroRegexp = make(map[regexpPattern]*regexp.Regexp)

type regexpPattern int

const (
	rel6z regexpPattern = iota
	rel5z
)

func init() {
	type elem struct {
		name    regexpPattern
		pattern string
	}
	patternList := []elem{
		elem{name: rel6z, pattern: `release 6\.([\d]+)`},
		elem{name: rel5z, pattern: `release 5\.([\d]+)`},
	}

	for _, p := range patternList {
		r, err := regexp.Compile(p.pattern)
		if err != nil {
			except.Fail(err, p.name, p.pattern)
		}
		distroRegexp[p.name] = r
	}
}

// getDistroID takes a distribution "pretty name" as input and returns the corresponding
// distribution ID, or "Unknown" if no match can be found.
func getDistroID(distroName string) string {
	if id, ok := DistroMap[distroName]; ok {
		return id
	}

	//Exceptions to the rule: There are many such cases, so bucketing them together
	if strings.HasPrefix(distroName, `Ubuntu 16.10`) {
		return "UBUNTU-yakkety"
	}
	if strings.HasPrefix(distroName, `Ubuntu 16.04`) {
		return "UBUNTU-xenial"
	}
	if strings.HasPrefix(distroName, `Ubuntu 14.04`) {
		return "UBUNTU-trusty"
	}
	if strings.HasPrefix(distroName, `Ubuntu precise`) {
		return "UBUNTU-precise"
	}
	if strings.HasPrefix(distroName, `Ubuntu 12.04`) {
		return "UBUNTU-precise"
	}
	if strings.HasPrefix(distroName, `Ubuntu 10.04`) {
		return "UBUNTU-lucid"
	}
	if strings.HasPrefix(distroName, `CentOS release 5`) ||
		strings.HasPrefix(distroName, `Red Hat Enterprise Linux Server release 5`) ||
		strings.HasPrefix(distroName, `Red Hat Enterprise Linux Server 5`) {
		m := distroRegexp[rel5z].FindStringSubmatch(distroName)
		if len(m) > 1 {
			return "REDHAT-5Server-5." + m[1]
		}
		return "REDHAT-5Server"
	}
	if strings.HasPrefix(distroName, `CentOS release 6`) ||
		strings.HasPrefix(distroName, `Red Hat Enterprise Linux Server release 6`) ||
		strings.HasPrefix(distroName, `Red Hat Enterprise Linux Server 6`) {
		m := distroRegexp[rel6z].FindStringSubmatch(distroName)
		if len(m) > 1 {
			return "REDHAT-6Server-6." + m[1]
		}
		return "REDHAT-6Server"
	}
	if strings.HasPrefix(distroName, `Red Hat Enterprise Linux Server release 7`) ||
		strings.HasPrefix(distroName, `Red Hat Enterprise Linux Server 7`) {
		return "REDHAT-7Server"
	}
	if strings.HasPrefix(distroName, `Ubuntu Vivid`) {
		return "UBUNTU-vivid"
	}
	if strings.HasPrefix(distroName, `Ubuntu Wily`) {
		return "UBUNTU-wily"
	}

	except.Warn("DISTRO %s UNKNOWN", distroName)
	return "Unknown"
}
