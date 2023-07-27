# This file contains all of the rules for building conmon

conmon := $(SOURCEDIR)/third_party/conmon/bin/conmon
conmon_dir := $(SOURCEDIR)/third_party/conmon/
conmon_src := $(SOURCEDIR)/third_party/conmon/Makefile
conmon_INSTALL := $(DESTDIR)$(LIBEXECDIR)/singularity/bin/conmon

# conmon currently fails to build with these warnings as errors,
# which are enforced by our own flags for CGO, or by distributions.
conmon_CFLAGS := $(filter-out -Wstrict-prototypes,$(CFLAGS))
conmon_CFLAGS := $(filter-out -Wframe-larger-than=2047,$(conmon_CFLAGS))
conmon_CFLAGS := $(filter-out -Wpointer-arith,$(conmon_CFLAGS))
conmon_CFLAGS += -std=c99

$(conmon): $(conmon_src)
	@echo " CONMON"
	$(MAKE) CFLAGS='$(conmon_CFLAGS)' -C $(conmon_dir)
	
$(conmon_INSTALL): $(conmon)
	@echo " INSTALL CONMON" $@
	$(V)umask 0022 && mkdir -p $(@D)
	$(V)install -m 0755 $< $@

.PHONY:
conmon_CLEAN:
	@echo " CLEAN CONMON"
	$(MAKE) -C $(conmon_dir) clean

INSTALLFILES += $(conmon_INSTALL)
ALL += $(conmon)
CLEANTARGETS += conmon_CLEAN