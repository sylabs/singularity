// Copyright (c) 2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package e2e

import (
	"errors"
	"sync"
	"testing"
)

var (
	privateRepoLoginStatuses = make(map[string]bool)
	privateRepoLoginLocks    sync.Map
)

var (
	ErrAlreadyLoggedIn  = errors.New("attempted to login to private e2e test repo when already logged in")
	ErrAlreadyLoggedOut = errors.New("attempted to logout from private e2e test repo when already logged out")
)

func PrivateRepoLogin(t *testing.T, env TestEnv, profile Profile) error {
	result, _ := privateRepoLoginLocks.LoadOrStore(profile.String(), &sync.Mutex{})
	//nolint:forcetypeassert
	loginLock := result.(*sync.Mutex)
	loginLock.Lock()
	defer loginLock.Unlock()
	if privateRepoLoginStatuses[profile.String()] {
		return ErrAlreadyLoggedIn
	}
	env.RunSingularity(
		t,
		WithProfile(profile),
		WithCommand("registry login"),
		WithArgs("-u", DefaultUsername, "-p", DefaultPassword, env.TestRegistryPrivURI),
		ExpectExit(0),
	)
	privateRepoLoginStatuses[profile.String()] = true
	return nil
}

func PrivateRepoLogout(t *testing.T, env TestEnv, profile Profile) error {
	result, _ := privateRepoLoginLocks.LoadOrStore(profile.String(), &sync.Mutex{})
	//nolint:forcetypeassert
	loginLock := result.(*sync.Mutex)
	loginLock.Lock()
	defer loginLock.Unlock()
	if !privateRepoLoginStatuses[profile.String()] {
		return ErrAlreadyLoggedOut
	}
	env.RunSingularity(
		t,
		WithProfile(profile),
		WithCommand("registry logout"),
		WithArgs(env.TestRegistryPrivURI),
		ExpectExit(0),
	)
	privateRepoLoginStatuses[profile.String()] = false
	return nil
}
