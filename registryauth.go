// registryauth.go has functions for Docker registry authentication and Docker Hub authentication and indexing.
package collector

import (
	"encoding/base64"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	config "github.com/banyanops/collector/config"
	except "github.com/banyanops/collector/except"
	// gcr "github.com/banyanops/collector/gcr"
	flag "github.com/spf13/pflag"
	// "k8s.io/kubernetes/pkg/credentialprovider"
)

const ()

var (
	// HubAPI indicates whether to use the Docker Hub API.
	HubAPI bool
	// LocalHost indicates whether to collect images from local host
	LocalHost     bool
	HTTPSRegistry = flag.Bool("registryhttps", true,
		"Set to false if registry does not need HTTPS (SSL/TLS)")
	AuthRegistry = flag.Bool("registryauth", true,
		"Set to false if registry does not need authentication")
	RegistryProto = flag.String("registryproto", "v2",
		"Select the registry protocol to use: v1, v2, quay")
	RegistryTokenAuthV1 = flag.Bool("registrytokenauthv1", false,
		"Registry uses v1 Token Auth, e.g., original v1 Docker Hub")
	RegistryTLSNoVerify = flag.Bool("registrytlsnoverify", false,
		"True to trust the registry without verifying certificate")
	// registryspec is the host.domainname of the registry
	RegistrySpec string
	// registryAPIURL is the http(s)://[user:password@]host.domainname of the registry
	RegistryAPIURL string
	// XRegistryAuth is the base64-encoded AuthConfig object (for X-Registry-Auth HTTP request header)
	XRegistryAuth string
	// BasicAuth is the base64-encoded Auth field read from $HOME/.dockercfg
	BasicAuth string
	// DockerConfig is the name of the config file containing registry authentication information.
	DockerConfig string
)

func init() {
	toHide := flag.Lookup("registrytokenauthv1")
	if toHide != nil {
		toHide.Hidden = true
	}
}

// DockerConfigJSON is used to decode $HOME/.docker/config.json
type DockerConfigJSON struct {
	Auths DockerAuthSet
}

// DockerAuthSet contains authentication info parsed from $HOME/.dockercfg or $HOME/.docker/config.json
type DockerAuthSet map[string]DockerAuth
type DockerAuth struct {
	Auth  string
	Email string
}

// GetRegistryURL determines the full URL, with or without HTTP Basic Auth, needed to
// access the registry or Docker Hub.
func GetRegistryURL() (URL string, hubAPI bool, BasicAuth string, XRegistryAuth string) {
	basicAuth, fullRegistry, XRegistryAuth := RegAuth(RegistrySpec)
	if *AuthRegistry == true {
		if basicAuth == "" {
			except.Fail("Registry auth could not be determined from docker config.")
		}
		BasicAuth = basicAuth
	}
	if *HTTPSRegistry == false {
		URL = "http://" + RegistrySpec
	} else {
		// HTTPS is required
		if strings.HasPrefix(fullRegistry, "https://") {
			URL = fullRegistry
		} else {
			URL = "https://" + RegistrySpec
		}
		if *RegistryTokenAuthV1 == true {
			// only for original Docker Hub registry, due to be phased out soon.
			hubAPI = true
		}
	}
	return
}

// RegAuth takes as input the name of a registry, and it parses the contents of
// $HOME/.dockercfg or $HOME/.docker/config.json to return the user authentication info and registry URL.
// TODO: Change this to return authConfig instead of user&password, and then
// use X-Registry-Auth in the HTTP request header.
func RegAuth(registry string) (basicAuth, fullRegistry, authConfig string) {
	if *AuthRegistry == false {
		fullRegistry = registry
		return
	}

	if len(DockerConfig) == 0 {
		major, minor, revision, err := DockerVersion()
		if err != nil {
			except.Fail("Could not determine Docker version")
		}
		if major < 1 || (major == 1 && minor <= 2) {
			except.Fail("Unsupported docker version %d.%d.%d", major, minor, revision)
		}
		if major == 1 && minor <= 6 {
			DockerConfig = os.Getenv("HOME") + "/.dockercfg"
		} else {
			DockerConfig = os.Getenv("HOME") + "/.docker/config.json"
		}
	}

	useDotDockerDir := strings.Contains(DockerConfig, ".docker/config.json")

	data, err := ioutil.ReadFile(DockerConfig)
	if err != nil {
		if useDotDockerDir == false {
			except.Fail("Could not read %s", DockerConfig)
		}
		// new .docker/config.json didn't work, so try the old .dockercfg
		except.Error("Could not read %s, trying $HOME/.dockercfg", DockerConfig)
		DockerConfig = os.Getenv("HOME") + "/.dockercfg"
		useDotDockerDir = false
		data, err = ioutil.ReadFile(DockerConfig)
		if err != nil {
			except.Fail("Could not read", DockerConfig)
		}
	}

	var dcj DockerConfigJSON
	var das DockerAuthSet
	if useDotDockerDir {
		err = json.Unmarshal(data, &dcj)
		das = dcj.Auths
	} else {
		err = json.Unmarshal(data, &das)
	}
	if err != nil {
		except.Error(err, "Couldn't JSON unmarshal from docker auth data")
		return
	}
	for r, d := range das {
		if r == registry || r == "https://"+registry || r == "https://"+registry+"/v1/" ||
			(strings.Contains(r, "docker.io") && strings.Contains(registry, "docker.io")) {
			if strings.Contains(registry, "docker.io") {
				// force use default v2 registry for Docker Hub, ugh.
				registry = config.DockerHub
			}
			encData, err := base64.StdEncoding.DecodeString(d.Auth)
			if err != nil {
				except.Error(err, ": error")
				return
			}
			up := strings.SplitN(string(encData), ":", 2)
			if len(up) != 2 {
				except.Error("Invalid auth: %s", string(encData))
				return
			}
			if strings.HasSuffix(registry, "/v1/") {
				registry = registry[0 : len(registry)-4]
			}
			user := up[0]
			password := up[1]
			basicAuth = d.Auth
			fullRegistry = registry
			authConfig = getAuthConfig(user, password, d.Auth, d.Email, r)
			return
		}
	}
	return
}

// globMatch returns true iff registry matches glob.
func globMatch(registry, glob string) bool {
	registryParts := strings.Split(registry, ".")
	globParts := strings.Split(glob, ".")
	if len(globParts) != len(registryParts) {
		return false
	}
	for i, globPart := range globParts {
		matched, e := filepath.Match(globPart, registryParts[i])
		if e != nil {
			return false
		}
		if !matched {
			return false
		}
	}
	return true
}

// AuthConfig is a Registry auth info type
// copied from docker package cliconfig config.go
// and needed to generate the Authorization header for the Docker Remote API.
type AuthConfig struct {
	Username      string `json:"username,omitempty"`
	Password      string `json:"password,omitempty"`
	Auth          string `json:"auth"`
	Email         string `json:"email"`
	ServerAddress string `json:"serveraddress,omitempty"`
}

// getAuthConfig returns the Base64-encoded JSONified AuthConfig struct needed to authorize
// with the Docker Remote API.
func getAuthConfig(user, password, auth, email, registry string) (authConfig string) {
	ac := AuthConfig{
		Username:      user,
		Password:      password,
		Auth:          auth,
		Email:         email,
		ServerAddress: registry,
	}
	jsonString, err := json.Marshal(ac)
	if err != nil {
		except.Fail("Failed to marshal authconfig")
	}
	dst := make([]byte, base64.URLEncoding.EncodedLen(len(jsonString)))
	base64.URLEncoding.Encode(dst, jsonString)
	authConfig = string(dst)
	return
}
