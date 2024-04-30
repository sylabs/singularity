# This file contains all of the rules for building squashfuse_ll

squashfuse_ll := $(SOURCEDIR)/third_party/squashfuse/squashfuse_ll
squashfuse_dir := $(SOURCEDIR)/third_party/squashfuse
squashfuse_src := $(SOURCEDIR)/third_party/squashfuse/autogen.sh
squashfuse_INSTALL := $(DESTDIR)$(LIBEXECDIR)/singularity/bin/squashfuse_ll

# squashfuse currently fails to build with these warnings as errors,
# which are enforced by our own flags for CGO, or by distributions.
squashfuse_CFLAGS := $(filter-out -Wstrict-prototypes,$(CFLAGS))
squashfuse_CFLAGS := $(filter-out -Wunused-parameter,$(squashfuse_CFLAGS))
squashfuse_CFLAGS := $(filter-out -Wunused-variable,$(squashfuse_CFLAGS))
squashfuse_CFLAGS += -Wno-unused-variable

# Workaround for Ubuntu 24.04... we currently build with -D_FORTIFY_SOURCE=2
# so filter out the distro -D_FORTIFY_SOURCE=3 from CPPFLAGS to avoid
# conflict between the two settings.
squashfuse_CPPFLAGS := $(filter-out -D_FORTIFY_SOURCE=3,$(CPPFLAGS))

$(squashfuse_ll): $(squashfuse_src)
	@echo " SQUASHFUSE"
	echo $(squashfuse_CFLAGS)
	cd $(squashfuse_dir) && ./autogen.sh
	cd $(squashfuse_dir) && CFLAGS='$(squashfuse_CFLAGS)' CPPFLAGS='$(squashfuse_CPPFLAGS)' ./configure
	$(MAKE) CFLAGS='$(squashfuse_CFLAGS)' -C $(squashfuse_dir) squashfuse_ll

$(squashfuse_INSTALL): $(squashfuse_ll)
	@echo " INSTALL SQUASHFUSE" $@
	$(V)umask 0022 && mkdir -p $(@D)
	$(V)install -m 0755 $< $@

.PHONY:
squashfuse_CLEAN:
	@echo " CLEAN SQUASHFUSE"
	$(MAKE) -C $(squashfuse_dir) clean || true

INSTALLFILES += $(squashfuse_INSTALL)
ALL += $(squashfuse_ll)
CLEANTARGETS += squashfuse_CLEAN
