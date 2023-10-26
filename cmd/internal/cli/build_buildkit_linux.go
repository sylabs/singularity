// Copyright 2015 The Linux Foundation.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Copyright (c) 2023, Sylabs Inc. All rights reserved.
// This software is licensed under a 3-clause BSD license. Please consult the
// LICENSE.md file distributed with the sources of this project regarding your
// rights to use or distribute this software.
//
// This file contains modified code originally taken from:
// github.com/moby/buildkit/blob/v0.12.3/examples/build-using-dockerfile/main.go

package cli

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	snapshotsapi "github.com/containerd/containerd/api/services/snapshots/v1"
	"github.com/containerd/containerd/defaults"
	"github.com/containerd/containerd/pkg/dialer"
	"github.com/containerd/containerd/pkg/userns"
	"github.com/containerd/containerd/platforms"
	"github.com/containerd/containerd/reference"
	"github.com/containerd/containerd/remotes/docker"
	ctdsnapshot "github.com/containerd/containerd/snapshots"
	"github.com/containerd/containerd/snapshots/native"
	"github.com/containerd/containerd/snapshots/overlay"
	"github.com/containerd/containerd/snapshots/overlay/overlayutils"
	snproxy "github.com/containerd/containerd/snapshots/proxy"
	"github.com/containerd/containerd/sys"
	fuseoverlayfs "github.com/containerd/fuse-overlayfs-snapshotter"
	sgzfs "github.com/containerd/stargz-snapshotter/fs"
	sgzconf "github.com/containerd/stargz-snapshotter/fs/config"
	sgzlayer "github.com/containerd/stargz-snapshotter/fs/layer"
	sgzsource "github.com/containerd/stargz-snapshotter/fs/source"
	remotesn "github.com/containerd/stargz-snapshotter/snapshot"
	"github.com/coreos/go-systemd/v22/activation"
	sddaemon "github.com/coreos/go-systemd/v22/daemon"
	"github.com/docker/docker/pkg/idtools"
	"github.com/gofrs/flock"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/moby/buildkit/cache/remotecache"
	"github.com/moby/buildkit/cache/remotecache/azblob"
	"github.com/moby/buildkit/cache/remotecache/gha"
	inlineremotecache "github.com/moby/buildkit/cache/remotecache/inline"
	localremotecache "github.com/moby/buildkit/cache/remotecache/local"
	registryremotecache "github.com/moby/buildkit/cache/remotecache/registry"
	s3remotecache "github.com/moby/buildkit/cache/remotecache/s3"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/cmd/buildkitd/config"
	"github.com/moby/buildkit/control"
	"github.com/moby/buildkit/executor/oci"
	"github.com/moby/buildkit/frontend"
	dockerfile "github.com/moby/buildkit/frontend/dockerfile/builder"
	"github.com/moby/buildkit/frontend/gateway"
	"github.com/moby/buildkit/frontend/gateway/forwarder"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/solver"
	"github.com/moby/buildkit/solver/bboltcachestorage"
	"github.com/moby/buildkit/util/appdefaults"
	"github.com/moby/buildkit/util/archutil"
	"github.com/moby/buildkit/util/bklog"
	"github.com/moby/buildkit/util/grpcerrors"
	"github.com/moby/buildkit/util/network/cniprovider"
	"github.com/moby/buildkit/util/network/netproviders"
	"github.com/moby/buildkit/util/resolver"
	"github.com/moby/buildkit/util/stack"
	"github.com/moby/buildkit/util/tracing/detect"
	"github.com/moby/buildkit/util/tracing/transform"
	"github.com/moby/buildkit/version"
	"github.com/moby/buildkit/worker"
	"github.com/moby/buildkit/worker/base"
	"github.com/moby/buildkit/worker/runc"
	ocispecs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pelletier/go-toml"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.etcd.io/bbolt"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	tracev1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
)

type workerInitializerOpt struct {
	config         *config.Config
	sessionManager *session.Manager
	traceSocket    string
}

type workerInitializer struct {
	fn func(ctx context.Context, common workerInitializerOpt) ([]worker.Worker, error)
	// less priority number, more preferred
	priority int
}

var workerInitializers []workerInitializer

func registerWorkerInitializer(wi workerInitializer) {
	workerInitializers = append(workerInitializers, wi)
	sort.Slice(workerInitializers,
		func(i, j int) bool {
			return workerInitializers[i].priority < workerInitializers[j].priority
		})
}

func init() {
	registerWorkerInitializer(
		workerInitializer{
			fn:       ociWorkerInitializer,
			priority: 0,
		},
	)
	// TODO: allow multiple oci runtimes
}

func runBuildkitd(ctx context.Context, readyChan chan<- bool) error {
	cfg, err := config.LoadFile(defaultConfigPath())
	if err != nil {
		return err
	}

	setDefaultConfig(&cfg)
	logrus.SetFormatter(&logrus.TextFormatter{FullTimestamp: true})
	if cfg.Debug {
		logrus.SetLevel(logrus.DebugLevel)
	}
	if cfg.Trace {
		logrus.SetLevel(logrus.TraceLevel)
	}

	tp, err := detect.TracerProvider()
	if err != nil {
		return err
	}

	unary := grpc_middleware.ChainUnaryServer(unaryInterceptor(ctx, tp), grpcerrors.UnaryServerInterceptor)
	server := grpc.NewServer(grpc.UnaryInterceptor(unary))

	// relative path does not work with nightlyone/lockfile
	root, err := filepath.Abs(cfg.Root)
	if err != nil {
		return err
	}
	cfg.Root = root

	if err := os.MkdirAll(root, 0o700); err != nil {
		return errors.Wrapf(err, "failed to create %s", root)
	}

	lockPath := filepath.Join(root, "buildkitd.lock")
	lock := flock.New(lockPath)
	locked, err := lock.TryLock()
	if err != nil {
		return errors.Wrapf(err, "could not lock %s", lockPath)
	}
	if !locked {
		return errors.Errorf("could not lock %s, another instance running?", lockPath)
	}
	defer func() {
		lock.Unlock()
		os.RemoveAll(lockPath)
	}()

	controller, err := newController(ctx, &cfg)
	if err != nil {
		return err
	}
	defer controller.Close()

	controller.Register(server)
	reflection.Register(server)

	errCh := make(chan error, 1)
	if err := serveGRPC(cfg.GRPC, server, errCh); err != nil {
		return err
	}

	readyChan <- true

	select {
	case serverErr := <-errCh:
		err = serverErr
	case <-ctx.Done():
		err = ctx.Err()
	}

	bklog.G(ctx).Infof("stopping server")
	if os.Getenv("NOTIFY_SOCKET") != "" {
		notified, notifyErr := sddaemon.SdNotify(false, sddaemon.SdNotifyStopping)
		bklog.G(ctx).Debugf("SdNotifyStopping notified=%v, err=%v", notified, notifyErr)
	}
	server.GracefulStop()

	return err
}

func ociWorkerInitializer(ctx context.Context, common workerInitializerOpt) ([]worker.Worker, error) {
	cfg := common.config.Workers.OCI

	if (cfg.Enabled == nil && !validOCIBinary()) || (cfg.Enabled != nil && !*cfg.Enabled) {
		return nil, nil
	}

	// TODO: this should never change the existing state dir
	idmapping, err := parseIdentityMapping(cfg.UserRemapUnsupported)
	if err != nil {
		return nil, err
	}

	hosts := resolverFunc(common.config)
	snFactory, err := snapshotterFactory(ctx, common.config.Root, cfg, common.sessionManager, hosts)
	if err != nil {
		return nil, err
	}

	if cfg.Rootless {
		bklog.L.Debugf("running in rootless mode")
		if common.config.Workers.OCI.NetworkConfig.Mode == "auto" {
			common.config.Workers.OCI.NetworkConfig.Mode = "host"
		}
	}

	processMode := oci.ProcessSandbox
	if cfg.NoProcessSandbox {
		bklog.L.Warn("NoProcessSandbox is enabled. Note that NoProcessSandbox allows build containers to kill (and potentially ptrace) an arbitrary process in the BuildKit host namespace. NoProcessSandbox should be enabled only when the BuildKit is running in a container as an unprivileged user.")
		if !cfg.Rootless {
			return nil, errors.New("can't enable NoProcessSandbox without Rootless")
		}
		processMode = oci.NoProcessSandbox
	}

	dns := getDNSConfig(common.config.DNS)

	nc := netproviders.Opt{
		Mode: common.config.Workers.OCI.NetworkConfig.Mode,
		CNI: cniprovider.Opt{
			Root:       common.config.Root,
			ConfigPath: common.config.Workers.OCI.CNIConfigPath,
			BinaryDir:  common.config.Workers.OCI.CNIBinaryPath,
			PoolSize:   common.config.Workers.OCI.CNIPoolSize,
		},
	}

	var parallelismSem *semaphore.Weighted
	if cfg.MaxParallelism > 0 {
		parallelismSem = semaphore.NewWeighted(int64(cfg.MaxParallelism))
	}

	opt, err := runc.NewWorkerOpt(common.config.Root, snFactory, cfg.Rootless, processMode, cfg.Labels, idmapping, nc, dns, cfg.Binary, cfg.ApparmorProfile, cfg.SELinux, parallelismSem, common.traceSocket, cfg.DefaultCgroupParent)
	if err != nil {
		return nil, err
	}
	opt.GCPolicy = getGCPolicy(cfg.GCConfig, common.config.Root)
	opt.BuildkitVersion = getBuildkitVersion()
	opt.RegistryHosts = hosts

	if platformsStr := cfg.Platforms; len(platformsStr) != 0 {
		platforms, err := parsePlatforms(platformsStr)
		if err != nil {
			return nil, errors.Wrap(err, "invalid platforms")
		}
		opt.Platforms = platforms
	}
	w, err := base.NewWorker(ctx, opt)
	if err != nil {
		return nil, err
	}
	return []worker.Worker{w}, nil
}

func snapshotterFactory(_ context.Context, commonRoot string, cfg config.OCIConfig, sm *session.Manager, hosts docker.RegistryHosts) (runc.SnapshotterFactory, error) {
	var (
		name    = cfg.Snapshotter
		address = cfg.ProxySnapshotterPath
	)
	if address != "" {
		snFactory := runc.SnapshotterFactory{
			Name: name,
		}
		if _, err := os.Stat(address); os.IsNotExist(err) {
			return snFactory, errors.Wrapf(err, "snapshotter doesn't exist on %q (Do not include 'unix://' prefix)", address)
		}
		snFactory.New = func(root string) (ctdsnapshot.Snapshotter, error) {
			backoffConfig := backoff.DefaultConfig
			backoffConfig.MaxDelay = 3 * time.Second
			connParams := grpc.ConnectParams{
				Backoff: backoffConfig,
			}
			gopts := []grpc.DialOption{
				grpc.WithTransportCredentials(insecure.NewCredentials()),
				grpc.WithConnectParams(connParams),
				grpc.WithContextDialer(dialer.ContextDialer),
				grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(defaults.DefaultMaxRecvMsgSize)),
				grpc.WithDefaultCallOptions(grpc.MaxCallSendMsgSize(defaults.DefaultMaxSendMsgSize)),
			}
			conn, err := grpc.Dial(dialer.DialAddress(address), gopts...)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to dial %q", address)
			}
			return snproxy.NewSnapshotter(snapshotsapi.NewSnapshotsClient(conn), name), nil
		}
		return snFactory, nil
	}

	if name == "auto" {
		if err := overlayutils.Supported(commonRoot); err == nil {
			name = "overlayfs"
		} else {
			bklog.L.Debugf("auto snapshotter: overlayfs is not available for %s, trying fuse-overlayfs: %v", commonRoot, err)
			if err2 := fuseoverlayfs.Supported(commonRoot); err2 == nil {
				name = "fuse-overlayfs"
			} else {
				bklog.L.Debugf("auto snapshotter: fuse-overlayfs is not available for %s, falling back to native: %v", commonRoot, err2)
				name = "native"
			}
		}
		bklog.L.Infof("auto snapshotter: using %s", name)
	}

	snFactory := runc.SnapshotterFactory{
		Name: name,
	}
	switch name {
	case "native":
		snFactory.New = native.NewSnapshotter
	case "overlayfs": // not "overlay", for consistency with containerd snapshotter plugin ID.
		snFactory.New = func(root string) (ctdsnapshot.Snapshotter, error) {
			return overlay.NewSnapshotter(root, overlay.AsynchronousRemove)
		}
	case "fuse-overlayfs":
		snFactory.New = func(root string) (ctdsnapshot.Snapshotter, error) {
			// no Opt (AsynchronousRemove is untested for fuse-overlayfs)
			return fuseoverlayfs.NewSnapshotter(root)
		}
	case "stargz":
		sgzCfg := sgzconf.Config{}
		if cfg.StargzSnapshotterConfig != nil {
			// In order to keep the stargz Config type (and dependency) out of
			// the main BuildKit config, the main config Unmarshalls it into a
			// generic map[string]interface{}. Here we convert it back into TOML
			// tree, and unmarshal it to the actual type.
			t, err := toml.TreeFromMap(cfg.StargzSnapshotterConfig)
			if err != nil {
				return snFactory, errors.Wrapf(err, "failed to parse stargz config")
			}
			err = t.Unmarshal(&sgzCfg)
			if err != nil {
				return snFactory, errors.Wrapf(err, "failed to parse stargz config")
			}
		}
		snFactory.New = func(root string) (ctdsnapshot.Snapshotter, error) {
			userxattr, err := overlayutils.NeedsUserXAttr(root)
			if err != nil {
				bklog.L.WithError(err).Warnf("cannot detect whether \"userxattr\" option needs to be used, assuming to be %v", userxattr)
			}
			opq := sgzlayer.OverlayOpaqueTrusted
			if userxattr {
				opq = sgzlayer.OverlayOpaqueUser
			}
			fs, err := sgzfs.NewFilesystem(filepath.Join(root, "stargz"),
				sgzCfg,
				// Source info based on the buildkit's registry config and session
				sgzfs.WithGetSources(sourceWithSession(hosts, sm)),
				sgzfs.WithMetricsLogLevel(logrus.DebugLevel),
				sgzfs.WithOverlayOpaqueType(opq),
			)
			if err != nil {
				return nil, err
			}
			return remotesn.NewSnapshotter(context.Background(),
				filepath.Join(root, "snapshotter"),
				fs, remotesn.AsynchronousRemove, remotesn.NoRestore)
		}
	default:
		return snFactory, errors.Errorf("unknown snapshotter name: %q", name)
	}
	return snFactory, nil
}

func validOCIBinary() bool {
	_, err := exec.LookPath("runc")
	_, err1 := exec.LookPath("buildkit-runc")
	if err != nil && err1 != nil {
		bklog.L.Warnf("skipping oci worker, as runc does not exist")
		return false
	}
	return true
}

const (
	// targetRefLabel is a label which contains image reference.
	targetRefLabel = "containerd.io/snapshot/remote/stargz.reference"

	// targetDigestLabel is a label which contains layer digest.
	targetDigestLabel = "containerd.io/snapshot/remote/stargz.digest"

	// targetImageLayersLabel is a label which contains layer digests contained in
	// the target image.
	targetImageLayersLabel = "containerd.io/snapshot/remote/stargz.layers"

	// targetSessionLabel is a labeld which contains session IDs usable for
	// authenticating the target snapshot.
	targetSessionLabel = "containerd.io/snapshot/remote/stargz.session"
)

// sourceWithSession returns a callback which implements a converter from labels to the
// typed snapshot source info. This callback is called everytime the snapshotter resolves a
// snapshot. This callback returns configuration that is based on buildkitd's registry config
// and utilizes the session-based authorizer.
func sourceWithSession(hosts docker.RegistryHosts, sm *session.Manager) sgzsource.GetSources {
	return func(labels map[string]string) (src []sgzsource.Source, err error) {
		// labels contains multiple source candidates with unique IDs appended on each call
		// to the snapshotter API. So, first, get all these IDs
		var ids []string
		for k := range labels {
			if strings.HasPrefix(k, targetRefLabel+".") {
				ids = append(ids, strings.TrimPrefix(k, targetRefLabel+"."))
			}
		}

		// Parse all labels
		for _, id := range ids {
			// Parse session labels
			ref, ok := labels[targetRefLabel+"."+id]
			if !ok {
				continue
			}
			named, err := reference.Parse(ref)
			if err != nil {
				continue
			}
			var sids []string
			for i := 0; ; i++ {
				sidKey := targetSessionLabel + "." + fmt.Sprintf("%d", i) + "." + id
				sid, ok := labels[sidKey]
				if !ok {
					break
				}
				sids = append(sids, sid)
			}

			// Get source information based on labels and RegistryHosts containing
			// session-based authorizer.
			parse := sgzsource.FromDefaultLabels(func(ref reference.Spec) ([]docker.RegistryHost, error) {
				return resolver.DefaultPool.GetResolver(hosts, named.String(), "pull", sm, session.NewGroup(sids...)).
					HostsFunc(ref.Hostname())
			})
			if s, err := parse(map[string]string{
				targetRefLabel:         ref,
				targetDigestLabel:      labels[targetDigestLabel+"."+id],
				targetImageLayersLabel: labels[targetImageLayersLabel+"."+id],
			}); err == nil {
				src = append(src, s...)
			}
		}

		return src, nil
	}
}

func parseIdentityMapping(str string) (*idtools.IdentityMapping, error) {
	if str == "" {
		return nil, nil
	}

	idparts := strings.SplitN(str, ":", 3)
	if len(idparts) > 2 {
		return nil, errors.Errorf("invalid userns remap specification in %q", str)
	}

	username := idparts[0]

	bklog.L.Debugf("user namespaces: ID ranges will be mapped to subuid ranges of: %s", username)

	mappings, err := idtools.LoadIdentityMapping(username)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create ID mappings")
	}
	return &mappings, nil
}

func serveGRPC(cfg config.GRPCConfig, server *grpc.Server, errCh chan error) error {
	addrs := cfg.Address
	if len(addrs) == 0 {
		return errors.New("--addr cannot be empty")
	}
	tlsConfig, err := serverCredentials(cfg.TLS)
	if err != nil {
		return err
	}
	eg, _ := errgroup.WithContext(context.Background())
	listeners := make([]net.Listener, 0, len(addrs))
	for _, addr := range addrs {
		l, err := getListener(addr, *cfg.UID, *cfg.GID, tlsConfig)
		if err != nil {
			for _, l := range listeners {
				l.Close()
			}
			return err
		}
		listeners = append(listeners, l)
	}

	if os.Getenv("NOTIFY_SOCKET") != "" {
		notified, notifyErr := sddaemon.SdNotify(false, sddaemon.SdNotifyReady)
		bklog.L.Debugf("SdNotifyReady notified=%v, err=%v", notified, notifyErr)
	}
	for _, l := range listeners {
		func(l net.Listener) {
			eg.Go(func() error {
				defer l.Close()
				bklog.L.Infof("running server on %s", l.Addr())
				return server.Serve(l)
			})
		}(l)
	}
	go func() {
		errCh <- eg.Wait()
	}()
	return nil
}

func defaultConfigPath() string {
	if userns.RunningInUserNS() {
		return filepath.Join(appdefaults.UserConfigDir(), "buildkitd.toml")
	}
	return filepath.Join(appdefaults.ConfigDir, "buildkitd.toml")
}

func setDefaultNetworkConfig(nc config.NetworkConfig) config.NetworkConfig {
	if nc.Mode == "" {
		nc.Mode = "auto"
	}
	if nc.CNIConfigPath == "" {
		nc.CNIConfigPath = appdefaults.DefaultCNIConfigPath
	}
	if nc.CNIBinaryPath == "" {
		nc.CNIBinaryPath = appdefaults.DefaultCNIBinDir
	}
	return nc
}

func setDefaultConfig(cfg *config.Config) {
	orig := *cfg

	if cfg.GRPC.UID == nil {
		uid := os.Getuid()
		cfg.GRPC.UID = &uid
	}

	if cfg.GRPC.GID == nil {
		gid := os.Getgid()
		cfg.GRPC.GID = &gid
	}

	enabled := true
	cfg.Workers.OCI.Enabled = &enabled

	if cfg.Root == "" {
		cfg.Root = appdefaults.Root
	}

	cfg.Workers.OCI.Snapshotter = "overlayfs"

	if len(cfg.GRPC.Address) == 0 {
		cfg.GRPC.Address = []string{appdefaults.Address}
	}

	if cfg.Workers.OCI.Platforms == nil {
		cfg.Workers.OCI.Platforms = formatPlatforms(archutil.SupportedPlatforms(false))
	}
	if cfg.Workers.Containerd.Platforms == nil {
		cfg.Workers.Containerd.Platforms = formatPlatforms(archutil.SupportedPlatforms(false))
	}

	cfg.Workers.OCI.NetworkConfig = setDefaultNetworkConfig(cfg.Workers.OCI.NetworkConfig)
	cfg.Workers.Containerd.NetworkConfig = setDefaultNetworkConfig(cfg.Workers.Containerd.NetworkConfig)

	if userns.RunningInUserNS() {
		// if buildkitd is being executed as the mapped-root (not only EUID==0 but also $USER==root)
		// in a user namespace, we need to enable the rootless mode but
		// we don't want to honor $HOME for setting up default paths.
		if u := os.Getenv("USER"); u != "" && u != "root" {
			if orig.Root == "" {
				cfg.Root = appdefaults.UserRoot()
			}
			if len(orig.GRPC.Address) == 0 {
				cfg.GRPC.Address = []string{appdefaults.UserAddress()}
			}
			appdefaults.EnsureUserAddressDir()
		}
	}
}

func getListener(addr string, uid, gid int, tlsConfig *tls.Config) (net.Listener, error) {
	addrSlice := strings.SplitN(addr, "://", 2)
	if len(addrSlice) < 2 {
		return nil, errors.Errorf("address %s does not contain proto, you meant unix://%s ?",
			addr, addr)
	}
	proto := addrSlice[0]
	listenAddr := addrSlice[1]
	switch proto {
	case "unix", "npipe":
		if tlsConfig != nil {
			bklog.L.Warnf("TLS is disabled for %s", addr)
		}
		return sys.GetLocalListener(listenAddr, uid, gid)
	case "fd":
		return listenFD(listenAddr, tlsConfig)
	case "tcp":
		l, err := net.Listen("tcp", listenAddr)
		if err != nil {
			return nil, err
		}

		if tlsConfig == nil {
			bklog.L.Warnf("TLS is not enabled for %s. enabling mutual TLS authentication is highly recommended", addr)
			return l, nil
		}
		return tls.NewListener(l, tlsConfig), nil
	default:
		return nil, errors.Errorf("addr %s not supported", addr)
	}
}

func unaryInterceptor(globalCtx context.Context, tp trace.TracerProvider) grpc.UnaryServerInterceptor {
	withTrace := otelgrpc.UnaryServerInterceptor(otelgrpc.WithTracerProvider(tp))

	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		go func() {
			select {
			case <-ctx.Done():
			case <-globalCtx.Done():
				cancel()
			}
		}()

		if strings.HasSuffix(info.FullMethod, "opentelemetry.proto.collector.trace.v1.TraceService/Export") {
			return handler(ctx, req)
		}

		resp, err = withTrace(ctx, req, info, handler)
		if err != nil {
			bklog.G(ctx).Errorf("%s returned error: %v", info.FullMethod, err)
			if logrus.GetLevel() >= logrus.DebugLevel {
				fmt.Fprintf(os.Stderr, "%+v", stack.Formatter(grpcerrors.FromGRPC(err)))
			}
		}
		return
	}
}

func serverCredentials(cfg config.TLSConfig) (*tls.Config, error) {
	certFile := cfg.Cert
	keyFile := cfg.Key
	caFile := cfg.CA
	if certFile == "" && keyFile == "" {
		return nil, nil
	}
	err := errors.New("you must specify key and cert file if one is specified")
	if certFile == "" {
		return nil, err
	}
	if keyFile == "" {
		return nil, err
	}
	certificate, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, errors.Wrap(err, "could not load server key pair")
	}
	tlsConf := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{certificate},
	}
	if caFile != "" {
		certPool := x509.NewCertPool()
		ca, err := os.ReadFile(caFile)
		if err != nil {
			return nil, errors.Wrap(err, "could not read ca certificate")
		}
		// Append the client certificates from the CA
		if ok := certPool.AppendCertsFromPEM(ca); !ok {
			return nil, errors.New("failed to append ca cert")
		}
		tlsConf.ClientAuth = tls.RequireAndVerifyClientCert
		tlsConf.ClientCAs = certPool
	}
	return tlsConf, nil
}

func newController(ctx context.Context, cfg *config.Config) (*control.Controller, error) {
	sessionManager, err := session.NewManager()
	if err != nil {
		return nil, err
	}

	tc, err := detect.Exporter()
	if err != nil {
		return nil, err
	}

	var traceSocket string
	if tc != nil {
		traceSocket = traceSocketPath(cfg.Root)
		if err := runTraceController(traceSocket, tc); err != nil {
			logrus.Warnf("failed set up otel-grpc controller: %v", err)
			traceSocket = ""
		}
	}

	wc, err := newWorkerController(ctx, workerInitializerOpt{
		config:         cfg,
		sessionManager: sessionManager,
		traceSocket:    traceSocket,
	})
	if err != nil {
		return nil, err
	}
	frontends := map[string]frontend.Frontend{}
	frontends["dockerfile.v0"] = forwarder.NewGatewayForwarder(wc, dockerfile.Build)
	frontends["gateway.v0"] = gateway.NewGatewayFrontend(wc)

	cacheStorage, err := bboltcachestorage.NewStore(filepath.Join(cfg.Root, "cache.db"))
	if err != nil {
		return nil, err
	}

	historyDB, err := bbolt.Open(filepath.Join(cfg.Root, "history.db"), 0o600, nil)
	if err != nil {
		return nil, err
	}

	resolverFn := resolverFunc(cfg)

	w, err := wc.GetDefault()
	if err != nil {
		return nil, err
	}

	remoteCacheExporterFuncs := map[string]remotecache.ResolveCacheExporterFunc{
		"registry": registryremotecache.ResolveCacheExporterFunc(sessionManager, resolverFn),
		"local":    localremotecache.ResolveCacheExporterFunc(sessionManager),
		"inline":   inlineremotecache.ResolveCacheExporterFunc(),
		"gha":      gha.ResolveCacheExporterFunc(),
		"s3":       s3remotecache.ResolveCacheExporterFunc(),
		"azblob":   azblob.ResolveCacheExporterFunc(),
	}
	remoteCacheImporterFuncs := map[string]remotecache.ResolveCacheImporterFunc{
		"registry": registryremotecache.ResolveCacheImporterFunc(sessionManager, w.ContentStore(), resolverFn),
		"local":    localremotecache.ResolveCacheImporterFunc(sessionManager),
		"gha":      gha.ResolveCacheImporterFunc(),
		"s3":       s3remotecache.ResolveCacheImporterFunc(),
		"azblob":   azblob.ResolveCacheImporterFunc(),
	}
	return control.NewController(control.Opt{
		SessionManager:            sessionManager,
		WorkerController:          wc,
		Frontends:                 frontends,
		ResolveCacheExporterFuncs: remoteCacheExporterFuncs,
		ResolveCacheImporterFuncs: remoteCacheImporterFuncs,
		CacheManager:              solver.NewCacheManager(ctx, "local", cacheStorage, worker.NewCacheResultStorage(wc)),
		Entitlements:              cfg.Entitlements,
		TraceCollector:            tc,
		HistoryDB:                 historyDB,
		CacheStore:                cacheStorage,
		LeaseManager:              w.LeaseManager(),
		ContentStore:              w.ContentStore(),
		HistoryConfig:             cfg.History,
	})
}

func resolverFunc(cfg *config.Config) docker.RegistryHosts {
	return resolver.NewRegistryConfig(cfg.Registries)
}

func newWorkerController(ctx context.Context, wiOpt workerInitializerOpt) (*worker.Controller, error) {
	wc := &worker.Controller{}
	nWorkers := 0
	for _, wi := range workerInitializers {
		ws, err := wi.fn(ctx, wiOpt)
		if err != nil {
			return nil, err
		}
		for _, w := range ws {
			p := w.Platforms(false)
			bklog.L.Infof("found worker %q, labels=%v, platforms=%v", w.ID(), w.Labels(), formatPlatforms(p))
			archutil.WarnIfUnsupported(p)
			if err = wc.Add(w); err != nil {
				return nil, err
			}
			nWorkers++
		}
	}
	if nWorkers == 0 {
		return nil, errors.New("no worker found, rebuild the buildkit daemon?")
	}
	defaultWorker, err := wc.GetDefault()
	if err != nil {
		return nil, err
	}
	bklog.L.Infof("found %d workers, default=%q", nWorkers, defaultWorker.ID())
	bklog.L.Warn("currently, only the default worker can be used.")
	return wc, nil
}

func formatPlatforms(p []ocispecs.Platform) []string {
	str := make([]string, 0, len(p))
	for _, pp := range p {
		str = append(str, platforms.Format(platforms.Normalize(pp)))
	}
	return str
}

func parsePlatforms(platformsStr []string) ([]ocispecs.Platform, error) {
	out := make([]ocispecs.Platform, 0, len(platformsStr))
	for _, s := range platformsStr {
		p, err := platforms.Parse(s)
		if err != nil {
			return nil, err
		}
		out = append(out, platforms.Normalize(p))
	}
	return out, nil
}

func getGCPolicy(cfg config.GCConfig, root string) []client.PruneInfo {
	if cfg.GC != nil && !*cfg.GC {
		return nil
	}
	if len(cfg.GCPolicy) == 0 {
		cfg.GCPolicy = config.DefaultGCPolicy(cfg.GCKeepStorage)
	}
	out := make([]client.PruneInfo, 0, len(cfg.GCPolicy))
	for _, rule := range cfg.GCPolicy {
		out = append(out, client.PruneInfo{
			Filter:       rule.Filters,
			All:          rule.All,
			KeepBytes:    rule.KeepBytes.AsBytes(root),
			KeepDuration: rule.KeepDuration.Duration,
		})
	}
	return out
}

func getBuildkitVersion() client.BuildkitVersion {
	return client.BuildkitVersion{
		Package:  version.Package,
		Version:  version.Version,
		Revision: version.Revision,
	}
}

func getDNSConfig(cfg *config.DNSConfig) *oci.DNSConfig {
	var dns *oci.DNSConfig
	if cfg != nil {
		dns = &oci.DNSConfig{
			Nameservers:   cfg.Nameservers,
			Options:       cfg.Options,
			SearchDomains: cfg.SearchDomains,
		}
	}
	return dns
}

func runTraceController(p string, exp sdktrace.SpanExporter) error {
	server := grpc.NewServer()
	tracev1.RegisterTraceServiceServer(server, &traceCollector{exporter: exp})
	l, err := getLocalListener(p)
	if err != nil {
		return errors.Wrap(err, "creating trace controller listener")
	}
	go server.Serve(l)
	return nil
}

type traceCollector struct {
	*tracev1.UnimplementedTraceServiceServer
	exporter sdktrace.SpanExporter
}

func (t *traceCollector) Export(ctx context.Context, req *tracev1.ExportTraceServiceRequest) (*tracev1.ExportTraceServiceResponse, error) {
	err := t.exporter.ExportSpans(ctx, transform.Spans(req.GetResourceSpans()))
	if err != nil {
		return nil, err
	}
	return &tracev1.ExportTraceServiceResponse{}, nil
}

func listenFD(addr string, tlsConfig *tls.Config) (net.Listener, error) {
	var (
		err       error
		listeners []net.Listener
	)
	// socket activation
	if tlsConfig != nil {
		listeners, err = activation.TLSListeners(tlsConfig)
	} else {
		listeners, err = activation.Listeners()
	}
	if err != nil {
		return nil, err
	}

	if len(listeners) == 0 {
		return nil, errors.New("no sockets found via socket activation: make sure the service was started by systemd")
	}

	// default to first fd
	if addr == "" {
		return listeners[0], nil
	}

	// TODO: systemd fd selection (default is 3)
	return nil, errors.New("not supported yet")
}

func traceSocketPath(root string) string {
	return filepath.Join(root, "otel-grpc.sock")
}

func getLocalListener(listenerPath string) (net.Listener, error) {
	uid := os.Getuid()
	l, err := sys.GetLocalListener(listenerPath, uid, uid)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(listenerPath, 0o666); err != nil {
		l.Close()
		return nil, err
	}
	return l, nil
}
