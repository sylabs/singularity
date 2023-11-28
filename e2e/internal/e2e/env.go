// Copyright (c) 2019, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.

package e2e

// TestEnv stores all the information under the control of e2e test developers,
// from specifying which Singularity binary to use to controlling how Singularity
// environment variables will be set.
type TestEnv struct {
	CmdPath                  string // Path to the Singularity binary to use for the execution of a Singularity command
	ImagePath                string // Path to the image that has to be used for the execution of a Singularity command
	OrasTestImage            string // URI to SIF image pushed into local registry with ORAS
	OrasTestOCISIF           string // URI to OCI-SIF image pushed into local registry with ORAS
	OrasTestPrivImage        string // URI to SIF image pushed into local registry with ORAS
	OCIArchivePath           string // Path to test OCI archive tar file
	OCILayoutPath            string // Path to test OCI layout directory
	OCISIFPath               string // Path to test OCI-SIF file
	DockerArchivePath        string // Path to test Docker archive tar file
	TestDir                  string // Path to the directory from which a Singularity command needs to be executed
	TestRegistry             string // Host:Port of local registry
	TestRegistryPrivPath     string // Host:Port of local registry + path to private location
	TestRegistryPrivURI      string // Transport (docker://) + Host:Port of local registry + path to private location
	TestRegistryImage        string // URI to single layer OCI image pushed into local registry
	TestRegistryLayeredImage string // URI to 7-layer OCI image pushed into local registry
	TestRegistryPrivImage    string // URI to OCI image pushed into private location in local registry
	TestRegistryOCISIF       string // URI to OCI SIF pushed into local registry as OCI image (non-oras)
	KeyringDir               string // KeyringDir sets the directory where the keyring will be created for the execution of a command (instead of using SINGULARITY_SYPGPDIR which should be avoided when running e2e tests)
	PrivCacheDir             string // PrivCacheDir sets the location of the image cache to be used by the Singularity command to be executed as root (instead of using SINGULARITY_CACHE_DIR which should be avoided when running e2e tests)
	UnprivCacheDir           string // UnprivCacheDir sets the location of the image cache to be used by the Singularity command to be executed as the unpriv user (instead of using SINGULARITY_CACHE_DIR which should be avoided when running e2e tests)
	RunDisabled              bool
	DisableCache             bool // DisableCache can be set to disable the cache during the execution of a e2e command
}
