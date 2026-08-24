V := @

PLUGIN_NAME := sylabs.io/log-plugin
PLUGIN_SHORTNAME := log-plugin
PLUGIN_SIF := $(PLUGIN_SHORTNAME).sif

SINGULARITY_VER := $(shell singularity version | grep "^4.")
ifneq ($(SINGULARITY_VER), )
SINGULARITY_BUILDDIR := $(shell singularity buildcfg | grep -oP '(?<=BUILDDIR=).*')
SINGULARITY_LIBEXECDIR := $(shell singularity buildcfg | grep -oP '(?<=LIBEXECDIR=).*')
SINGULARITY_CONFDIR := $(shell singularity buildcfg | grep -oP '(?<=SINGULARITY_CONFDIR=).*')
SINGULARITY_SOURCEDIR := $(shell dirname $(SINGULARITY_BUILDDIR))
PLUGIN_INSTALLDIR := $(SINGULARITY_LIBEXECDIR)/singularity/plugin/$(PLUGIN_NAME)
PLUGIN_CONFDIR := $(SINGULARITY_CONFDIR)/plugins/$(PLUGIN_NAME)
endif


all: $(PLUGIN_SIF)

.PHONY: has_singularity
has_singularity:
ifeq ($(SINGULARITY_VER), )
	@echo 'This plugin only supports SingularityCE 4.x'
	@echo 'Please ensure SingularityCE 4.x is in your PATH'
	exit 1
else
	@echo 'Using Singularity source directory: $(SINGULARITY_SOURCEDIR)'
endif

$(PLUGIN_SIF): has_singularity *.go
	@echo " COMPILE singularity plugin compile"
	$(V)cd $(SINGULARITY_SOURCEDIR) && singularity plugin compile -o log-plugin/$(PLUGIN_SIF) log-plugin/
	@echo " COPY $(PLUGIN_SIF)"

.PHONY: clean
clean:
	@echo " CLEAN"
	$(V)rm -f $(PLUGIN_SIF)

install: $(PLUGIN_SIF)
	@echo " INSTALL singularity plugin install"
	$(V)singularity plugin install $(PLUGIN_SIF)
	$(V)mkdir -p $(PLUGIN_CONFDIR)
	$(V)cp config.yml default.tpl $(PLUGIN_CONFDIR)
	$(V)singularity plugin list

uninstall: has_singularity
	@echo " UNINSTALL singularity plugin uninstall"
	$(V)singularity plugin disable $(PLUGIN_NAME)
	$(V)rm -f $(PLUGIN_INSTALLDIR)/config.yml $(PLUGIN_INSTALLDIR)/default.tpl
	$(v)singularity plugin uninstall $(PLUGIN_NAME)

test: has_singularity
	@echo " TEST go test"
	$(V)go test -count 1 -race -cover

check: has_singularity
	@echo " CHECK golangci-lint"
	$(V)golangci-lint run
