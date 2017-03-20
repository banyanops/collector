package collector

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	fsutil "github.com/banyanops/collector/fsutil"
)

const (
	DOCKERPROTO = "unix"
	DOCKERADDR  = "/var/run/docker.sock"
)

func TestCreateCmd(t *testing.T) {
	os.Setenv("HOSTNAME", "")
	jsonString, err := createCmd(ImageIDType("imageID"), "scriptName", "staticBinary", "dirPath")
	if err != nil {
		t.Fatal(err)
	}
	dst := bytes.NewBuffer([]byte{})
	json.Indent(dst, jsonString, "", "    ")
	fmt.Println(string(dst.Bytes()))
}

func TestBashScriptRun(t *testing.T) {
	var e error
	DockerClient, e = NewDockerClient(DOCKERPROTO, DOCKERADDR)
	if e != nil {
		t.Fatal(e)
	}
	RegistrySpec = "index.docker.io"
	RegistryAPIURL, HubAPI, BasicAuth, XRegistryAuth = GetRegistryURL()
	metadata := ImageMetadataInfo{
		OtherMetadata: OtherMetadata{
			Repo: "ubuntu",
			Tag:  "latest",
		},
	}
	fmt.Println("TestPullImage %v", metadata)
	PullImage(&metadata)

	PWD := os.Getenv("PWD")
	os.Setenv("BANYAN_HOST_DIR", PWD+"/banyandir")
	fsutil.CreateDirIfNotExist(os.Getenv("BANYAN_HOST_DIR") + "/hosttarget/bin")
	defer os.RemoveAll(os.Getenv("BANYAN_HOST_DIR"))
	fsutil.CreateDirIfNotExist(os.Getenv("BANYAN_HOST_DIR") + "/hosttarget/defaultscripts")
	fsutil.CopyDirTree(os.Getenv("PWD")+"/data/bin/*", os.Getenv("BANYAN_HOST_DIR")+"/hosttarget/bin")
	fsutil.CopyDir(os.Getenv("PWD")+"/data/defaultscripts", os.Getenv("BANYAN_HOST_DIR")+"/hosttarget/defaultscripts")
	bs := newBashScript("pkgextractscript.sh", "/banyancollector/defaultscripts", []string{})
	b, err := bs.Run(ImageIDType("ubuntu"))
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("Run returned", string(b))
}

func TestPostDockerAPI(t *testing.T) {
	client, e := NewDockerClient(DOCKERPROTO, DOCKERADDR)
	if e != nil {
		t.Fatal(e)
	}
	apipath := "/containers/create"
	jsonString := []byte(`{ "Hostname": "", "User": "0", "AttachStdin": false, "AttachStdout": true, ` +
		`"AttachStderr": true, "Tty": false, "Env": null, "Cmd": [ "-c", ` +
		`"PATH=/banyancollector:$PATH /banyancollector/pkgextractscript.sh" ], "Entrypoint": [ ` +
		`"/banyancollector/bash-static" ], "Image": "ubuntu", "WorkingDir": "", "HostConfig": { ` +
		`"Binds": [ "` + os.Getenv("PWD") + `/docker/data:/banyancollector:ro" ], "Links": null, ` +
		`"Privileged": false, "VolumesFrom": null } }`)
	resp, err := DockerAPI(client, "POST", apipath, jsonString, "")
	if err != nil {
		t.Fatal(err)
	}
	var msg struct {
		Id       string
		Warnings string
	}
	e = json.Unmarshal(resp, &msg)
	if e != nil {
		fmt.Println(e, "jsonunmarshal failed for string", string(resp))
		return
	}
	fmt.Printf("Got ID %s Warnings %s\n", msg.Id, msg.Warnings)
}
