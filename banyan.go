// This file has functions needed to interact with the Banyan service.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"

	blog "github.com/ccpaging/log4go"
	flag "github.com/docker/docker/pkg/mflag"
)

var (
	banyanURL = flag.String([]string{"#-banyanURL"},
		"https://app.banyanops.com/api_server_host_v1", "URL of Banyan API server")
	tokenStore = flag.String([]string{"#-tokenstore"}, BANYANDIR()+"/hostcollector/token",
		"File to record the Banyan Collector auth token")
	hubAPI bool
)

// getTokenStore reads the Banyan Collector auth token from the tokenstore file.
func getTokenStore() (token string, e error) {
	f, e := os.Open(*tokenStore)
	if e != nil {
		return
	}
	defer f.Close()
	r := bufio.NewReader(f)
	token, e = r.ReadString('\n')
	if e != nil {
		return
	}
	return
}

// persistToken saves the Banyan Collector auth token to a file, read on restart of collector.
func persistToken(token string) {
	f, e := os.Create(*tokenStore)
	if e != nil {
		blog.Exit("Failed to open persistent store ", *tokenStore, " for writing: ", e)
	}
	defer f.Close()
	_, e = f.WriteString(token + "\n")
	if e != nil {
		blog.Exit("Failed to write token to persistent store ", *tokenStore, ": ", e)
	}
	return
}

// registerCollector obtains the Banyan Collector auth token for this Collector instance.
// The first time the Collector is registered, it obtains the token from the Banyan service
// by providing the Organization ID string from the environment variable COLLECTOR_ID.
// This value can be obtained by registering an Organization through the Banyan web UI.
// After initial registration is successful, then on subsequent startups of Collector
// the Collector auth token will be retrieved from a local file specified using the
// -tokenstore flag on the command line.
func registerCollector() (token string) {
	banyanWriter := false
	for _, d := range strings.Split(*dests, ",") {
		if d == "banyan" {
			banyanWriter = true
			break
		}
	}
	if !banyanWriter {
		return ""
	}

	// check if the token exists in the persistent store
	if token, e := getTokenStore(); e == nil {
		blog.Info("Found token in persistent token store")
		return token
	}

	ID := os.Getenv("COLLECTOR_ID")
	if ID == "" {
		blog.Exit("Please set the COLLECTOR_ID environment variable " +
			"to your collector ID value (from web interface)")
	}
	URL := *banyanURL + "/register_collector"
	r, e := http.PostForm(URL, url.Values{"ID": {ID}})
	if e != nil {
		blog.Exit("Could not register collector with ", URL, ": ", e)
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		blog.Exit("registerCollector URL ", URL, " http response status code ",
			r.StatusCode, ": ", r.Status)
	}

	response, e := ioutil.ReadAll(r.Body)
	if e != nil {
		blog.Exit("Unable to retrieve authorization token from ", URL, ": ", e)
	}

	var msg struct {
		Message string
	}
	e = json.Unmarshal(response, &msg)
	if e != nil {
		blog.Exit("Auth Token not found in response from ", *banyanURL, ": ", e)
	}
	token = msg.Message
	if token == "" {
		blog.Exit("registerCollector bad empty token")
	}

	persistToken(token)
	blog.Info("Successfully registered collector, token saved in %s", *tokenStore)
	return token
}

// doPostBanyanAPI performs an authenticated HTTP POST operation to send data to the Banyan service.
func doPostBanyanAPI(caller string, authToken string, URL string, jsonString []byte) (e error) {
	blog.Debug("doPostBanyanAPI %s %s", caller, URL)
	req, e := http.NewRequest("POST", URL, bytes.NewBuffer(jsonString))
	if e != nil {
		blog.Error(e, ":doPostBanyanAPI failed to create http request")
		return
	}
	req.Header.Set("Authorization", "Bearer "+authToken)
	client := &http.Client{}
	r, e := client.Do(req)
	if e != nil {
		blog.Error(e, ":doPostBanyanAPI URL", URL, "client request failed")
		return
	}
	defer r.Body.Close()
	resp, e := ioutil.ReadAll(r.Body)
	if e != nil {
		blog.Error(e, ":doPostBanyanAPI URL", URL, "invalid response body")
		return
	}
	if r.StatusCode < 200 || r.StatusCode > 299 {
		blog.Error("doPostBanyanAPI URL: %s, status code: %d, error: %s",
			URL, r.StatusCode, string(resp))
	}
	var msg struct {
		Message string
	}
	e = json.Unmarshal(resp, &msg)
	if e != nil {
		blog.Error(e, "doPostBanyanAPI URL:", URL, "Invalid json response: json=", string(resp))
		return
	}
	return
}
