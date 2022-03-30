module github.com/sylabs/singularity/e2e-cli-plugin

go 1.17

require github.com/sylabs/singularity v0.0.0

require (
	github.com/containerd/cgroups v1.0.3 // indirect
	github.com/coreos/go-systemd/v22 v22.3.2 // indirect
	github.com/docker/go-units v0.4.0 // indirect
	github.com/godbus/dbus/v5 v5.0.6 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/opencontainers/runtime-spec v1.0.3-0.20210326190908-1c3f411f0417 // indirect
	github.com/seccomp/containers-golang v0.6.0 // indirect
	github.com/seccomp/libseccomp-golang v0.9.2-0.20210429002308-3879420cc921 // indirect
	github.com/sylabs/sif/v2 v2.4.1 // indirect
	golang.org/x/sys v0.0.0-20220128215802-99c3d69c2c27 // indirect
)

replace github.com/sylabs/singularity => ./singularity_source
