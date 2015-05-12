// Functions in this file invoke Docker registry authentication and Docker Hub authentication and indexing.
package main

import (
	"encoding/base64"
	"encoding/json"
	"io/ioutil"
	"os"
	"strings"

	blog "github.com/ccpaging/log4go"
	flag "github.com/docker/docker/pkg/mflag"
)

const ()

var (
	HTTPSRegistry = flag.Bool([]string{"-registryhttps"}, true,
		"Set to false if registry does not need HTTPS (SSL/TLS)")
	AuthRegistry = flag.Bool([]string{"-registryauth"}, true,
		"Set to false if registry does not need authentication")
	// registryspec is the host.domainname of the registry
	registryspec string
	// registryAPIURL is the http(s)://[user:password@]host.domainname of the registry
	registryAPIURL string
	// XRegistryAuth is the base64-encoded AuthConfig object (for X-Registry-Auth HTTP request header)
	XRegistryAuth string
)

// DockerAuthSet contains authentication info parsed from $HOME/.dockercfg
type DockerAuthSet map[string]DockerAuth
type DockerAuth struct {
	Auth  string
	Email string
}

// getRegistryURL determines the full URL, with or without HTTP Basic Auth, needed to
// access the registry or Docker Hub.
func getRegistryURL() (URL string, hubAPI bool, XRegistryAuth string) {
	user, password, fullRegistry, XRegistryAuth := RegAuth(registryspec)
	if *AuthRegistry == true && user == "" {
		blog.Exit("Registry auth could not be determined from $HOME/.dockercfg")
	}
	if *HTTPSRegistry == false {
		if user != "" {
			URL = "http://" + user + ":" + password + "@" + registryspec
		} else {
			URL = "http://" + registryspec
		}
	} else {
		// HTTPS is required
		if strings.HasPrefix(fullRegistry, "https://") {
			if user != "" {
				URL = "https://" + user + ":" + password + "@" + fullRegistry[8:]
			} else {
				URL = fullRegistry
			}
		} else {
			if user != "" {
				URL = "https://" + user + ":" + password + "@" + registryspec
			} else {
				URL = "https://" + registryspec
			}
		}
		if strings.Contains(URL, "docker.io") {
			hubAPI = true
		}
	}
	return
}

// RegAuth takes as input the name of a registry, and it parses the contents of
// $HOME/.dockercfg to return the user authentication info and registry URL.
// TODO: Change this to return authConfig instead of user&password, and then
// use X-Registry-Auth in the HTTP request header.
func RegAuth(registry string) (user, password, fullRegistry, authConfig string) {
	if *AuthRegistry == false {
		fullRegistry = registry
		return
	}
	home := os.Getenv("HOME")
	data, err := ioutil.ReadFile(home + "/.dockercfg")
	if err != nil {
		blog.Exit("Could not read $HOME/.dockercfg:", home+"/.dockercfg")
	}
	return getRegAuth(data, registry)
}

// getRegAuth parses JSON data (from $HOME/.dockercfg) to get authentication and URL info
// for the registry specified in the parameter list.
func getRegAuth(data []byte, registry string) (user, password, fullRegistry, authConfig string) {
	var das DockerAuthSet
	e := json.Unmarshal(data, &das)
	if e != nil {
		blog.Error(e, "Couldn't JSON unmarshal from docker auth data")
		return
	}
	for r, d := range das {
		if strings.Contains(r, registry) {
			encData, err := base64.StdEncoding.DecodeString(d.Auth)
			if err != nil {
				blog.Error(err, ": error")
				return
			}
			up := strings.Split(string(encData), ":")
			if len(up) != 2 {
				blog.Error("Invalid auth: %s", string(encData))
				return
			}
			if strings.HasSuffix(registry, "/v1/") {
				registry = registry[0 : len(registry)-4]
			}
			user = up[0]
			password = up[1]
			fullRegistry = registry
			authConfig = getAuthConfig(user, password, d.Auth, d.Email, r)
			return
		}
	}
	return
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
		blog.Exit("Failed to marshal authconfig")
	}
	blog.Info(string(jsonString))
	dst := make([]byte, base64.URLEncoding.EncodedLen(len(jsonString)))
	base64.URLEncoding.Encode(dst, jsonString)
	authConfig = string(dst)
	blog.Info("authconfig is %s", authConfig)
	return
}
