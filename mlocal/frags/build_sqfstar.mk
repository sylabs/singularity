# This file contains all of the rules for building sqfstar

sqfstar := $(SOURCEDIR)/third_party/squashfs-tools/squashfs-tools/sqfstar
sqfstar_dir := $(SOURCEDIR)/third_party/squashfs-tools/squashfs-tools
sqfstar_INSTALL := $(DESTDIR)$(LIBEXECDIR)/singularity/bin/sqfstar

$(sqfstar):
	@echo "SQFSTAR"
	$(MAKE) -C $(sqfstar_dir) mksquashfs
	
$(sqfstar_INSTALL): $(sqfstar)
	@echo " INSTALL SQFSTAR" $@
	$(V)umask 0022 && mkdir -p $(@D)
	$(V)install -m 0755 $< $@

.PHONY:
sqfstar_CLEAN:
	@echo " CLEAN SQFSTAR"
	$(MAKE) -C $(sqfstar_dir) clean

INSTALLFILES += $(sqfstar_INSTALL)
ALL += $(sqfstar)
CLEANTARGETS += sqfstar_CLEAN
