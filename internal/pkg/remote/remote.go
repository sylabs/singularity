// Copyright (c) 2020, Control Command Inc. All rights reserved.
// Copyright (c) 2019-2022, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package remote

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/sylabs/singularity/internal/pkg/buildcfg"
	"github.com/sylabs/singularity/internal/pkg/remote/credential"
	"github.com/sylabs/singularity/internal/pkg/remote/endpoint"
	remoteutil "github.com/sylabs/singularity/internal/pkg/remote/util"
	"github.com/sylabs/singularity/pkg/syfs"
	"github.com/sylabs/singularity/pkg/sylog"
	yaml "gopkg.in/yaml.v3"
)

// ErrNoDefault indicates no default remote being set
var ErrNoDefault = errors.New("no default remote")

const (
	// DefaultRemoteName is the default remote name
	DefaultRemoteName = "SylabsCloud"
)

// SystemConfigPath holds the path to the remote system configuration.
var SystemConfigPath = filepath.Join(buildcfg.SYSCONFDIR, "singularity", syfs.RemoteConfFile)

// Config stores the state of remote endpoint configurations
type Config struct {
	DefaultRemote string                      `yaml:"Active"`
	Remotes       map[string]*endpoint.Config `yaml:"Remotes"`
	Credentials   []*credential.Config        `yaml:"Credentials,omitempty"`

	// set to true when this is the system configuration
	system bool
}

// ReadFrom reads remote configuration from io.Reader
// returns Config populated with remotes
func ReadFrom(r io.Reader) (*Config, error) {
	c := &Config{
		Remotes: make(map[string]*endpoint.Config),
	}

	// check if the reader point to the remote system configuration
	if f, ok := r.(*os.File); ok {
		c.system = f.Name() == SystemConfigPath
	}

	// read all data from r into b
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read from io.Reader: %s", err)
	}

	if len(b) > 0 {
		// If we had data to read in io.Reader, attempt to unmarshal as YAML.
		// Also, it will fail if the YAML file does not have the expected
		// structure.
		dec := yaml.NewDecoder(bytes.NewReader(b))
		dec.KnownFields(true)
		if err := dec.Decode(c); err != nil {
			return nil, fmt.Errorf("failed to decode YAML data from io.Reader: %s", err)
		}
	}
	return c, nil
}

// WriteTo writes the configuration to the io.Writer
// returns and error if write is incomplete
func (c *Config) WriteTo(w io.Writer) (int64, error) {
	yaml, err := yaml.Marshal(c)
	if err != nil {
		return 0, fmt.Errorf("failed to marshall remote config to yaml: %v", err)
	}

	n, err := w.Write(yaml)
	if err != nil {
		return 0, fmt.Errorf("failed to write remote config to io.Writer: %v", err)
	}

	return int64(n), err
}

// CheckForRemoteCollisions will return a name-collision error if there is an
// EndPoint name which exists in both c & sys.
func (c *Config) CheckForRemoteCollisions(sys *Config) error {
	for name := range sys.Remotes {
		eUsr, err := c.GetRemote(name)
		if err == nil && !eUsr.System { // usr & sys name collision
			return fmt.Errorf("name collision while syncing: %s", name)
		}
	}

	return nil
}

// SetDefault sets default remote endpoint or returns an error if it does not exist.
// A remote endpoint can also be set as exclusive.
func (c *Config) SetDefault(name string, makeExclusive bool) error {
	if !c.system && makeExclusive {
		return fmt.Errorf("exclusive can't be set by user")
	}

	// get system remote-endpoint configuration
	cSys, err := GetSysConfig()
	if err != nil {
		return fmt.Errorf("while trying to access system remote-endpoint config: %w", err)
	}

	var exclusive *endpoint.Config
	var prevExclusiveName string
	for name, r := range cSys.Remotes {
		if r.Exclusive {
			if exclusive != nil {
				return fmt.Errorf("internal error: encountered more than one 'exclusive' remote-endpoint: %s and %s", exclusive.URI, r.URI)
			}
			exclusive = r
			prevExclusiveName = name
		}
	}

	if (exclusive != nil) && !c.system {
		return fmt.Errorf("cannot set another remote endpoint as default when a system endpoint is set as 'exclusive'")
	}

	r, ok := c.Remotes[name]
	if !ok {
		r, ok = cSys.Remotes[name]
		if !ok {
			return fmt.Errorf("%s is not a remote", name)
		}
	}

	if exclusive != nil {
		prevExclusive, ok := c.Remotes[prevExclusiveName]
		if !ok {
			return fmt.Errorf("internal error: unable to retrieve exclusive remote-endpoint %q from config", prevExclusiveName)
		}
		prevExclusive.Exclusive = false
	}

	r.Exclusive = makeExclusive
	c.DefaultRemote = name
	return nil
}

// GetDefault returns default remote endpoint or an error
func (c *Config) GetDefault() (*endpoint.Config, error) {
	cSys, err := GetSysConfig()
	if err != nil {
		return nil, fmt.Errorf("while trying to access system remote-endpoint config: %w", err)
	}

	return c.GetDefaultWithSys(cSys)
}

// GetDefault returns default remote endpoint or an error using the cSys
// variable as a pre-read system endpoint configuration.
func (c *Config) GetDefaultWithSys(cSys *Config) (*endpoint.Config, error) {
	if (cSys == nil) || (cSys.DefaultRemote == "") {
		if c.DefaultRemote == "" {
			return endpoint.DefaultEndpointConfig, nil
		}
	} else {
		sysDefault, err := cSys.GetRemote(cSys.DefaultRemote)
		if err != nil {
			return nil, fmt.Errorf("error resolving default system remote-endpoint: %w", err)
		}
		if sysDefault.Exclusive || c.DefaultRemote == "" {
			return sysDefault, nil
		}
	}

	defRemote, err := c.GetRemoteWithSys(c.DefaultRemote, cSys)
	if err != nil {
		return nil, fmt.Errorf("error resolving default user-level remote-endpoint: %w", err)
	}

	return defRemote, nil
}

// Add a new remote endpoint
// returns an error if it already exists
func (c *Config) Add(name string, e *endpoint.Config) error {
	if _, ok := c.Remotes[name]; ok {
		return fmt.Errorf("%s is already a remote", name)
	}

	c.Remotes[name] = e
	return nil
}

// Remove a remote endpoint
// if endpoint is the default, the default is cleared
// returns an error if it does not exist
func (c *Config) Remove(name string) error {
	if r, ok := c.Remotes[name]; !ok {
		return fmt.Errorf("%s is not a remote", name)
	} else if r.System && !c.system {
		return fmt.Errorf("%s is global and can't be removed", name)
	}

	if c.DefaultRemote == name {
		c.DefaultRemote = ""
	}

	delete(c.Remotes, name)
	return nil
}

// GetRemote returns a reference to an existing endpoint
func (c *Config) GetRemote(name string) (*endpoint.Config, error) {
	r, ok := c.Remotes[name]
	if !ok {
		cSys, err := GetSysConfig()
		if err != nil {
			return nil, fmt.Errorf("while trying to access system remote-endpoint config: %w", err)
		}

		return c.GetRemoteWithSys(name, cSys)
	}

	r.SetCredentials(c.Credentials)
	return r, nil
}

// GetRemote returns a reference to an existing endpoint using the cSys
// variable as a pre-read system endpoint configuration.
func (c *Config) GetRemoteWithSys(name string, cSys *Config) (*endpoint.Config, error) {
	r, ok := c.Remotes[name]
	if !ok {
		r, ok = cSys.Remotes[name]
		if !ok {
			return nil, fmt.Errorf("%s is not a remote", name)
		}
	}
	r.SetCredentials(c.Credentials)
	return r, nil
}

// Login validates and stores credentials for a service like Docker/OCI registries
// and keyservers.
func (c *Config) Login(uri, username, password string, insecure bool) error {
	_, err := remoteutil.NormalizeKeyserverURI(uri)
	// if there is no error, we consider it as a keyserver
	if err == nil {
		var keyserverConfig *endpoint.ServiceConfig

		for _, ep := range c.Remotes {
			if keyserverConfig != nil {
				break
			}
			for _, kc := range ep.Keyservers {
				if !kc.External {
					continue
				}
				if remoteutil.SameKeyserver(uri, kc.URI) {
					keyserverConfig = kc
					break
				}
			}
		}
		if keyserverConfig == nil {
			return fmt.Errorf("no external keyserver configuration found for %s", uri)
		} else if keyserverConfig.Insecure && !insecure {
			sylog.Warningf("%s is configured as insecure, forcing insecure flag for login", uri)
			insecure = true
		} else if !keyserverConfig.Insecure && insecure {
			insecure = false
		}
	}

	credConfig, err := credential.Manager.Login(uri, username, password, insecure)
	if err != nil {
		return err
	}

	// Remove any existing remote.yaml entry for the same URI.
	// Older versions of Singularity can create duplicate entries with same URI,
	// so loop must handle removing multiple matches (#214).
	for i := 0; i < len(c.Credentials); i++ {
		cred := c.Credentials[i]
		if remoteutil.SameURI(cred.URI, uri) {
			c.Credentials = append(c.Credentials[:i], c.Credentials[i+1:]...)
			i = -1
		}
	}

	c.Credentials = append(c.Credentials, credConfig)
	return nil
}

// Logout removes previously stored credentials for a service.
func (c *Config) Logout(uri string) error {
	if err := credential.Manager.Logout(uri); err != nil {
		return err
	}
	// Older versions of Singularity can create duplicate entries with same URI,
	// so loop must handle removing multiple matches (#214).
	for i := 0; i < len(c.Credentials); i++ {
		cred := c.Credentials[i]
		if remoteutil.SameURI(cred.URI, uri) {
			c.Credentials = append(c.Credentials[:i], c.Credentials[i+1:]...)
			i = -1
		}
	}
	return nil
}

// Rename an existing remote
// returns an error if it does not exist
func (c *Config) Rename(name, newName string) error {
	if _, ok := c.Remotes[name]; !ok {
		return fmt.Errorf("%s is not a remote", name)
	}

	if _, ok := c.Remotes[newName]; ok {
		return fmt.Errorf("%s is already a remote", newName)
	}

	if c.DefaultRemote == name {
		c.DefaultRemote = newName
	}

	c.Remotes[newName] = c.Remotes[name]
	delete(c.Remotes, name)
	return nil
}

// GetSysConfig returns the system remote-endpoint configuration.
func GetSysConfig() (*Config, error) {
	// opening system config file
	f, err := os.OpenFile(SystemConfigPath, os.O_RDONLY, 0o600)
	if err != nil && os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("while opening remote config file: %s", err)
	}
	defer f.Close()

	// read file contents to config struct
	cSys, err := ReadFrom(f)
	if err != nil {
		return nil, fmt.Errorf("while parsing remote config data: %s", err)
	}

	return cSys, nil
}
