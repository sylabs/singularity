// Copyright (c) 2019-2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package e2e

import (
	"strings"
	"testing"

	"github.com/sylabs/singularity/v4/internal/pkg/fakeroot"
	"github.com/sylabs/singularity/v4/internal/pkg/test/tool/require"
	"github.com/sylabs/singularity/v4/internal/pkg/util/user"
)

const (
	userProfile     = "UserProfile"
	rootProfile     = "RootProfile"
	fakerootProfile = "FakerootProfile"

	userNamespaceProfile     = "UserNamespaceProfile"
	rootUserNamespaceProfile = "RootUserNamespaceProfile"
	ociUserProfile           = "OCIUserProfile"
	ociRootProfile           = "OCIRootProfile"
	ociFakerootProfile       = "OCIFakerootProfile"
)

var (
	// UserProfile is the execution profile for a regular user, using the Singularity native runtime.
	UserProfile = NativeProfiles[userProfile]
	// RootProfile is the execution profile for root, using the Singularity native runtime.
	RootProfile = NativeProfiles[rootProfile]
	// FakerootProfile is the execution profile for fakeroot, using the Singularity native runtime.
	FakerootProfile = NativeProfiles[fakerootProfile]
	// UserNamespaceProfile is the execution profile for a regular user and a user namespace, using the Singularity native runtime.
	UserNamespaceProfile = NativeProfiles[userNamespaceProfile]
	// RootUserNamespaceProfile is the execution profile for root and a user namespace, using the Singularity native runtime.
	RootUserNamespaceProfile = NativeProfiles[rootUserNamespaceProfile]
	// OCIUserProfile is the execution profile for a regular user, using Singularity's OCI mode.
	OCIUserProfile = OCIProfiles[ociUserProfile]
	// OCIRootProfile is the execution profile for root, using Singularity's OCI mode.
	OCIRootProfile = OCIProfiles[ociRootProfile]
	// OCIFakerootProfile is the execution profile for fakeroot, using Singularity's OCI mode.
	OCIFakerootProfile = OCIProfiles[ociFakerootProfile]
)

// Profile represents various properties required to run an E2E test
// under a particular user profile. A profile can define if `RunSingularity`
// will run with privileges (`privileged`), if an option flag is injected
// (`singularityOption`), the option injection is also controllable for a
// subset of singularity commands with `optionForCommands`. A profile can
// also set a default current working directory via `defaultCwd`, profile
// like "RootUserNamespace" need to run from a directory owned by root. A
// profile can also have two identities (eg: "Fakeroot" profile), a host
// identity corresponding to user ID `hostUID` and a container identity
// corresponding to user ID `containerUID`.
type Profile struct {
	name              string           // name of the profile
	privileged        bool             // is the profile will run with privileges ?
	hostUID           int              // user ID corresponding to the profile outside container
	containerUID      int              // user ID corresponding to the profile inside container
	defaultCwd        string           // the default current working directory if specified
	requirementsFn    func(*testing.T) // function checking requirements for the profile
	singularityOption string           // option added to singularity command for the profile
	optionForCommands []string         // singularity commands concerned by the option to be added
	oci               bool             // whether the profile uses the OCI low-level runtime
}

// NativeProfiles defines all available profiles for the native singularity runtime
var NativeProfiles = map[string]Profile{
	userProfile: {
		name:              "User",
		privileged:        false,
		hostUID:           origUID,
		containerUID:      origUID,
		defaultCwd:        "",
		requirementsFn:    nil,
		singularityOption: "",
		optionForCommands: []string{},
		oci:               false,
	},
	rootProfile: {
		name:              "Root",
		privileged:        true,
		hostUID:           0,
		containerUID:      0,
		defaultCwd:        "",
		requirementsFn:    nil,
		singularityOption: "",
		optionForCommands: []string{},
		oci:               false,
	},
	fakerootProfile: {
		name:              "Fakeroot",
		privileged:        false,
		hostUID:           origUID,
		containerUID:      0,
		defaultCwd:        "",
		requirementsFn:    fakerootRequirements,
		singularityOption: "--fakeroot",
		optionForCommands: []string{"shell", "exec", "run", "test", "instance start", "build"},
		oci:               false,
	},
	userNamespaceProfile: {
		name:              "UserNamespace",
		privileged:        false,
		hostUID:           origUID,
		containerUID:      origUID,
		defaultCwd:        "",
		requirementsFn:    require.UserNamespace,
		singularityOption: "--userns",
		optionForCommands: []string{"shell", "exec", "run", "test", "instance start"},
		oci:               false,
	},
	rootUserNamespaceProfile: {
		name:              "RootUserNamespace",
		privileged:        true,
		hostUID:           0,
		containerUID:      0,
		defaultCwd:        "/root", // need to run in a directory owned by root
		requirementsFn:    require.UserNamespace,
		singularityOption: "--userns",
		optionForCommands: []string{"shell", "exec", "run", "test", "instance start"},
		oci:               false,
	},
}

// OCIProfiles defines all available profiles for the OCI runtime
var OCIProfiles = map[string]Profile{
	ociUserProfile: {
		name:              "OCIUser",
		privileged:        false,
		hostUID:           origUID,
		containerUID:      origUID,
		defaultCwd:        "",
		requirementsFn:    ociRequirements,
		singularityOption: "--oci",
		optionForCommands: []string{"shell", "exec", "run", "test", "instance start", "pull", "build"},
		oci:               true,
	},
	ociRootProfile: {
		name:              "OCIRoot",
		privileged:        true,
		hostUID:           0,
		containerUID:      0,
		defaultCwd:        "",
		requirementsFn:    ociRequirements,
		singularityOption: "--oci",
		optionForCommands: []string{"shell", "exec", "run", "test", "instance start", "pull", "build"},
		oci:               true,
	},
	ociFakerootProfile: {
		name:              "OCIFakeroot",
		privileged:        false,
		hostUID:           origUID,
		containerUID:      0,
		defaultCwd:        "",
		requirementsFn:    ociRequirements,
		singularityOption: "--oci --fakeroot",
		optionForCommands: []string{"shell", "exec", "run", "test", "instance start", "pull", "build"},
		oci:               true,
	},
}

// AllProfiles is initialized to the union of NativeProfiles and OCIProfiles
func AllProfiles() map[string]Profile {
	ap := map[string]Profile{}
	for k, p := range NativeProfiles {
		ap[k] = p
	}
	for k, p := range OCIProfiles {
		ap[k] = p
	}
	return ap
}

// Privileged returns whether the test should be executed with
// elevated privileges or not.
func (p Profile) Privileged() bool {
	return p.privileged
}

// OCI returns whether the profile is using an OCI runtime, rather than the singularity native runtime.
func (p Profile) OCI() bool {
	return p.oci
}

// Requirements calls the different require.* functions
// necessary for running an E2E test under this profile.
func (p Profile) Requirements(t *testing.T) {
	if p.requirementsFn != nil {
		p.requirementsFn(t)
	}
}

// Args returns the additional arguments, if any, to be passed
// to the singularity command specified by cmd in order to run a
// test under this profile.
func (p Profile) args(cmd []string) []string {
	if p.singularityOption == "" {
		return nil
	}

	command := strings.Join(cmd, " ")

	for _, c := range p.optionForCommands {
		if c == command {
			return strings.Split(p.singularityOption, " ")
		}
	}

	return nil
}

// ContainerUser returns the container user information.
func (p Profile) ContainerUser(t *testing.T) *user.User {
	u, err := user.GetPwUID(uint32(p.containerUID))
	if err != nil {
		t.Fatalf("failed to retrieve user container information for user ID %d: %s", p.containerUID, err)
	}

	return u
}

// HostUser returns the host user information.
func (p Profile) HostUser(t *testing.T) *user.User {
	u, err := user.GetPwUID(uint32(p.hostUID))
	if err != nil {
		t.Fatalf("failed to retrieve user host information for user ID %d: %s", p.containerUID, err)
	}

	return u
}

// In returns true if the specified list of profiles contains
// this profile.
func (p Profile) In(profiles ...Profile) bool {
	for _, pr := range profiles {
		if p.name == pr.name {
			return true
		}
	}

	return false
}

// String provides a string representation of this profile.
func (p Profile) String() string {
	return p.name
}

// fakerootRequirements ensures requirements are satisfied to
// correctly execute commands with the fakeroot profile.
func fakerootRequirements(t *testing.T) {
	require.UserNamespace(t)

	uid := uint32(origUID)

	// check that current user has valid mappings in /etc/subuid
	if _, err := fakeroot.GetIDRange(fakeroot.SubUIDFile, uid); err != nil {
		t.Fatalf("fakeroot configuration error: %s", err)
	}

	// check that current user has valid mappings in /etc/subgid;
	// since that file contains the group mappings for a given user
	// *name*, it is keyed by user name, not by group name. This
	// means that even if we are requesting the *group* mappings, we
	// need to pass the *user* ID.
	if _, err := fakeroot.GetIDRange(fakeroot.SubGIDFile, uid); err != nil {
		t.Fatalf("fakeroot configuration error: %s", err)
	}
}

// ociRequirements ensures requirements are satisfied to correctly execute
// commands with the OCI runtime / profile.
func ociRequirements(t *testing.T) {
	require.Kernel(t, 4, 18) // FUSE in userns
	require.UserNamespace(t)
	require.OneCommand(t, []string{"runc", "crun"})
	require.OneCommand(t, []string{"sqfstar", "tar2sqfs"})

	uid := uint32(origUID)

	// check that current user has valid mappings in /etc/subuid
	if _, err := fakeroot.GetIDRange(fakeroot.SubUIDFile, uid); err != nil {
		t.Fatalf("fakeroot configuration error: %s", err)
	}

	// check that current user has valid mappings in /etc/subgid;
	// since that file contains the group mappings for a given user
	// *name*, it is keyed by user name, not by group name. This
	// means that even if we are requesting the *group* mappings, we
	// need to pass the *user* ID.
	if _, err := fakeroot.GetIDRange(fakeroot.SubGIDFile, uid); err != nil {
		t.Fatalf("fakeroot configuration error: %s", err)
	}
}
