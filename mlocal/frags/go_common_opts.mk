# go tool default build options
GO111MODULE := on
GO_TAGS := sylog singularity_engine fakeroot_engine
GO_TAGS_SUID := sylog singularity_engine fakeroot_engine
GO_LDFLAGS :=
# Need to use non-pie build on ppc64le
# https://github.com/hpcng/singularity/issues/5762
# Need to disable race detector on ppc64le
# https://github.com/hpcng/singularity/issues/5914
uname_m := $(shell uname -m)
ifeq ($(uname_m),$(filter $(uname_m),ppc64le i386 i686))
GO_BUILDMODE := -buildmode=default
GO_RACE :=
else
GO_BUILDMODE := -buildmode=pie
GO_RACE := -race
endif
GO_MODFLAGS := $(if $(wildcard $(SOURCEDIR)/vendor/modules.txt),-mod=vendor,-mod=readonly)
GO_MODFILES := $(SOURCEDIR)/go.mod $(SOURCEDIR)/go.sum
GOFLAGS := $(GO_MODFLAGS) -trimpath
GOPROXY := https://proxy.golang.org

export GOFLAGS GO111MODULE GOPROXY
