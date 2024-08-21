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
// github.com/moby/buildkit/tree/v0.12.3/executor
// github.com/moby/buildkit/tree/v0.12.3/worker/runc

package daemon

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/containerd/containerd/content/local"
	"github.com/containerd/containerd/diff/apply"
	"github.com/containerd/containerd/diff/walking"
	ctdmetadata "github.com/containerd/containerd/metadata"
	"github.com/containerd/containerd/mount"
	"github.com/containerd/containerd/oci"
	ctdsnapshot "github.com/containerd/containerd/snapshots"
	"github.com/containerd/continuity/fs"
	runc "github.com/containerd/go-runc"
	"github.com/containerd/platforms"
	"github.com/docker/docker/pkg/idtools"
	"github.com/moby/buildkit/cache"
	"github.com/moby/buildkit/cache/metadata"
	"github.com/moby/buildkit/executor"
	bkoci "github.com/moby/buildkit/executor/oci"
	"github.com/moby/buildkit/executor/resources"
	resourcestypes "github.com/moby/buildkit/executor/resources/types"
	gatewayapi "github.com/moby/buildkit/frontend/gateway/pb"
	"github.com/moby/buildkit/identity"
	containerdsnapshot "github.com/moby/buildkit/snapshot/containerd"
	"github.com/moby/buildkit/solver/pb"
	"github.com/moby/buildkit/util/leaseutil"
	bknet "github.com/moby/buildkit/util/network"
	"github.com/moby/buildkit/util/network/netproviders"
	rootlessspecconv "github.com/moby/buildkit/util/rootless/specconv"
	"github.com/moby/buildkit/util/stack"
	"github.com/moby/buildkit/util/winlayers"
	"github.com/moby/buildkit/worker/base"
	wlabel "github.com/moby/buildkit/worker/label"
	ocispecs "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/pkg/errors"
	"github.com/sylabs/singularity/v4/pkg/sylog"
	bolt "go.etcd.io/bbolt"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
)

// BkSnapshotterFactory instantiates a snapshotter
type BkSnapshotterFactory struct {
	Name string
	New  func(root string) (ctdsnapshot.Snapshotter, error)
}

// NewWorkerOpt creates a WorkerOpt.
func NewWorkerOpt(ctx context.Context, root string, snFactory BkSnapshotterFactory, rootless bool, processMode bkoci.ProcessMode, labels map[string]string, idmap *idtools.IdentityMapping, nopt netproviders.Opt, dns *bkoci.DNSConfig, binary, apparmorProfile string, selinux bool, parallelismSem *semaphore.Weighted, traceSocket, defaultCgroupParent string) (base.WorkerOpt, error) {
	var opt base.WorkerOpt
	name := "runc-" + snFactory.Name
	root = filepath.Join(root, name)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return opt, err
	}

	np, npResolvedMode, err := netproviders.Providers(nopt)
	if err != nil {
		return opt, err
	}

	// Check if user has specified OCI worker binary; if they have, append it to cmds
	var cmds []string
	if binary != "" {
		cmds = append(cmds, binary)
	}

	rm, err := resources.NewMonitor()
	if err != nil {
		return opt, err
	}

	exe, err := NewBuildExecutor(WorkerOpt{
		// Root directory
		Root: filepath.Join(root, "executor"),
		// If user has specified OCI worker binary, it will be sent to the runc executor to find and use
		// Otherwise, a nil array will be sent and the default OCI worker binary will be used
		CommandCandidates: cmds,
		// without root privileges
		Rootless:            rootless,
		ProcessMode:         processMode,
		IdentityMapping:     idmap,
		DNS:                 dns,
		ApparmorProfile:     apparmorProfile,
		SELinux:             selinux,
		TracingSocket:       traceSocket,
		DefaultCgroupParent: defaultCgroupParent,
		ResourceMonitor:     rm,
	}, np)
	if err != nil {
		return opt, err
	}
	s, err := snFactory.New(filepath.Join(root, "snapshots"))
	if err != nil {
		return opt, err
	}

	localstore, err := local.NewStore(filepath.Join(root, "content"))
	if err != nil {
		return opt, err
	}

	sylog.Debugf("About to open bolt db at: %s", filepath.Join(root, "containerdmeta.db"))
	db, err := bolt.Open(filepath.Join(root, "containerdmeta.db"), 0o644, nil)
	if err != nil {
		return opt, err
	}
	sylog.Debugf("Opened bolt db at: %s", filepath.Join(root, "containerdmeta.db"))

	mdb := ctdmetadata.NewDB(db, localstore, map[string]ctdsnapshot.Snapshotter{
		snFactory.Name: s,
	})
	if err := mdb.Init(ctx); err != nil {
		return opt, err
	}

	c := containerdsnapshot.NewContentStore(mdb.ContentStore(), "buildkit")

	id, err := base.ID(root)
	if err != nil {
		return opt, err
	}
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	xlabels := map[string]string{
		wlabel.Executor:       "oci",
		wlabel.Snapshotter:    snFactory.Name,
		wlabel.Hostname:       hostname,
		wlabel.Network:        npResolvedMode,
		wlabel.OCIProcessMode: processMode.String(),
		wlabel.SELinuxEnabled: strconv.FormatBool(selinux),
	}
	if apparmorProfile != "" {
		xlabels[wlabel.ApparmorProfile] = apparmorProfile
	}

	for k, v := range labels {
		xlabels[k] = v
	}
	lm := leaseutil.WithNamespace(ctdmetadata.NewLeaseManager(mdb), "buildkit")
	snap := containerdsnapshot.NewSnapshotter(snFactory.Name, mdb.Snapshotter(snFactory.Name), "buildkit", idmap)
	if err := cache.MigrateV2(
		ctx,
		filepath.Join(root, "metadata.db"),
		filepath.Join(root, "metadata_v2.db"),
		c,
		snap,
		lm,
	); err != nil {
		return opt, err
	}

	md, err := metadata.NewStore(filepath.Join(root, "metadata_v2.db"))
	if err != nil {
		return opt, err
	}

	opt = base.WorkerOpt{
		ID:               id,
		Labels:           xlabels,
		MetadataStore:    md,
		NetworkProviders: np,
		Executor:         exe,
		Snapshotter:      snap,
		ContentStore:     c,
		Applier:          winlayers.NewFileSystemApplierWithWindows(c, apply.NewFileSystemApplier(c)),
		Differ:           winlayers.NewWalkingDiffWithWindows(c, walking.NewWalkingDiff(c)),
		ImageStore:       nil, // explicitly
		Platforms:        []ocispecs.Platform{platforms.Normalize(platforms.DefaultSpec())},
		IdentityMapping:  idmap,
		LeaseManager:     lm,
		GarbageCollect:   mdb.GarbageCollect,
		ParallelismSem:   parallelismSem,
		MountPoolRoot:    filepath.Join(root, "cachemounts"),
		ResourceMonitor:  rm,
	}
	return opt, nil
}

type WorkerOpt struct {
	// root directory
	Root              string
	CommandCandidates []string
	// without root privileges (has nothing to do with Opt.Root directory)
	Rootless bool
	// DefaultCgroupParent is the cgroup-parent name for executor
	DefaultCgroupParent string
	// ProcessMode
	ProcessMode     bkoci.ProcessMode
	IdentityMapping *idtools.IdentityMapping
	// runc run --no-pivot (unrecommended)
	NoPivot         bool
	DNS             *bkoci.DNSConfig
	OOMScoreAdj     *int
	ApparmorProfile string
	SELinux         bool
	TracingSocket   string
	ResourceMonitor *resources.Monitor
}

var defaultCommandCandidates = []string{"buildkit-runc", "runc"}

type buildExecutor struct {
	runc             *runc.Runc
	root             string
	cgroupParent     string
	rootless         bool
	networkProviders map[pb.NetMode]bknet.Provider
	processMode      bkoci.ProcessMode
	idmap            *idtools.IdentityMapping
	noPivot          bool
	dns              *bkoci.DNSConfig
	oomScoreAdj      *int
	running          map[string]chan error
	mu               sync.Mutex
	apparmorProfile  string
	selinux          bool
	tracingSocket    string
	resmon           *resources.Monitor
}

func NewBuildExecutor(opt WorkerOpt, networkProviders map[pb.NetMode]bknet.Provider) (executor.Executor, error) {
	cmds := opt.CommandCandidates
	if cmds == nil {
		cmds = defaultCommandCandidates
	}

	var cmd string
	var found bool
	for _, cmd = range cmds {
		if _, err := exec.LookPath(cmd); err == nil {
			found = true
			break
		}
	}
	if !found {
		return nil, errors.Errorf("failed to find %s binary", cmd)
	}

	root := opt.Root

	if err := os.MkdirAll(root, 0o711); err != nil {
		return nil, errors.Wrapf(err, "failed to create %s", root)
	}

	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}

	// clean up old hosts/resolv.conf file. ignore errors
	os.RemoveAll(filepath.Join(root, "hosts"))
	os.RemoveAll(filepath.Join(root, "resolv.conf"))

	runtime := &runc.Runc{
		Command:   cmd,
		Log:       filepath.Join(root, "runc-log.json"),
		LogFormat: runc.JSON,
		Setpgid:   true,
		// we don't execute runc with --rootless=(true|false) explicitly,
		// so as to support non-runc runtimes
	}

	updateRuncFieldsForHostOS(runtime)

	w := &buildExecutor{
		runc:             runtime,
		root:             root,
		cgroupParent:     opt.DefaultCgroupParent,
		rootless:         opt.Rootless,
		networkProviders: networkProviders,
		processMode:      opt.ProcessMode,
		idmap:            opt.IdentityMapping,
		noPivot:          opt.NoPivot,
		dns:              opt.DNS,
		oomScoreAdj:      opt.OOMScoreAdj,
		running:          make(map[string]chan error),
		apparmorProfile:  opt.ApparmorProfile,
		selinux:          opt.SELinux,
		tracingSocket:    opt.TracingSocket,
		resmon:           opt.ResourceMonitor,
	}
	return w, nil
}

//nolint:maintidx
func (w *buildExecutor) Run(ctx context.Context, id string, root executor.Mount, mounts []executor.Mount, process executor.ProcessInfo, started chan<- struct{}) (rec resourcestypes.Recorder, err error) {
	meta := process.Meta

	startedOnce := sync.Once{}
	done := make(chan error, 1)
	w.mu.Lock()
	w.running[id] = done
	w.mu.Unlock()
	defer func() {
		w.mu.Lock()
		delete(w.running, id)
		w.mu.Unlock()
		done <- err
		close(done)
		if started != nil {
			startedOnce.Do(func() {
				close(started)
			})
		}
	}()

	provider, ok := w.networkProviders[meta.NetMode]
	if !ok {
		return nil, errors.Errorf("unknown network mode %s", meta.NetMode)
	}
	namespace, err := provider.New(ctx, meta.Hostname)
	if err != nil {
		return nil, err
	}
	doReleaseNetwork := true
	defer func() {
		if doReleaseNetwork {
			namespace.Close()
		}
	}()

	if meta.NetMode == pb.NetMode_HOST {
		sylog.Infof("enabling HostNetworking")
	}

	resolvConf, err := bkoci.GetResolvConf(ctx, w.root, w.idmap, w.dns, meta.NetMode)
	if err != nil {
		return nil, err
	}

	hostsFile, clean, err := bkoci.GetHostsFile(ctx, w.root, meta.ExtraHosts, w.idmap, meta.Hostname)
	if err != nil {
		return nil, err
	}
	if clean != nil {
		defer clean()
	}

	mountable, err := root.Src.Mount(ctx, false)
	if err != nil {
		return nil, err
	}

	rootMount, release, err := mountable.Mount()
	if err != nil {
		return nil, err
	}
	if release != nil {
		defer release()
	}

	if id == "" {
		id = identity.NewID()
	}
	bundle := filepath.Join(w.root, id)

	if err := os.Mkdir(bundle, 0o711); err != nil {
		return nil, err
	}
	defer os.RemoveAll(bundle)

	identity := idtools.Identity{}
	if w.idmap != nil {
		identity = w.idmap.RootPair()
	}

	rootFSPath := filepath.Join(bundle, "rootfs")
	if err := idtools.MkdirAllAndChown(rootFSPath, 0o700, identity); err != nil {
		return nil, err
	}
	if err := mount.All(rootMount, rootFSPath); err != nil {
		return nil, err
	}
	defer mount.Unmount(rootFSPath, 0)

	defer executor.MountStubsCleaner(ctx, rootFSPath, mounts, meta.RemoveMountStubsRecursive)()

	uid, gid, sgids, err := bkoci.GetUser(rootFSPath, meta.User)
	if err != nil {
		return nil, err
	}

	f, err := os.Create(filepath.Join(bundle, "config.json"))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	opts := []oci.SpecOpts{bkoci.WithUIDGID(uid, gid, sgids)}

	if meta.ReadonlyRootFS {
		opts = append(opts, oci.WithRootFSReadonly())
	}

	identity = idtools.Identity{
		UID: int(uid),
		GID: int(gid),
	}
	if w.idmap != nil {
		identity, err = w.idmap.ToHost(identity)
		if err != nil {
			return nil, err
		}
	}

	spec, cleanup, err := bkoci.GenerateSpec(ctx, meta, mounts, id, resolvConf, hostsFile, namespace, w.cgroupParent, w.processMode, w.idmap, w.apparmorProfile, w.selinux, w.tracingSocket, opts...)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	spec.Root.Path = rootFSPath
	if root.Readonly {
		spec.Root.Readonly = true
	}

	newp, err := fs.RootPath(rootFSPath, meta.Cwd)
	if err != nil {
		return nil, errors.Wrapf(err, "working dir %s points to invalid target", newp)
	}
	if _, err := os.Stat(newp); err != nil {
		if err := idtools.MkdirAllAndChown(newp, 0o755, identity); err != nil {
			return nil, errors.Wrapf(err, "failed to create working directory %s", newp)
		}
	}

	spec.Process.Terminal = meta.Tty
	spec.Process.OOMScoreAdj = w.oomScoreAdj
	if w.rootless {
		if err := rootlessspecconv.ToRootless(spec); err != nil {
			return nil, err
		}
	}

	if err := json.NewEncoder(f).Encode(spec); err != nil {
		return nil, err
	}

	sylog.Debugf("> creating %s %v", id, meta.Args)

	cgroupPath := spec.Linux.CgroupsPath
	if cgroupPath != "" {
		rec, err = w.resmon.RecordNamespace(cgroupPath, resources.RecordOpt{
			NetworkSampler: namespace,
		})
		if err != nil {
			return nil, err
		}
	}

	err = w.run(ctx, id, bundle, process, nil)

	releaseContainer := func(_ context.Context) error {
		if w.processMode == bkoci.NoProcessSandbox {
			return nil
		}

		return namespace.Close()
	}
	doReleaseNetwork = false

	err = exitError(ctx, err)
	if err != nil {
		if rec != nil {
			rec.Close()
		}
		releaseContainer(ctx)
		return nil, err
	}

	if rec == nil {
		err := releaseContainer(ctx)
		sylog.Debugf("container released; error value: %#v", err)
		return nil, err
	}

	return rec, rec.CloseAsync(releaseContainer)
}

func exitError(ctx context.Context, err error) error {
	if err != nil {
		exitErr := &gatewayapi.ExitError{
			ExitCode: gatewayapi.UnknownExitStatus,
			Err:      err,
		}
		var runcExitError *runc.ExitError
		if errors.As(err, &runcExitError) && runcExitError.Status >= 0 {
			exitErr = &gatewayapi.ExitError{
				ExitCode: uint32(runcExitError.Status),
			}
		}
		select {
		case <-ctx.Done():
			exitErr.Err = errors.Wrap(ctx.Err(), exitErr.Error())
			return exitErr
		default:
			return stack.Enable(exitErr)
		}
	}
	return nil
}

func (w *buildExecutor) Exec(_ context.Context, _ string, _ executor.ProcessInfo) error {
	return nil
}

type forwardIO struct {
	stdin          io.ReadCloser
	stdout, stderr io.WriteCloser
}

func (s *forwardIO) Close() error {
	return nil
}

func (s *forwardIO) Set(cmd *exec.Cmd) {
	cmd.Stdin = s.stdin
	cmd.Stdout = s.stdout
	cmd.Stderr = s.stderr
}

func (s *forwardIO) Stdin() io.WriteCloser {
	return nil
}

func (s *forwardIO) Stdout() io.ReadCloser {
	return nil
}

func (s *forwardIO) Stderr() io.ReadCloser {
	return nil
}

// newRuncProcKiller returns an abstraction for sending SIGKILL to the
// process inside the container initiated from `runc run`.
func newRunProcKiller(runC *runc.Runc, id string) procKiller {
	return procKiller{runC: runC, id: id}
}

type procKiller struct {
	runC    *runc.Runc
	id      string
	pidfile string
	cleanup func()
}

// Cleanup will delete any tmp files created for the pidfile allocation
// if this killer was for a `runc exec` process.
func (k procKiller) Cleanup() {
	if k.cleanup != nil {
		k.cleanup()
	}
}

// Kill will send SIGKILL to the process running inside the container.
// If the process was created by `runc run` then we will use `runc kill`,
// otherwise for `runc exec` we will read the pid from a pidfile and then
// send the signal directly that process.
func (k procKiller) Kill(ctx context.Context) (err error) {
	sylog.Debugf("sending sigkill to process in container %s", k.id)
	defer func() {
		if err != nil {
			sylog.Errorf("failed to kill process in container id %s: %+v", k.id, err)
		}
	}()

	// this timeout is generally a no-op, the Kill ctx should already have a
	// shorter timeout but here as a fail-safe for future refactoring.
	ctx, timeout := context.WithTimeout(ctx, 10*time.Second)
	defer timeout()

	if k.pidfile == "" {
		// for `runc run` process we use `runc kill` to terminate the process
		return k.runC.Kill(ctx, k.id, int(syscall.SIGKILL), nil)
	}

	// `runc exec` will write the pidfile a few milliseconds after we
	// get the runc pid via the startedCh, so we might need to retry until
	// it appears in the edge case where we want to kill a process
	// immediately after it was created.
	var pidData []byte
	for {
		pidData, err = os.ReadFile(k.pidfile)
		if err != nil {
			if os.IsNotExist(err) {
				select {
				case <-ctx.Done():
					return errors.New("context canceled before runc wrote pidfile")
				case <-time.After(10 * time.Millisecond):
					continue
				}
			}
			return errors.Wrap(err, "failed to read pidfile from runc")
		}
		break
	}
	pid, err := strconv.Atoi(string(pidData))
	if err != nil {
		return errors.Wrap(err, "read invalid pid from pidfile")
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		// error only possible on non-unix hosts
		return errors.Wrapf(err, "failed to find process for pid %d from pidfile", pid)
	}
	defer process.Release()
	return process.Signal(syscall.SIGKILL)
}

// procHandle is to track the process so we can send signals to it
// and handle graceful shutdown.
type procHandle struct {
	// this is for the runc process (not the process in-container)
	monitorProcess *os.Process
	ready          chan struct{}
	ended          chan struct{}
	shutdown       func()
	// this only used when the request context is canceled and we need
	// to kill the in-container process.
	killer procKiller
}

// runcProcessHandle will create a procHandle that will be monitored, where
// on ctx.Done the in-container process will receive a SIGKILL.  The returned
// context should be used for the go-runc.(Run|Exec) invocations.  The returned
// context will only be canceled in the case where the request context is
// canceled and we are unable to send the SIGKILL to the in-container process.
// The goal is to allow for runc to gracefully shutdown when the request context
// is canceled.
func runcProcessHandle(ctx context.Context, killer procKiller) (*procHandle, context.Context) {
	runcCtx, cancel := context.WithCancel(context.Background())
	p := &procHandle{
		ready:    make(chan struct{}),
		ended:    make(chan struct{}),
		shutdown: cancel,
		killer:   killer,
	}

	go func() {
		// Wait for pid
		select {
		case <-ctx.Done():
			return // nothing to kill
		case <-p.ready:
		}

		for {
			select {
			case <-ctx.Done():
				killCtx, timeout := context.WithTimeout(ctx, 7*time.Second)
				if err := p.killer.Kill(killCtx); err != nil {
					select {
					case <-killCtx.Done():
						timeout()
						cancel()
						return
					default:
					}
				}
				timeout()
				select {
				case <-time.After(50 * time.Millisecond):
				case <-p.ended:
					return
				}
			case <-p.ended:
				return
			}
		}
	}()

	return p, runcCtx
}

// Release will free resources with a procHandle.
func (p *procHandle) Release() {
	close(p.ended)
	if p.monitorProcess != nil {
		p.monitorProcess.Release()
	}
}

// Shutdown should be called after the runc process has exited. This will allow
// the signal handling and tty resize loops to exit, terminating the
// goroutines.
func (p *procHandle) Shutdown() {
	if p.shutdown != nil {
		p.shutdown()
	}
}

// WaitForReady will wait until we have received the runc pid via the go-runc
// Started channel, or until the request context is canceled.  This should
// return without errors before attempting to send signals to the runc process.
func (p *procHandle) WaitForReady(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-p.ready:
		return nil
	}
}

// WaitForStart will record the runc pid reported by go-runc via the channel.
// We wait for up to 10s for the runc pid to be reported.  If the started
// callback is non-nil it will be called after receiving the pid.
func (p *procHandle) WaitForStart(ctx context.Context, startedCh <-chan int, started func()) error {
	startedCtx, timeout := context.WithTimeout(ctx, 10*time.Second)
	defer timeout()
	select {
	case <-startedCtx.Done():
		return errors.New("go-runc started message never received")
	case runcPid, ok := <-startedCh:
		if !ok {
			return errors.New("go-runc failed to send pid")
		}
		if started != nil {
			started()
		}
		var err error
		p.monitorProcess, err = os.FindProcess(runcPid)
		if err != nil {
			// error only possible on non-unix hosts
			return errors.Wrapf(err, "failed to find runc process %d", runcPid)
		}
		close(p.ready)
	}
	return nil
}

// handleSignals will wait until the procHandle is ready then will
// send each signal received on the channel to the runc process (not directly
// to the in-container process)
func handleSignals(ctx context.Context, runcProcess *procHandle, signals <-chan syscall.Signal) error {
	if signals == nil {
		return nil
	}
	err := runcProcess.WaitForReady(ctx)
	if err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case sig := <-signals:
			if sig == syscall.SIGKILL {
				// never send SIGKILL directly to runc, it needs to go to the
				// process in-container
				if err := runcProcess.killer.Kill(ctx); err != nil {
					return err
				}
				continue
			}
			if err := runcProcess.monitorProcess.Signal(sig); err != nil {
				sylog.Errorf("failed to signal %s to process: %s", sig, err)
				return err
			}
		}
	}
}

func updateRuncFieldsForHostOS(runtime *runc.Runc) {
	// PdeathSignal only supported on unix platforms
	runtime.PdeathSignal = syscall.SIGKILL // this can still leak the process
}

func (w *buildExecutor) run(ctx context.Context, id, bundle string, process executor.ProcessInfo, started func()) error {
	killer := newRunProcKiller(w.runc, id)
	return w.callWithIO(ctx, id, bundle, process, started, killer, func(ctx context.Context, started chan<- int, io runc.IO, _ string) error {
		_, err := w.runc.Run(ctx, id, bundle, &runc.CreateOpts{
			NoPivot: w.noPivot,
			Started: started,
			IO:      io,
		})
		return err
	})
}

type runcCall func(ctx context.Context, started chan<- int, io runc.IO, pidfile string) error

func (w *buildExecutor) callWithIO(ctx context.Context, _, _ string, process executor.ProcessInfo, started func(), killer procKiller, call runcCall) error {
	runcProcess, ctx := runcProcessHandle(ctx, killer)
	defer runcProcess.Release()

	eg, ctx := errgroup.WithContext(ctx)
	defer func() {
		if err := eg.Wait(); err != nil && !errors.Is(err, context.Canceled) {
			sylog.Errorf("runc process monitoring error: %s", err)
		}
	}()
	defer runcProcess.Shutdown()

	startedCh := make(chan int, 1)
	eg.Go(func() error {
		return runcProcess.WaitForStart(ctx, startedCh, started)
	})

	eg.Go(func() error {
		return handleSignals(ctx, runcProcess, process.Signal)
	})

	err := call(ctx, startedCh, &forwardIO{stdin: process.Stdin, stdout: process.Stdout, stderr: process.Stderr}, killer.pidfile)
	sylog.Debugf("internal call finished; error value: %#v", err)

	return err
}
