V := @

PLUGIN_NAME := sylabs.io/enterprise-plugin
PLUGIN_SHORTNAME := enterprise-plugin
PLUGIN_SIF := $(PLUGIN_SHORTNAME).sif
PLUGIN_VERSION := 1.0.0

SINGULARITY_4 := $(shell singularity version | grep "^4.")
ifneq ($(SINGULARITY_4), )
SINGULARITY_BUILDDIR := $(shell singularity buildcfg | grep -oP '(?<=BUILDDIR=).*')
SINGULARITY_LIBEXECDIR := $(shell singularity buildcfg | grep -oP '(?<=LIBEXECDIR=).*')
SINGULARITY_CONFDIR := $(shell singularity buildcfg | grep -oP '(?<=SINGULARITY_CONFDIR=).*')
SINGULARITY_SOURCEDIR := $(shell dirname $(SINGULARITY_BUILDDIR))
PLUGIN_BUILDDIR := $(SINGULARITY_SOURCEDIR)/$(PLUGIN_SHORTNAME)
PLUGIN_INSTALLDIR := $(SINGULARITY_LIBEXECDIR)/singularity/plugin/$(PLUGIN_NAME)
PLUGIN_CONFDIR := $(SINGULARITY_CONFDIR)/plugins/$(PLUGIN_NAME)
endif

all: $(PLUGIN_SIF)

.PHONY: has_singularity
has_singularity:
ifeq ($(SINGULARITY_4), )
	@echo 'This plugin only supports SingularityCE 4.x'
	@echo 'Please ensure SingularityCE 4.x is in your PATH'
	exit 1
else
	@echo 'Using Singularity source directory: $(SINGULARITY_SOURCEDIR)'
endif

.PHONY: clean
clean: clean
	@echo " CLEAN"
	$(V)go clean
	$(V)rm -f $(PLUGIN_SIF)

$(PLUGIN_SIF): has_singularity
	@echo "Building plugin at: $(PLUGIN_BUILDDIR)"
	@echo " COMPILE singularity plugin compile"
	$(V)cd $(SINGULARITY_SOURCEDIR) && singularity plugin compile -o $(PLUGIN_BUILDDIR)/$(PLUGIN_SIF) $(PLUGIN_BUILDDIR)

install: $(PLUGIN_SIF)
	@echo " INSTALL singularity plugin install"
	$(V)singularity plugin install $(PLUGIN_SIF)
	$(V)mkdir -p $(PLUGIN_CONFDIR)
	$(V)singularity plugin list

uninstall: has_singularity
	@echo " UNINSTALL singularity plugin uninstall"
	$(V)singularity plugin disable $(PLUGIN_NAME)
	$(V)rm -rf $(PLUGIN_CONFDIR)
	$(v)singularity plugin uninstall $(PLUGIN_NAME)

test: unit-test

unit-test:
	@echo " TEST go test (unit-tests)"
	$(V)go test -count 1 -race -cover -tags sylog  ./...

# Integration + unit tests
# Integration tests require the mock services
integration-test:
	@echo " TEST go test (integration-tests)"
	$(V)TZ=UTC go test -count 1 -race -cover -tags="enterprise_integration,sylog"  ./...

check:
	@echo " CHECK golangci-lint"
	$(V)golangci-lint run

# Call `go test -update` for goldie files in the CLI code.
test-update:
	@echo " TEST go test -update"
	$(V)TZ=UTC go test -count 1 -race -v -tags "enterprise_integration,sylog" ./... -update
