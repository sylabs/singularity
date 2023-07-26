# This file contains all of the rules for building squashfuse_ll

squashfuse_ll := $(SOURCEDIR)/third_party/squashfuse/squashfuse_ll
squashfuse_dir := $(SOURCEDIR)/third_party/squashfuse
squashfuse_src := $(SOURCEDIR)/third_party/squashfuse/autogen.sh
squashfuse_INSTALL := $(DESTDIR)$(LIBEXECDIR)/singularity/bin/squashfuse_ll

$(squashfuse_ll): $(squashfuse_src)
	@echo " SQUASHFUSE"
	cd $(squashfuse_dir) && ./autogen.sh
	cd $(squashfuse_dir) && ./configure
	$(MAKE) -C $(squashfuse_dir) squashfuse_ll
	
$(squashfuse_INSTALL): $(squashfuse_ll)
	@echo " INSTALL SQUASHFUSE" $@
	$(V)umask 0022 && mkdir -p $(@D)
	$(V)install -m 0755 $< $@

.PHONY:
squashfuse_CLEAN:
	@echo " CLEAN SQUASHFUSE"
	$(MAKE) -C $(squashfuse_dir) clean

INSTALLFILES += $(squashfuse_INSTALL)
ALL += $(squashfuse_ll)
CLEANTARGETS += squashfuse_CLEAN