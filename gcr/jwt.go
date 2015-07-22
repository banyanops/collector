package gcr

import (
	"io/ioutil"
	"net/http"
	"time"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/credentialprovider"
	blog "github.com/ccpaging/log4go"
	"github.com/golang/glog"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/jwt"
)

const (
	storageReadOnlyScope = "https://www.googleapis.com/auth/devstorage.read_only"
)

type jwtProvider struct {
	path     *string
	config   *jwt.Config
	tokenUrl string
}

var containerRegistryUrls = []string{"container.cloud.google.com", "gcr.io", "*.gcr.io"}
var jprovider *credentialprovider.CachingDockerConfigProvider
var mprovider *containerRegistryProvider

// Enabled implements DockerConfigProvider for the JSON Key based implementation.
func (j *jwtProvider) Enabled() bool {
	if *j.path == "" {
		return false
	}

	data, err := ioutil.ReadFile(*j.path)
	if err != nil {
		glog.Errorf("while reading file %s got %v", *j.path, err)
		return false
	}
	config, err := google.JWTConfigFromJSON(data, storageReadOnlyScope)
	if err != nil {
		glog.Errorf("while parsing %s data got %v", *j.path, err)
		return false
	}

	j.config = config
	if j.tokenUrl != "" {
		j.config.TokenURL = j.tokenUrl
	}
	return true
}

// Provide implements DockerConfigProvider
func (j *jwtProvider) Provide() credentialprovider.DockerConfig {
	cfg := credentialprovider.DockerConfig{}

	ts := j.config.TokenSource(oauth2.NoContext)
	token, err := ts.Token()
	if err != nil {
		glog.Errorf("while exchanging json key %s for access token %v", *j.path, err)
		return cfg
	}
	if !token.Valid() {
		glog.Errorf("Got back invalid token: %v", token)
		return cfg
	}

	entry := credentialprovider.DockerConfigEntry{
		Username: "_token",
		Password: token.AccessToken,
		Email:    j.config.Email,
	}

	// Add our entry for each of the supported container registry URLs
	for _, k := range containerRegistryUrls {
		cfg[k] = entry
	}
	return cfg
}

// JWTInit sets up the JWT provider which uses a GCE service account key to obtain DockerConfig.
func JWTInit(jsonKeyPath string) {
	if jprovider != nil {
		return
	}
	jprovider = &credentialprovider.CachingDockerConfigProvider{
		Provider: &jwtProvider{
			path: &jsonKeyPath,
		},
		Lifetime: 30 * time.Minute,
	}
}

// JWT uses the JWT provider to obtain DockerConfig.
func JWT() credentialprovider.DockerConfig {
	enabled := jprovider.Enabled()
	if !enabled {
		blog.Exit("Failed to enable JWT credential provider")
	}
	DockerConfig := jprovider.Provide()
	/*
		for registry, entry := range DockerConfig {
			fmt.Println("Registry", registry)
			fmt.Println("\tUsername:", entry.Username)
			fmt.Println("\tPassword:", entry.Password)
			fmt.Println("\tEmail:", entry.Email)
			fieldValue := entry.Username + ":" + entry.Password
			fmt.Println("\tAuth:", base64.StdEncoding.EncodeToString([]byte(fieldValue)))
		}
	*/
	return DockerConfig
}

// MetadataInit sets up the GCE instance metadata provider.
func MetadataInit() {
	if mprovider != nil {
		return
	}
	// Never cache this.  The access token is already
	// cached by the metadata service.
	mprovider = &containerRegistryProvider{
		metadataProvider{Client: http.DefaultClient},
	}
}

// Metadata uses the metadata provider to obtain a DockerConfig.
func Metadata() credentialprovider.DockerConfig {
	enabled := mprovider.Enabled()
	if !enabled {
		blog.Exit("Failed to enable GCE metadata provider")
	}
	DockerConfig := mprovider.Provide()
	/*
		for registry, entry := range DockerConfig {
			fmt.Println("Registry", registry)
			fmt.Println("\tUsername:", entry.Username)
			fmt.Println("\tPassword:", entry.Password)
			fmt.Println("\tEmail:", entry.Email)
			fieldValue := entry.Username + ":" + entry.Password
			fmt.Println("\tAuth:", base64.StdEncoding.EncodeToString([]byte(fieldValue)))
		}
	*/
	return DockerConfig
}
