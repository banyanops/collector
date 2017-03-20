// registry.go deals with issuing requests to Docker registries.
package collector

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"

	except "github.com/banyanops/collector/except"
	blog "github.com/ccpaging/log4go"
)

var (
	registryRateLimiters limiterSet
)

type limiterSet struct {
	limiters []chan time.Time
	quitters []chan bool
}

// AddRegistryRateLimiters sets up rate limiter.
// Each rate limiter makes collector issue at most numRequests requests to
// the registry every timePeriod.
func AddRegistryRateLimiter(numRequests int, period time.Duration) (err error) {
	if numRequests <= 0 {
		err = errors.New("Invalid numRequests <= 0")
		return
	}
	if period == 0 {
		err = errors.New("Invalid zero time period")
		return
	}

	limiter := make(chan time.Time, numRequests)
	for i := 0; i < numRequests; i++ {
		limiter <- time.Now()
	}
	quitter := make(chan bool, 1)

	registryRateLimiters.limiters = append(registryRateLimiters.limiters, limiter)
	registryRateLimiters.quitters = append(registryRateLimiters.quitters, quitter)
	// ID = len(registryRateLimiters.limiters) - 1

	go func() {
		for t := range time.Tick(period) {
			select {
			case <-quitter:
				close(limiter)
				return
			default:
			}
			for i := 0; i < numRequests; i++ {
				if limiter == nil {
					return
				}
				select {
				case limiter <- t:
				default:
				}
			}
		}
	}()

	blog.Info("Added registry rate limiter: %d requests every %s", numRequests, period.String())

	return
}

// RegistryLimiterWait blocks until an event is received on each rate limiter.
func RegistryLimiterWait() {
	for _, limiter := range registryRateLimiters.limiters {
		select {
		case <-limiter:
		default:
			blog.Info("Waiting for registry rate limiter...")
			<-limiter
		}
	}
}

// DelRegistryRateLimiters removes all the rate limiters.
func DelRegistryRateLimiters() {
	for i, quitter := range registryRateLimiters.quitters {
		blog.Info("Quitting rate limiter %d", i)
		quitter <- true
		close(quitter)
	}
	registryRateLimiters.limiters = nil
	registryRateLimiters.quitters = nil
}

type HTTPStatusCodeError struct {
	error
	StatusCode int
}

func (s *HTTPStatusCodeError) Error() string {
	return "HTTP Status Code " + strconv.Itoa(s.StatusCode)
}

// RegistryQueryV1 performs an HTTP GET operation from a V1 registry and returns the response.
func RegistryQueryV1(client *http.Client, URL string) (response []byte, e error) {
	RegistryLimiterWait()
	_, _, BasicAuth, XRegistryAuth = GetRegistryURL()
	req, e := http.NewRequest("GET", URL, nil)
	if e != nil {
		return nil, e
	}
	if BasicAuth != "" {
		req.Header.Set("Authorization", "Basic "+BasicAuth)
	}
	r, e := client.Do(req)
	if e != nil {
		return nil, e
	}
	defer r.Body.Close()
	if r.StatusCode < 200 || r.StatusCode > 299 {
		e = &HTTPStatusCodeError{StatusCode: r.StatusCode}
		return
	}
	response, e = ioutil.ReadAll(r.Body)
	if e != nil {
		return
	}
	return
}

// RegistryQueryV2 performs an HTTP GET operation from the registry and returns the response.
// If the initial response has status code 401 Unauthorized and includes WWW-Authenticate header,
// then we follow the directions in that header to get an access token, and finally
// re-issue the initial call to get the final response.
func RegistryQueryV2(client *http.Client, URL string) (response []byte, e error) {
	RegistryLimiterWait()
	_, _, BasicAuth, XRegistryAuth = GetRegistryURL()
	req, e := http.NewRequest("GET", URL, nil)
	if e != nil {
		return nil, e
	}
	req.Header.Set("Authorization", "Basic "+BasicAuth)
	r, e := client.Do(req)
	if e != nil {
		return nil, e
	}
	if r.StatusCode == 401 {
		blog.Debug("Registry Query %s got 401", URL)
		// get the WWW-Authenticate header
		WWWAuth := r.Header.Get("WWW-Authenticate")
		if WWWAuth == "" {
			except.Error("Empty WWW-Authenticate", URL)
			return
		}
		arr := strings.Fields(WWWAuth)
		if len(arr) != 2 {
			e = errors.New("Invalid WWW-Authenticate format for " + WWWAuth)
			except.Error(e)
			return
		}
		authType := arr[0]
		blog.Debug("Authorization type: %s", authType)
		fieldMap := make(map[string]string)
		e = parseAuthenticateFields(arr[1], fieldMap)
		if e != nil {
			except.Error(e)
			return
		}
		r.Body.Close()
		// access the authentication server to get a token
		token, err := queryAuthServerV2(client, fieldMap, BasicAuth)
		if err != nil {
			except.Error(err)
			return nil, err
		}
		// re-issue the original request, this time using the token
		req, e = http.NewRequest("GET", URL, nil)
		if e != nil {
			return nil, e
		}
		req.Header.Set("Authorization", authType+" "+token)
		r, e = client.Do(req)
		if e != nil {
			return nil, e
		}
	}
	defer r.Body.Close()
	if r.StatusCode < 200 || r.StatusCode > 299 {
		e = &HTTPStatusCodeError{StatusCode: r.StatusCode}
		return
	}
	response, e = ioutil.ReadAll(r.Body)
	if e != nil {
		return
	}
	blog.Debug("Registry query succeeded")
	return
}

// Registry V2 authorization server result
type authServerResult struct {
	Token string `json:"token"`
}

/* queryAuthServerV2 retrieves an authorization token from a V2 auth server */
func queryAuthServerV2(client *http.Client, fieldMap map[string]string, BasicAuth string) (token string, e error) {
	authServer := fieldMap["realm"]
	if authServer == "" {
		e = errors.New("No registry token auth server specified")
		return
	}
	blog.Debug("authServer=%s\n", authServer)
	URL := authServer
	first := true
	for key, value := range fieldMap {
		if key != "realm" {
			if first {
				URL = URL + "?"
				first = false
			} else {
				URL = URL + "&"
			}
			URL = URL + key + "=" + value
		}
	}
	blog.Debug("Auth server URL is %s", URL)

	req, e := http.NewRequest("GET", URL, nil)
	if e != nil {
		return
	}
	req.Header.Set("Authorization", "Basic "+BasicAuth)
	r, e := client.Do(req)
	if e != nil {
		return
	}
	defer r.Body.Close()
	if r.StatusCode < 200 || r.StatusCode > 299 {
		e = &HTTPStatusCodeError{StatusCode: r.StatusCode}
		return
	}
	response, e := ioutil.ReadAll(r.Body)
	if e != nil {
		return
	}
	var parsedReply authServerResult
	e = json.Unmarshal(response, &parsedReply)
	if e != nil {
		return
	}
	token = parsedReply.Token
	return token, e
}

func parseAuthenticateFields(s string, fieldMap map[string]string) (e error) {
	fields := strings.Split(s, ",")
	for _, f := range fields {
		arr := strings.Split(f, "=")
		if len(arr) != 2 {
			e = errors.New("Invalid WWW-Auth field format for " + f)
			return
		}
		key := arr[0]
		value := strings.Replace(arr[1], `"`, "", -1)
		fieldMap[key] = value
	}
	return
}
