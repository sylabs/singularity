// Copyright (c) 2020, Control Command Inc. All rights reserved.
// Copyright (c) 2019-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package endpoint

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	jsonresp "github.com/sylabs/json-resp"
	"github.com/sylabs/singularity/v4/internal/pkg/remote/credential"
	useragent "github.com/sylabs/singularity/v4/pkg/util/user-agent"
)

const defaultTimeout = 10 * time.Second

// Default Sylabs cloud service endpoints.
const (
	// SCSConfigPath is the path to the exposed configuration information of an SCS / Singularity Enterprise instance.
	SCSConfigPath = "/assets/config/config.prod.json"
	// SCSDefaultCloudURI is the primary hostname for Sylabs Singularity Container Services.
	SCSDefaultCloudURI = "cloud.sylabs.io"
	// SCSDefaultLibraryURI is the URI for the library service in SCS.
	SCSDefaultLibraryURI = "https://library.sylabs.io"
	// SCSDefaultKeyserverURI is the URI for the keyserver service in SCS.
	SCSDefaultKeyserverURI = "https://keys.sylabs.io"
	// SCSDefaultBuilderURI is the URI for the remote build service in SCS.
	SCSDefaultBuilderURI = "https://build.sylabs.io"
)

// SCS cloud services - suffixed with 'API' in config.prod.json.
const (
	Consent   = "consent"
	Token     = "token"
	Library   = "library"
	Keystore  = "keystore" // alias for keyserver
	Keyserver = "keyserver"
	Builder   = "builder"
)

// RegistryURIConfigKey is the config key for the library OCI registry URI
const RegistryURIConfigKey = "registryUri"

var errorCodeMap = map[int]string{
	404: "Invalid Credentials",
	500: "Internal Server Error",
}

// ErrStatusNotSupported represents the error returned by
// a service which doesn't support SCS status check.
var ErrStatusNotSupported = errors.New("status not supported")

// Service represents a remote service, accessible at Service.URI
type Service interface {
	// URI returns the URI used to access the remote service.
	URI() string
	// Status returns the status of the remote service, if supported.
	Status() (string, error)
	// configKey returns the value of a requested configuration key, if set.
	configVal(string) string
}

type service struct {
	// cfg holds the serializable service configuration.
	cfg *ServiceConfig
	// configMap holds additional specific service configuration key/val pairs.
	// e.g. `registryURI` most be known for the SCS/Enterprise library service to facilitate OCI-SIF push/pull/
	configMap map[string]string
}

// URI returns the service URI.
func (s *service) URI() string {
	return s.cfg.URI
}

// Status checks the service status and returns the version
// of the corresponding service. An ErrStatusNotSupported is
// returned if the service doesn't support this check.
func (s *service) Status() (version string, err error) {
	if s.cfg.External {
		return "", ErrStatusNotSupported
	}

	client := &http.Client{
		Timeout: (30 * time.Second),
	}

	req, err := http.NewRequest(http.MethodGet, s.cfg.URI+"/version", nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("User-Agent", useragent.Value())

	res, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error making request to server: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("error response from server: %v", res.StatusCode)
	}

	var vRes struct {
		Version string `json:"version"`
	}

	if err := jsonresp.ReadResponse(res.Body, &vRes); err != nil {
		return "", err
	}

	return vRes.Version, nil
}

// configVal returns the value of the specified key (if present), in the
// service's additional known configuration.
func (s *service) configVal(key string) string {
	return s.configMap[key]
}

func (ep *Config) GetAllServices() (map[string][]Service, error) {
	if ep.services != nil {
		return ep.services, nil
	}

	ep.services = make(map[string][]Service)

	client := &http.Client{
		Timeout: defaultTimeout,
	}

	epURL, err := ep.GetURL()
	if err != nil {
		return nil, err
	}

	configURL := epURL + SCSConfigPath

	req, err := http.NewRequest(http.MethodGet, configURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", useragent.Value())

	cacheReader := getCachedConfig(epURL)
	reader := cacheReader

	if cacheReader == nil {
		res, err := client.Do(req) //nolint:bodyclose
		if err != nil {
			return nil, fmt.Errorf("error making request to server: %s", err)
		} else if res.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("error response from server: %s", err)
		}
		reader = res.Body
	}
	defer reader.Close()

	b, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("while reading response body: %v", err)
	}

	var a map[string]map[string]interface{}

	if err := json.Unmarshal(b, &a); err != nil {
		return nil, fmt.Errorf("jsonresp: failed to unmarshal response: %v", err)
	}

	if reader != cacheReader {
		updateCachedConfig(epURL, b)
	}

	for k, v := range a {
		s := strings.TrimSuffix(k, "API")
		uri, ok := v["uri"].(string)
		if !ok {
			continue
		}

		sConfig := &ServiceConfig{
			URI: uri,
			credential: &credential.Config{
				URI:  uri,
				Auth: credential.TokenPrefix + ep.Token,
			},
		}
		sConfigMap := map[string]string{}

		// If the SCS/Enterprise instance reports a service called 'keystore'
		// then override this to 'keyserver', as Singularity uses 'keyserver'
		// internally.
		if s == Keystore {
			s = Keyserver
		}

		// Store the backing OCI registry URI for the library service (if any).
		if s == Library {
			registryURI, ok := v[RegistryURIConfigKey].(string)
			if ok {
				sConfigMap[RegistryURIConfigKey] = registryURI
			}
		}

		ep.services[s] = []Service{
			&service{
				cfg:       sConfig,
				configMap: sConfigMap,
			},
		}
	}

	return ep.services, nil
}

// GetServiceURI returns the URI for the service at the specified SCS endpoint
// Examples of services: consent, build, library, key, token
func (ep *Config) GetServiceURI(service string) (string, error) {
	services, err := ep.GetAllServices()
	if err != nil {
		return "", err
	}

	s, ok := services[service]
	if !ok || len(s) == 0 {
		return "", fmt.Errorf("%v is not a service at endpoint", service)
	} else if s[0].URI() == "" {
		return "", fmt.Errorf("%v service at endpoint failed to provide URI in response", service)
	}

	return s[0].URI(), nil
}

// getServiceConfigVal returns the value for the additional config key associated with service.
func (ep *Config) getServiceConfigVal(service, key string) (string, error) {
	services, err := ep.GetAllServices()
	if err != nil {
		return "", err
	}

	s, ok := services[service]
	if !ok || len(s) == 0 {
		return "", fmt.Errorf("%v is not a service at endpoint", service)
	}
	return s[0].configVal(key), nil
}
