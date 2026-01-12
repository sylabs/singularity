#!/bin/bash
set -eux -o pipefail

arch=$(uname -i)
if [[ $arch != aarch64 ]];
then
  exit 0
fi

echo "aarch64 (arm64) detected - configuring gold linker workaround for go"

echo "Renaming gcc to gcc.orig"
mv /usr/bin/gcc /usr/bin/gcc.orig

echo "Adding gcc wrapper script"

cat << 'EOF' > /usr/bin/gcc
#!/bin/bash
#
# When using CGO (which is required when using go-microsoft for FIPS builds), Go has an
# issue on arm64 where it passes -fuse-ld=gold to GCC which is a workaround for an issue
# that doesn't exist with AL2023 ld, and the workaround actually breaks builds.
#
# So this hack wraps gcc and removes any -fuse-ld=gold if passed, and generates output to
# trick the Go compiler into thinking the workaround was applied.  This is only put in
# place on arm64.
#
# Many builds of Go remove the workaround by applying
# https://go-review.googlesource.com/c/go/+/391115 but rather than compile Microsoft's Go
# distribution from source, we use this gcc-wrapping kludge instead.
#
# This wrapper can be removed when https://github.com/golang/go/issues/22040 is closed.
args=()
for arg in "$@"; do
    if [[ "$arg" == "-fuse-ld=gold" ]]; then
        # The presence of the substring "GNU gold" in stdout is required to trick the Go
        # compiler into thinking it was used, otherwise it will abort.
        echo "Go build hack: dropping -fuse-ld=gold arg to disable use of GNU gold (see https://github.com/golang/go/issues/22040)"
    else
        args+=("$arg")
    fi
done

exec gcc.orig "${args[@]}"
EOF

chmod +x /usr/bin/gcc

echo "gold linker workaround installed"
