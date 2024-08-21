// Copyright 2015 The Linux Foundation.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Copyright (c) 2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.
//
// This file contains modified code originally taken from:
// github.com/moby/buildkit/tree/v0.12.3/session/auth/authprovider

package auth

import (
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	authutil "github.com/containerd/containerd/remotes/docker/auth"
	remoteserrors "github.com/containerd/containerd/remotes/errors"
	"github.com/docker/cli/cli/config"
	"github.com/gofrs/flock"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/auth"
	"github.com/moby/buildkit/session/auth/authprovider"
	"github.com/moby/buildkit/util/progress/progresswriter"
	"github.com/pkg/errors"
	"github.com/sylabs/singularity/v4/internal/pkg/remote/credential/ociauth"
	"github.com/sylabs/singularity/v4/pkg/sylog"
	"github.com/sylabs/singularity/v4/pkg/util/maps"
	"golang.org/x/crypto/nacl/sign"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	defaultExpiration            = 60
	dockerHubConfigfileKey       = "https://index.docker.io/v1/"
	dockerHubConfigfileCompatKey = "docker.io"
	dockerHubRegistryHost        = "registry-1.docker.io"
)

func NewAuthProvider(authConf *authn.AuthConfig, reqAuthFile string) session.Attachable {
	if authConf != nil {
		return &authProvider{
			authConfigCache: map[string]*authn.AuthConfig{},
			config:          *authConf,
			seeds:           &tokenSeeds{dir: config.Dir()},
			loggerCache:     map[string]struct{}{},
		}
	}

	cf, err := ociauth.ConfigFileFromPath(reqAuthFile)
	if err != nil {
		sylog.Fatalf("While trying to read docker auth config from %q: %v", reqAuthFile, err)
	}
	if maps.HasKey(cf.AuthConfigs, dockerHubConfigfileCompatKey) && !maps.HasKey(cf.AuthConfigs, dockerHubConfigfileKey) {
		cf.AuthConfigs[dockerHubConfigfileKey] = cf.AuthConfigs[dockerHubConfigfileCompatKey]
	}

	return authprovider.NewDockerAuthProvider(cf, nil)
}

type authProvider struct {
	authConfigCache map[string]*authn.AuthConfig
	config          authn.AuthConfig
	seeds           *tokenSeeds
	logger          progresswriter.Logger
	loggerCache     map[string]struct{}

	// The need for this mutex is not well understood.
	// Without it, the docker cli on OS X hangs when
	// reading credentials from docker-credential-osxkeychain.
	// See issue https://github.com/docker/cli/issues/1862
	mu sync.Mutex
}

func (ap *authProvider) SetLogger(l progresswriter.Logger) {
	ap.mu.Lock()
	ap.logger = l
	ap.mu.Unlock()
}

func (ap *authProvider) Register(server *grpc.Server) {
	auth.RegisterAuthServer(server, ap)
}

func (ap *authProvider) FetchToken(ctx context.Context, req *auth.FetchTokenRequest) (rr *auth.FetchTokenResponse, err error) {
	ac, err := ap.getAuthConfig(req.Host)
	if err != nil {
		return nil, err
	}

	// check for statically configured bearer token
	if ac.RegistryToken != "" {
		return toTokenResponse(ac.RegistryToken, time.Time{}, 0), nil
	}

	creds, err := ap.credentials(req.Host)
	if err != nil {
		return nil, err
	}

	to := authutil.TokenOptions{
		Realm:    req.Realm,
		Service:  req.Service,
		Scopes:   req.Scopes,
		Username: creds.Username,
		Secret:   creds.Secret,
	}

	if creds.Secret != "" {
		done := func(progresswriter.SubLogger) error {
			return err
		}
		defer func() {
			err = errors.Wrap(err, "failed to fetch oauth token")
		}()
		ap.mu.Lock()
		name := fmt.Sprintf("[auth] %v token for %s", strings.Join(trimScopePrefix(req.Scopes), " "), req.Host)
		if _, ok := ap.loggerCache[name]; !ok {
			progresswriter.Wrap(name, ap.logger, done)
		}
		ap.mu.Unlock()
		// credential information is provided, use oauth POST endpoint
		resp, err := authutil.FetchTokenWithOAuth(ctx, http.DefaultClient, nil, "buildkit-client", to)
		if err != nil {
			var errStatus remoteserrors.ErrUnexpectedStatus
			if errors.As(err, &errStatus) {
				// Registries without support for POST may return 404 for POST /v2/token.
				// As of September 2017, GCR is known to return 404.
				// As of February 2018, JFrog Artifactory is known to return 401.
				if (errStatus.StatusCode == 405 && to.Username != "") || errStatus.StatusCode == 404 || errStatus.StatusCode == 401 {
					resp, err := authutil.FetchToken(ctx, http.DefaultClient, nil, to)
					if err != nil {
						return nil, err
					}
					return toTokenResponse(resp.Token, resp.IssuedAt, resp.ExpiresIn), nil
				}
			}
			return nil, err
		}
		return toTokenResponse(resp.AccessToken, resp.IssuedAt, resp.ExpiresIn), nil
	}
	// do request anonymously
	resp, err := authutil.FetchToken(ctx, http.DefaultClient, nil, to)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch anonymous token")
	}
	return toTokenResponse(resp.Token, resp.IssuedAt, resp.ExpiresIn), nil
}

func (ap *authProvider) credentials(host string) (*auth.CredentialsResponse, error) {
	ac, err := ap.getAuthConfig(host)
	if err != nil {
		return nil, err
	}
	res := &auth.CredentialsResponse{}
	if ac.IdentityToken != "" {
		res.Secret = ac.IdentityToken
	} else {
		res.Username = ac.Username
		res.Secret = ac.Password
	}
	return res, nil
}

func (ap *authProvider) Credentials(_ context.Context, req *auth.CredentialsRequest) (*auth.CredentialsResponse, error) {
	resp, err := ap.credentials(req.Host)
	if err != nil || resp.Secret != "" {
		ap.mu.Lock()
		defer ap.mu.Unlock()
		_, ok := ap.loggerCache[req.Host]
		ap.loggerCache[req.Host] = struct{}{}
		if !ok {
			return resp, progresswriter.Wrap(fmt.Sprintf("[auth] sharing credentials for %s", req.Host), ap.logger, func(progresswriter.SubLogger) error {
				return err
			})
		}
	}
	return resp, err
}

func (ap *authProvider) GetTokenAuthority(_ context.Context, req *auth.GetTokenAuthorityRequest) (*auth.GetTokenAuthorityResponse, error) {
	key, err := ap.getAuthorityKey(req.Host, req.Salt)
	if err != nil {
		return nil, err
	}

	return &auth.GetTokenAuthorityResponse{PublicKey: key[32:]}, nil
}

func (ap *authProvider) VerifyTokenAuthority(_ context.Context, req *auth.VerifyTokenAuthorityRequest) (*auth.VerifyTokenAuthorityResponse, error) {
	key, err := ap.getAuthorityKey(req.Host, req.Salt)
	if err != nil {
		return nil, err
	}

	priv := new([64]byte)
	copy((*priv)[:], key)

	return &auth.VerifyTokenAuthorityResponse{Signed: sign.Sign(nil, req.Payload, priv)}, nil
}

func (ap *authProvider) getAuthConfig(host string) (*authn.AuthConfig, error) {
	ap.mu.Lock()
	defer ap.mu.Unlock()

	if host == dockerHubRegistryHost {
		host = dockerHubConfigfileKey
	}

	if _, exists := ap.authConfigCache[host]; !exists {
		ap.authConfigCache[host] = &authn.AuthConfig{
			Username:      ap.config.Username,
			Password:      ap.config.Password,
			IdentityToken: ap.config.IdentityToken,
		}
	}

	return ap.authConfigCache[host], nil
}

func (ap *authProvider) getAuthorityKey(host string, salt []byte) (ed25519.PrivateKey, error) {
	if v, err := strconv.ParseBool(os.Getenv("BUILDKIT_NO_CLIENT_TOKEN")); err == nil && v {
		return nil, status.Errorf(codes.Unavailable, "client side tokens disabled")
	}

	creds, err := ap.credentials(host)
	if err != nil {
		return nil, err
	}
	seed, err := ap.seeds.getSeed(host)
	if err != nil {
		return nil, err
	}

	mac := hmac.New(sha256.New, salt)
	if creds.Secret != "" {
		mac.Write(seed)
	}

	sum := mac.Sum(nil)

	return ed25519.NewKeyFromSeed(sum[:ed25519.SeedSize]), nil
}

func toTokenResponse(token string, issuedAt time.Time, expires int) *auth.FetchTokenResponse {
	if expires == 0 {
		expires = defaultExpiration
	}
	resp := &auth.FetchTokenResponse{
		Token:     token,
		ExpiresIn: int64(expires),
	}
	if !issuedAt.IsZero() {
		resp.IssuedAt = issuedAt.Unix()
	}
	return resp
}

func trimScopePrefix(scopes []string) []string {
	out := make([]string, len(scopes))
	for i, s := range scopes {
		out[i] = strings.TrimPrefix(s, "repository:")
	}
	return out
}

type tokenSeeds struct {
	mu  sync.Mutex
	dir string
	m   map[string]seed
}

type seed struct {
	Seed []byte
}

func (ts *tokenSeeds) getSeed(host string) ([]byte, error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if err := os.MkdirAll(ts.dir, 0o755); err != nil {
		return nil, err
	}

	if ts.m == nil {
		ts.m = map[string]seed{}
	}

	l := flock.New(filepath.Join(ts.dir, ".token_seed.lock"))
	if err := l.Lock(); err != nil {
		if !errors.Is(err, syscall.EROFS) && !errors.Is(err, os.ErrPermission) {
			return nil, err
		}
	} else {
		defer l.Unlock()
	}

	fp := filepath.Join(ts.dir, ".token_seed")

	// we include client side randomness to avoid chosen plaintext attack from the daemon side
	dt, err := os.ReadFile(fp)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) && !errors.Is(err, syscall.ENOTDIR) && !errors.Is(err, os.ErrPermission) {
			return nil, err
		}
	} else {
		// ignore error in case of crash during previous marshal
		_ = json.Unmarshal(dt, &ts.m)
	}
	v, ok := ts.m[host]
	if !ok {
		v = seed{Seed: newSeed()}
	}

	ts.m[host] = v

	dt, err = json.MarshalIndent(ts.m, "", "  ")
	if err != nil {
		return nil, err
	}

	if err := os.WriteFile(fp, dt, 0o600); err != nil {
		if !errors.Is(err, syscall.EROFS) && !errors.Is(err, os.ErrPermission) {
			return nil, err
		}
	}
	return v.Seed, nil
}

func newSeed() []byte {
	b := make([]byte, 16)
	rand.Read(b)
	return b
}
