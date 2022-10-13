# Deprecation Warning

This plugin, implementing support for unprivileged overlays on patched Ubuntu
kernels, will not be supported from SingularityCE 4.0.

Image driver plugins, implementing the RegisterImageDriver callback, are
deprecated and will be removed in 4.0. Support for this example plugin,
permitting Ubuntu unprivileged overlay functionality, will be replaced with a
non-plugin implementation.
