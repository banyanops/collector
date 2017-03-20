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
	"regexp"
	"strconv"
	"strings"
	"time"

	config "github.com/banyanops/collector/config"
	except "github.com/banyanops/collector/except"
	blog "github.com/ccpaging/log4go"
	uuid "github.com/pborman/uuid"
)

var (
	// DockerClient is the http Client used to connect to the docker daemon endpoint
	DockerClient    *http.Client
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
	// DockerTimeout specifies how long to wait for Docker daemon to finish a task, like pull image.
	DockerTimeout = time.Minute * 10
	// Prefix of container name for image scanning
	ScanContainerNamePrefix = "banyan-collector-image-scan-"
)

type HostConfig struct {
	Binds       []string
	Links       []string
	Privileged  bool
	VolumesFrom []string
}

type ContainerConfig struct {
	User         string
	AttachStdin  bool
	AttachStdout bool
	AttachStderr bool
	Tty          bool
	Env          []string
	Cmd          []string
	Entrypoint   []string
	Image        string
	Labels       map[string]string
	WorkingDir   string
}

type Container struct {
	ContainerConfig
	HostConfig HostConfig
}

type ContainerInspection struct {
	Config     ContainerConfig
	HostConfig HostConfig
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

// NewDockerClient creates an HTTP transport to the Docker unix/tcp socket.
func NewDockerClient(proto, addr string) (client *http.Client, e error) {
	var tr *http.Transport

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
			except.Fail("Unexpected value in $DOCKER_HOST:", dockerHost)
		}
	}

	// create transport for unix socket
	if DockerProto != "unix" && DockerProto != "tcp" {
		e = errors.New("Protocol " + DockerProto + " is not yet supported")
		goto out_err
	}
	if DockerProto == "unix" {
		tr = &http.Transport{}
		tr.DisableCompression = true
		tr.Dial = func(_, _ string) (net.Conn, error) {
			return net.DialTimeout(DockerProto, DockerAddr, HTTPTIMEOUT)
		}
		goto out
	}
	if DockerTLSVerify {
		certfile := dockerCertPath + "/cert.pem"
		cafile := dockerCertPath + "/ca.pem"
		keyfile := dockerCertPath + "/key.pem"
		tr, e = NewTLSTransport(DockerAddr, certfile, cafile, keyfile)
		if e != nil {
			except.Fail(e, "NewTLSTransport")
		}
		goto out
	}

	tr = &http.Transport{}
out:
	client = &http.Client{Transport: tr, Timeout: DockerTimeout}
	return
out_err:
	return
}

// DockerAPI performs an HTTP GET,POST,DELETE operation to the Docker daemon.
func DockerAPI(client *http.Client, operation, apipath string, jsonString []byte,
	XRegistryAuth string) (resp []byte, e error) {
	if client == nil {
		e = errors.New("nil docker client")
		return
	}
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
	blog.Info("DockerAPI %s", URL)
	req, e := http.NewRequest(operation, URL, bytes.NewBuffer(jsonString))
	if e != nil {
		except.Error(e, ":DockerAPI failed to create http request")
		return
	}
	req.Header.Add("Content-Type", "application/json")
	if XRegistryAuth != "" {
		req.Header.Add("X-Registry-Auth", XRegistryAuth)
	}

	//req.Header.Set("Authorization", "Bearer "+authToken)
	// client := &http.Client{Transport: tr, Timeout: DockerTimeout}
	r, e := client.Do(req)
	if e != nil {
		except.Error(e, ":DockerAPI URL", URL, "client request failed")
		return
	}
	defer r.Body.Close()
	resp, e = ioutil.ReadAll(r.Body)
	if e != nil {
		except.Error(e, ":DockerAPI URL", URL, "invalid response body")
		return
	}
	if r.StatusCode < 200 || r.StatusCode > 299 {
		e = errors.New("DockerAPI URL: " + URL + " status code: " + strconv.Itoa(r.StatusCode) +
			"error: " + string(resp))
		return
	}
	return
}

func DockerVersion() (major, minor, revision int, err error) {
	apipath := "/version"
	resp, err := DockerAPI(DockerClient, "GET", apipath, []byte{}, "")
	if err != nil {
		except.Error(err, ": Error in Remote Docker API call: ", apipath)
		return
	}
	var msg struct {
		Version string
	}
	err = json.Unmarshal(resp, &msg)
	if err != nil {
		except.Error(err, "unmarshal", string(resp))
		return
	}
	version := msg.Version
	arr := strings.Split(version, ".")
	if len(arr) >= 1 {
		major, err = strconv.Atoi(arr[0])
		if err != nil {
			except.Error(err)
			return
		}
	}
	if len(arr) >= 2 {
		minor, err = strconv.Atoi(arr[1])
		if err != nil {
			except.Error(err)
			return
		}
	}
	if len(arr) >= 3 {
		field := arr[2]
		var re *regexp.Regexp
		re, err = regexp.Compile(`^(\d+).*$`)
		if err != nil {
			except.Error(err)
			return
		}
		result := re.FindStringSubmatch(field)
		if len(result) < 2 {
			return
		}
		revStr := result[1]
		revision, err = strconv.Atoi(revStr)
		if err != nil {
			except.Error(err)
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

// CreateContainer makes a docker remote API call to create a container.
func CreateContainer(containerSpec []byte) (containerID string, err error) {
	apipath := "/containers/create?name=" + ScanContainerNamePrefix + uuid.New()
	resp, err := DockerAPI(DockerClient, "POST", apipath, containerSpec, "")
	if err != nil {
		except.Error(err, ": Error in Remote Docker API call: ", apipath, string(containerSpec))
		return
	}
	blog.Debug("Response from docker remote API call for create: " + string(resp))
	var msg struct {
		Id       string
		Warnings string
	}
	err = json.Unmarshal(resp, &msg)
	if err != nil {
		except.Error(err, "createContainer resp", string(resp))
		return
	}
	blog.Info("Got ID %s Warnings %s\n", msg.Id, msg.Warnings)
	containerID = msg.Id
	return
}

// StartContainer makes a docker remote API call to start a container.
func StartContainer(containerID string) (jsonOut []byte, err error) {
	apipath := "/containers/" + containerID + "/start"
	resp, err := DockerAPI(DockerClient, "POST", apipath, []byte{}, "")
	if err != nil {
		except.Error(err, ": Error in Remote Docker API call: ", apipath)
		return
	}
	blog.Debug("Response from docker remote API call for start: " + string(resp))
	return
}

// WaitContainer makes a docker remote API call to wait for a container to finish running.
func WaitContainer(containerID string) (statusCode int, err error) {
	apipath := "/containers/" + containerID + "/wait"
	resp, err := DockerAPI(DockerClient, "POST", apipath, []byte{}, "")
	if err != nil {
		except.Error(err, ": Error in Remote Docker API call: ", apipath)
		return
	}
	blog.Debug("Response from docker remote API call for wait: " + string(resp))
	var msg struct {
		StatusCode int
	}
	err = json.Unmarshal(resp, &msg)
	if err != nil {
		except.Error(err, "waitContainer resp", string(resp))
		return
	}
	blog.Info("Got StatusCode %d\n", msg.StatusCode)
	statusCode = msg.StatusCode
	return
}

// LogsContainer makes a docker remote API call to get logs from a container.
func LogsContainer(containerID string) (output []byte, err error) {
	apipath := "/containers/" + containerID + "/logs?stdout=1"
	resp, err := DockerAPI(DockerClient, "GET", apipath, []byte{}, "")
	if err != nil {
		except.Error(err, ": Error in Remote Docker API call: ", apipath)
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

// RemoveContainer makes a docker remote API call to remove a container.
func RemoveContainer(containerID string) (resp []byte, err error) {
	apipath := "/containers/" + containerID
	resp, err = DockerAPI(DockerClient, "DELETE", apipath, []byte{}, "")
	if err != nil {
		except.Error(err)
		return
	}
	blog.Debug("Response from docker remote API call for remove: " + string(resp))
	return
}

// listImages makes a docker remote API call to get a list of images
func listImages() (resp []byte, err error) {
	apipath := "/images/json"
	resp, err = DockerAPI(DockerClient, "GET", apipath, []byte{}, "")
	if err != nil {
		except.Error(err)
		return
	}
	blog.Debug("Response from docker remote API call for list images: " + string(resp))
	return
}

// ListDanglingImages calls Docker to get the list of dangling images, and
// returns a list of their image IDs.
func ListDanglingImages() (imageList []ImageIDType, err error) {
	apipath := `/images/json?filters={"dangling":["true"]}`
	// apipath = "/images/json"
	resp, err := DockerAPI(DockerClient, "GET", apipath, []byte{}, "")
	if err != nil {
		except.Error(err, "ListDanglingImages")
		return
	}

	var localImageList []LocalImageStruct
	if err = json.Unmarshal(resp, &localImageList); err != nil {
		except.Error(err, "ListDanglingImages JSON unmarshal")
		return
	}

	for _, imInfo := range localImageList {
		imageList = append(imageList, ImageIDType(imInfo.ID))
	}
	return
}

// RemoveImageByID calls Docker to remove an image specified by ID.
func RemoveImageByID(image ImageIDType) (resp []byte, err error) {
	apipath := "/images/" + string(image)
	resp, err = DockerAPI(DockerClient, "DELETE", apipath, []byte{}, "")
	if err != nil {
		except.Error(err, "RemoveImageByID")
		return
	}
	return
}

func InspectImage(imageID string) (resp []byte, err error) {
	apipath := "/images/" + imageID + "/json"
	resp, err = DockerAPI(DockerClient, "GET", apipath, []byte{}, "")
	if err != nil {
		except.Error(err)
		return
	}
	blog.Debug("Response from docker remote API call for inspect image " + imageID + " : \n" + string(resp))
	return
}

func InspectContainer(containerID string) (containerSpec ContainerInspection, err error) {
	apipath := "/containers/" + containerID + "/json"
	resp, err := DockerAPI(DockerClient, "GET", apipath, []byte{}, "")
	if err != nil {
		except.Error(err)
		return
	}
	err = json.Unmarshal(resp, &containerSpec)
	return
}
