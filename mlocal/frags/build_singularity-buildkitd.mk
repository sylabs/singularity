# This file contains all of the rules for building the singularity-buildkitd binary

singularity-buildkitd_deps := $(BUILDDIR_ABSPATH)/singularity-buildkitd.d

-include $(singularity-buildkitd_deps)

$(singularity-buildkitd_deps): $(GO_MODFILES)
	@echo " GEN GO DEP" $@
	$(V)$(SOURCEDIR)/makeit/gengodep -v3 "$(GO)" "singularity-buildkitd_SOURCE" "$(GO_TAGS)" "$@" "$(SOURCEDIR)/cmd/singularity-buildkitd"

# Look at dependencies file changes via singularity_deps
# because it means that a module was updated.
singularity-buildkitd := $(BUILDDIR)/singularity-buildkitd
$(singularity-buildkitd): $(singularity_build_config) $(singularity-buildkitd_deps) $(singularity-buildkitd_SOURCE)
	@echo " GO" $@; echo "    [+] GO_TAGS" \"$(GO_TAGS)\"
	$(V)$(GO) build $(GO_MODFLAGS) $(GO_BUILDMODE) -tags "$(GO_TAGS)" $(GO_LDFLAGS) \
		-o $(BUILDDIR)/singularity-buildkitd $(SOURCEDIR)/cmd/singularity-buildkitd

singularity-buildkitd_INSTALL := $(DESTDIR)$(LIBEXECDIR)/singularity/bin/singularity-buildkitd
$(singularity-buildkitd_INSTALL): $(singularity-buildkitd)
	@echo " INSTALL" $@
	$(V)umask 0022 && mkdir -p $(@D)
	$(V)install -m 0755 $(singularity-buildkitd) $(singularity-buildkitd_INSTALL)

CLEANFILES += $(singularity-buildkitd)
INSTALLFILES += $(singularity-buildkitd_INSTALL)
ALL += $(singularity-buildkitd)
