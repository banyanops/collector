package collector

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	config "github.com/banyanops/collector/config"
	blog "github.com/ccpaging/log4go"
)

var (
	// DockerTransport points to the http transport used to connect to the docker unix socket
	DockerTransport *http.Transport
	DockerTLSVerify = true
	DockerProto     = "unix"
	DockerAddr      = dummydomain
)

const (
	// dummydomain is a fake domain name needed to perform HTTP requests to the Docker UNIX socket.
	dummydomain = "example.com"
	// HTTPTIMEOUT is the time to wait for an HTTP request to complete before giving up.
	HTTPTIMEOUT = 32 * time.Second
	// TARGETCONTAINERDIR is the path in the target container where the exported binaries and scripts are located.
	TARGETCONTAINERDIR = "/banyancollector"
)

type HostConfig struct {
	Binds       []string
	Links       []string
	Privileged  bool
	VolumesFrom []string
}

type Container struct {
	User         string
	AttachStdin  bool
	AttachStdout bool
	AttachStderr bool
	Tty          bool
	Env          []string
	Cmd          []string
	Entrypoint   []string
	Image        string
	WorkingDir   string
	HostConfig   HostConfig
}

func NewTLSTransport(hostpath string, certfile, cafile, keyfile string) (transport *http.Transport, err error) {
	cert, err := tls.LoadX509KeyPair(certfile, keyfile)
	if err != nil {
		return
	}

	caCert, err := ioutil.ReadFile(cafile)
	if err != nil {
		return
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
	}
	tlsConfig.BuildNameToCertificate()
	transport = &http.Transport{TLSClientConfig: tlsConfig}
	return
}

// NewDockerTransport creates an HTTP transport to the Docker unix/tcp socket.
func NewDockerTransport(proto, addr string) (tr *http.Transport, e error) {
	// check Docker environment variables
	dockerHost := os.Getenv("DOCKER_HOST")
	if os.Getenv("DOCKER_TLS_VERIFY") == "0" {
		DockerTLSVerify = false
	}
	dockerCertPath := os.Getenv("DOCKER_CERT_PATH")
	if dockerHost == "" {
		DockerProto = proto
		DockerAddr = addr
	} else {
		blog.Info("$DOCKER_HOST env var = %s", dockerHost)
		switch {
		case strings.HasPrefix(dockerHost, "tcp://"):
			blog.Info("Using protocol tcp")
			DockerProto = "tcp"
			DockerAddr = dockerHost[6:]
		case strings.HasPrefix(dockerHost, "unix://"):
			blog.Info("Using protocol unix")
			DockerProto = "unix"
			DockerAddr = dockerHost[6:]
		default:
			blog.Exit("Unexpected value in $DOCKER_HOST:", dockerHost)
		}
	}

	// create transport for unix socket
	if DockerProto != "unix" && DockerProto != "tcp" {
		e = errors.New("Protocol " + DockerProto + " is not yet supported")
		return
	}
	if DockerProto == "unix" {
		tr = &http.Transport{}
		tr.DisableCompression = true
		tr.Dial = func(_, _ string) (net.Conn, error) {
			return net.DialTimeout(DockerProto, DockerAddr, HTTPTIMEOUT)
		}
		return
	}
	if DockerTLSVerify {
		certfile := dockerCertPath + "/cert.pem"
		cafile := dockerCertPath + "/ca.pem"
		keyfile := dockerCertPath + "/key.pem"
		tr, e = NewTLSTransport(DockerAddr, certfile, cafile, keyfile)
		if e != nil {
			blog.Exit(e, "NewTLSTransport")
		}
		return
	}

	tr = &http.Transport{}
	return
}

// doDockerAPI performs an HTTP GET,POST,DELETE operation to the Docker daemon.
func doDockerAPI(tr *http.Transport, operation, apipath string, jsonString []byte,
	XRegistryAuth string) (resp []byte, e error) {
	switch operation {
	case "GET", "POST", "DELETE":
		break
	default:
		e = errors.New("Operation " + operation + " not supported")
		return
	}
	// for unix socket, URL (host.domain) is needed but can be anything
	var host string
	HTTP := "http://"
	if DockerProto == "unix" {
		host = dummydomain
	} else {
		host = DockerAddr
		if DockerTLSVerify {
			HTTP = "https://"
		}
	}
	URL := HTTP + host + apipath
	blog.Debug("doDockerAPI %s", URL)
	req, e := http.NewRequest(operation, URL, bytes.NewBuffer(jsonString))
	if e != nil {
		blog.Error(e, ":doDockerAPI failed to create http request")
		return
	}
	req.Header.Add("Content-Type", "application/json")
	if XRegistryAuth != "" {
		req.Header.Add("X-Registry-Auth", XRegistryAuth)
	}

	//req.Header.Set("Authorization", "Bearer "+authToken)
	client := &http.Client{Transport: tr}
	r, e := client.Do(req)
	if e != nil {
		blog.Error(e, ":doDockerAPI URL", URL, "client request failed")
		return
	}
	defer r.Body.Close()
	resp, e = ioutil.ReadAll(r.Body)
	if e != nil {
		blog.Error(e, ":doDockerAPI URL", URL, "invalid response body")
		return
	}
	if r.StatusCode < 200 || r.StatusCode > 299 {
		e = errors.New("doDockerAPI URL: " + URL + " status code: " + strconv.Itoa(r.StatusCode) +
			"error: " + string(resp))
		return
	}
	return
}

func dockerVersion() (major, minor, revision int, err error) {
	apipath := "/version"
	resp, err := doDockerAPI(DockerTransport, "GET", apipath, []byte{}, "")
	if err != nil {
		blog.Error(err, ": Error in Remote Docker API call: ", apipath)
		return
	}
	var msg struct {
		Version string
	}
	err = json.Unmarshal(resp, &msg)
	if err != nil {
		blog.Error(err, "unmarshal", string(resp))
		return
	}
	version := msg.Version
	arr := strings.Split(version, ".")
	if len(arr) >= 1 {
		major, err = strconv.Atoi(arr[0])
		if err != nil {
			blog.Error(err)
			return
		}
	}
	if len(arr) >= 2 {
		minor, err = strconv.Atoi(arr[1])
		if err != nil {
			blog.Error(err)
			return
		}
	}
	if len(arr) >= 3 {
		revision, err = strconv.Atoi(arr[2])
		if err != nil {
			blog.Error(err)
			return
		}
	}
	return
}

// createCmd returns a json byte slice desribing the container we want to create
func createCmd(imageID ImageIDType, scriptName, staticBinary, dirPath string) (jsonString []byte, err error) {
	var container Container
	container.User = "0"
	container.AttachStdout = true
	container.AttachStderr = true
	container.HostConfig.Binds = []string{config.BANYANHOSTDIR() + "/hosttarget" + ":" + TARGETCONTAINERDIR + ":ro"}
	container.Image = string(imageID)

	container.Entrypoint = []string{TARGETCONTAINERDIR + "/bin/bash-static", "-c"}
	container.Cmd = []string{"PATH=" + TARGETCONTAINERDIR + "/bin" + ":$PATH " + staticBinary + " " + dirPath + "/" + scriptName}
	blog.Info("Executing command: docker %v", container.Cmd)
	return json.Marshal(container)
}

// createContainer makes a docker remote API call to create a container
func createContainer(containerSpec []byte) (containerID string, err error) {
	apipath := "/containers/create"
	resp, err := doDockerAPI(DockerTransport, "POST", apipath, containerSpec, "")
	if err != nil {
		blog.Error(err, ": Error in Remote Docker API call: ", apipath, string(containerSpec))
		return
	}
	blog.Debug("Response from docker remote API call for create: " + string(resp))
	var msg struct {
		Id       string
		Warnings string
	}
	err = json.Unmarshal(resp, &msg)
	if err != nil {
		blog.Error(err, "createContainer resp", string(resp))
		return
	}
	blog.Info("Got ID %s Warnings %s\n", msg.Id, msg.Warnings)
	containerID = msg.Id
	return
}

// startContainer makes a docker remote API call to create a container
func startContainer(containerID string) (jsonOut []byte, err error) {
	apipath := "/containers/" + containerID + "/start"
	resp, err := doDockerAPI(DockerTransport, "POST", apipath, []byte{}, "")
	if err != nil {
		blog.Error(err, ": Error in Remote Docker API call: ", apipath)
		return
	}
	blog.Debug("Response from docker remote API call for start: " + string(resp))
	return
}

// waitContainer makes a docker remote API call to wait for a container to finish running
func waitContainer(containerID string) (statusCode int, err error) {
	apipath := "/containers/" + containerID + "/wait"
	resp, err := doDockerAPI(DockerTransport, "POST", apipath, []byte{}, "")
	if err != nil {
		blog.Error(err, ": Error in Remote Docker API call: ", apipath)
		return
	}
	blog.Debug("Response from docker remote API call for wait: " + string(resp))
	var msg struct {
		StatusCode int
	}
	err = json.Unmarshal(resp, &msg)
	if err != nil {
		blog.Error(err, "waitContainer resp", string(resp))
		return
	}
	blog.Info("Got StatusCode %d\n", msg.StatusCode)
	statusCode = msg.StatusCode
	return
}

// logsContainer makes a docker remote API call to get logs from a container
func logsContainer(containerID string) (output []byte, err error) {
	apipath := "/containers/" + containerID + "/logs?stdout=1"
	resp, err := doDockerAPI(DockerTransport, "GET", apipath, []byte{}, "")
	if err != nil {
		blog.Error(err, ": Error in Remote Docker API call: ", apipath)
		return
	}
	blog.Debug("Response from docker remote API call for logs: " + string(resp))
	for {
		if len(resp) < 8 {
			break
		}
		header := resp[0:8]
		var size int32
		buf := bytes.NewBuffer(header[4:8])
		binary.Read(buf, binary.BigEndian, &size)
		payload := resp[8:(8 + size)]
		// blog.Info(string(frame))
		resp = resp[(8 + size):]
		if header[0] == uint8(1) {
			// 1=stdout: return only the stdout log
			output = append(output, payload...)
		}
	}
	return
}

// removeContainer makes a docker remote API call to remove a container
func removeContainer(containerID string) (resp []byte, err error) {
	apipath := "/containers/" + containerID
	resp, err = doDockerAPI(DockerTransport, "DELETE", apipath, []byte{}, "")
	if err != nil {
		blog.Error(err)
		return
	}
	blog.Debug("Response from docker remote API call for remove: " + string(resp))
	return
}
