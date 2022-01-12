/*
Copyright 2021 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package containerd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/containerd/containerd/containers"
	"github.com/containerd/containerd/metadata"
	"github.com/containerd/containerd/namespaces"
	"github.com/gogo/protobuf/types"
	"github.com/google/container-explorer/explorers"

	spec "github.com/opencontainers/runtime-spec/specs-go"

	log "github.com/sirupsen/logrus"
	bolt "go.etcd.io/bbolt"
)

type explorer struct {
	imageroot string   // mounted image path
	root      string   // containerd root
	manifest  string   // path to manifest database file i.e. meta.db
	snapshot  string   // path to snapshot database file i.e. metadata.db
	mdb       *bolt.DB // manifest database
}

// NewExplorer returns a ContainerExplorer interface to explore containerd.
func NewExplorer(imageroot string, root string, manifest string, snapshot string) (explorers.ContainerExplorer, error) {
	opt := &bolt.Options{
		ReadOnly: true,
	}
	db, err := bolt.Open(manifest, 0444, opt)
	if err != nil {
		return &explorer{}, err
	}

	return &explorer{
		imageroot: imageroot,
		root:      root,
		manifest:  manifest,
		snapshot:  snapshot,
		mdb:       db,
	}, nil
}

// SnapshotRoot returns the root directory containing snapshot information.
//
// Containerd requires snapshot database metadata.db which is stored within
// the snapshot root directory.
//
// The default snapshot root directrion location for containerd is
// /var/lib/containerd/io.containerd.snapshotter.v1.overlayfs
func (e *explorer) SnapshotRoot(snapshotter string) string {
	dirs, _ := filepath.Glob(filepath.Join(e.root, "*"))
	for _, dir := range dirs {
		fmt.Println(dir)
		if strings.Contains(strings.ToLower(dir), strings.ToLower(snapshotter)) {
			return dir
		}
	}
	return "unknown"
}

// ListNamespace returns namespaces.
//
// In containerd the namespace information is stored in metadata file meta.db.
func (e *explorer) ListNamespaces(ctx context.Context) ([]string, error) {
	var nss []string

	err := e.mdb.View(func(tx *bolt.Tx) error {
		store := metadata.NewNamespaceStore(tx)
		results, err := store.List(ctx)
		if err != nil {
			return err
		}
		nss = results
		return nil
	})
	if err != nil {
		return nil, err
	}

	return nss, nil
}

// ListContainers returns the information about containers.
//
// In containerd the container information is stored in metadata file meta.db.
func (e *explorer) ListContainers(ctx context.Context) ([]explorers.Container, error) {
	var cecontainers []explorers.Container

	nss, err := e.ListNamespaces(ctx)
	if err != nil {
		return nil, err
	}

	store := metadata.NewContainerStore(metadata.NewDB(e.mdb, nil, nil))

	for _, ns := range nss {
		ctx = namespaces.WithNamespace(ctx, ns)

		results, err := store.List(ctx)
		if err != nil {
			return nil, err
		}

		for _, result := range results {
			cecontainers = append(cecontainers, convertToContainerExplorerContainer(ns, result))
		}
	}
	return cecontainers, nil
}

// ListImages returns the information about content.
//
// In containerd, the image information is stored in metadata file meta.db.
func (e *explorer) ListImages(ctx context.Context) ([]explorers.Image, error) {
	var ceimages []explorers.Image

	nss, err := e.ListNamespaces(ctx)
	if err != nil {
		return nil, err
	}

	store := metadata.NewImageStore(metadata.NewDB(e.mdb, nil, nil))

	for _, ns := range nss {
		ctx = namespaces.WithNamespace(ctx, ns)

		results, err := store.List(ctx)
		if err != nil {
			return nil, err
		}

		for _, result := range results {
			//ceimages = append(ceimages, convertToContainerExplorerImage(ns, result))
			ceimages = append(ceimages, explorers.Image{
				Namespace:             ns,
				SupportContainerImage: isKubernetesSupportContainerImage(result.Name),
				Image:                 result,
			})
		}
	}
	return ceimages, nil
}

// ListContent returns the information about content.
//
// In containerd, the content information is stored in metadata file meta.db.
func (e *explorer) ListContent(ctx context.Context) ([]explorers.Content, error) {
	var cecontent []explorers.Content

	nss, err := e.ListNamespaces(ctx)
	if err != nil {
		return nil, err
	}

	store := NewBlobStore(e.mdb)

	for _, ns := range nss {
		ctx = namespaces.WithNamespace(ctx, ns)

		results, err := store.List(ctx)
		if err != nil {
			return nil, err
		}

		for _, result := range results {
			cecontent = append(cecontent, explorers.Content{
				Namespace: ns,
				Info:      result,
			})
		}
	}

	return cecontent, nil
}

// ListSnapshots returns the snapshot information.
//
// In containerd, the snapshot information is stored in two different files:
// - metadata file (meta.db)
// - snapshot file (metadata.db)
//
// These files contain some overlapping fields.
//
// The metadata file meta.db contains snapshot information and container
// references the the snapshot information.
//
// The snapshot file metadata.db contains information about the snapshots only
// without reference to a container. This file also containers informations
// that are more relevant to manage snapshots.
//
// For Examples:
//   - Snapshot type i.e. active or committed
//   - Snapshot ID that refers to overlay path i.e /var/lib/containerd/io.containerd.snapshotter.v1.overlayfs/snapshots/<id>/fs
//
// Snapshot ID is required when mounting the container.
func (e *explorer) ListSnapshots(ctx context.Context) ([]explorers.SnapshotKeyInfo, error) {
	var cesnapshots []explorers.SnapshotKeyInfo

	nss, err := e.ListNamespaces(ctx)
	if err != nil {
		return nil, err
	}

	// snapshot database
	opts := bolt.Options{
		ReadOnly: true,
	}
	ssdb, err := bolt.Open(e.snapshot, 0444, &opts)
	if err != nil {
		log.WithFields(log.Fields{
			"snapshotfile": e.snapshot,
		}).Error(err)
	}

	store := NewSnaptshotStore(e.root, e.mdb, ssdb)

	for _, ns := range nss {
		ctx = namespaces.WithNamespace(ctx, ns)

		results, err := store.List(ctx)
		if err != nil {
			return nil, err
		}

		cesnapshots = append(cesnapshots, results...)
	}

	return cesnapshots, nil
}

// ListTasks returns container tasks status
func (e *explorer) ListTasks(ctx context.Context) ([]explorers.Task, error) {
	if e.imageroot == "" {
		log.Error("image-root is empty. Unable to list tasks.")
		return nil, nil
	}

	// Holds container task information.
	var cetasks []explorers.Task

	ctrs, err := e.ListContainers(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}
	for _, ctr := range ctrs {
		cetask, err := e.GetContainerTask(ctx, ctr)
		if err != nil {
			return nil, fmt.Errorf("failed getting a container's task: %w", err)
		}

		cetasks = append(cetasks, cetask)
	}

	return cetasks, nil
}

// GetContainerTask returns container task
func (e *explorer) GetContainerTask(ctx context.Context, ctr explorers.Container) (explorers.Task, error) {
	ctx = namespaces.WithNamespace(ctx, ctr.Namespace)

	// Only return container spec
	v, err := e.InfoContainer(ctx, ctr.ID, true)
	if err != nil {
		return explorers.Task{}, fmt.Errorf("failed getting container spec for %s container: %w", ctr.ID, err)
	}
	ctrspec := v.(spec.Spec)

	var cgroupspath string
	var containertype string

	// Compute cgroup path for docker and containerd containers
	if strings.Contains(ctrspec.Linux.CgroupsPath, "docker") {
		containertype = "docker"

		// compute for docker
		//
		// Spec file `config.json` contains key cgroupsPath as `system.slice:docker:<container_id>`.
		// The path maps on file system to `/sys/fs/cgroup/system.slice/docker-<container_id>.scope`.
		m := strings.Split(ctrspec.Linux.CgroupsPath, ":")
		if len(m) != 3 {
			return explorers.Task{}, fmt.Errorf("expecting pattern system.slice:docker:<container_id> and got %d fields", len(m))
		}

		// docker cgroup directory i.e. system.slice
		cgroupns := m[0]
		// container cgroup information
		cgroupctrdir := fmt.Sprintf("%s-%s.scope", m[1], m[2])
		// abolute path to container cgroup directory
		cgroupspath = filepath.Join(e.imageroot, "sys", "fs", "cgroup", cgroupns, cgroupctrdir)
	} else {
		containertype = "containerd"

		// compute for containerd
		//
		// Spec file contains "cgroupsPath": "/default/<container_id>",
		cgroupspath = filepath.Join(e.imageroot, "sys", "fs", "cgroup", ctrspec.Linux.CgroupsPath)
	}

	// Verify the path actually exist on the system.
	// If a container is deleted then cgroup may not exist for the container
	if !pathExists(cgroupspath, false) {
		log.WithFields(log.Fields{
			"contianerid": ctr.ID,
			"cgroupspath": cgroupspath,
		}).Error("container cgroup path does not exit")

		return explorers.Task{
			Namespace:     ctr.Namespace,
			Name:          ctr.ID,
			ContainerType: containertype,
			Status:        "TERMINATED",
		}, nil
	}

	status, err := getTaskStatus(cgroupspath)
	if err != nil {
		// Only print the error message.
		// The default return should contain status UNKNOWN
		log.WithField("containerid", ctr.ID).Error("failed getting container status for container: ", err)
	}

	// Get container process ID
	ctrpid := getTaskPID(cgroupspath)
	if ctrpid == -1 && containertype == "containerd" {
		state, err := e.GetContainerState(ctx, ctr)
		if err != nil {
			log.WithField("containerid", ctr.ID).Error("failed getting container state")
		}
		if state.InitProcessPid != 0 {
			ctrpid = state.InitProcessPid
		}
	}

	return explorers.Task{
		Namespace:     ctr.Namespace,
		Name:          ctr.ID,
		PID:           ctrpid,
		ContainerType: containertype,
		Status:        status,
	}, nil
}

// GetContainerState returns container runtime state
func (e *explorer) GetContainerState(ctx context.Context, ctr explorers.Container) (explorers.State, error) {
	statedir := filepath.Join(e.imageroot, "run", "containerd", "runc", ctr.Namespace, ctr.ID)
	if !pathExists(statedir, false) {
		return explorers.State{}, fmt.Errorf("container state directory %s did not exist", statedir)
	}

	statefile := filepath.Join(statedir, "state.json")
	if !pathExists(statefile, true) {
		return explorers.State{}, fmt.Errorf("container state file %s did not exist", statefile)
	}

	data, err := os.ReadFile(statefile)
	if err != nil {
		return explorers.State{}, err
	}

	var state explorers.State
	if err := json.Unmarshal(data, &state); err != nil {
		return explorers.State{}, fmt.Errorf("unmarshalling state data: %w", err)
	}
	return state, nil
}

// InfoContainer returns container internal information.
func (e *explorer) InfoContainer(ctx context.Context, containerid string, spec bool) (interface{}, error) {
	store := metadata.NewContainerStore(metadata.NewDB(e.mdb, nil, nil))

	container, err := store.Get(ctx, containerid)
	if err != nil {
		return nil, err
	}

	if container.Spec != nil && container.Spec.Value != nil {
		v, err := parseSpec(container.Spec)
		if err != nil {
			return nil, err
		}

		// Only return spec
		if spec {
			return v, nil
		}

		// Return container and spec info
		return struct {
			containers.Container
			Spec interface{} `json:"Spec,omitempty"`
		}{
			Container: container,
			Spec:      v,
		}, nil
	}

	// default return
	return nil, nil
}

// MountContainer mounts a container to the specified path
func (e *explorer) MountContainer(ctx context.Context, containerid string, mountpoint string) error {
	store := metadata.NewContainerStore(metadata.NewDB(e.mdb, nil, nil))

	container, err := store.Get(ctx, containerid)
	if err != nil {
		return fmt.Errorf("failed getting container information %v", err)
	}
	log.WithFields(log.Fields{
		"snapshotter": container.Snapshotter,
		"snapshotKey": container.SnapshotKey,
		"image":       container.Image,
	}).Debug("container snapshotter")

	// Snapshot database metadata.db access
	opts := bolt.Options{
		ReadOnly: true,
	}
	ssdb, err := bolt.Open(e.snapshot, 0444, &opts)
	if err != nil {
		return fmt.Errorf("failed to open snapshot database %v", err)
	}

	// snapshot store
	ssstore := NewSnaptshotStore(e.root, e.mdb, ssdb)
	lowerdir, upperdir, workdir, err := ssstore.OverlayPath(ctx, container)
	log.WithFields(log.Fields{
		"lowerdir": lowerdir,
		"upperdir": upperdir,
		"workdir":  workdir,
	}).Debug("overlay directories")
	if err != nil {
		return fmt.Errorf("failed to get overlay path %v", err)
	}

	if lowerdir == "" {
		return fmt.Errorf("lowerdir is empty")
	}

	// TODO(rmaskey): Use github.com/containerd/containerd/mount.Mount to mount
	// a container
	mountopts := fmt.Sprintf("ro,lowerdir=%s:%s", upperdir, lowerdir)
	mountArgs := []string{"-t", "overlay", "overlay", "-o", mountopts, mountpoint}
	log.Debug("container mount command ", mountArgs)

	cmd := exec.Command("mount", mountArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Errorf("running mount command %v", err)

		if strings.Contains(err.Error(), " 32") {
			log.Error("invalid overlayfs lowerdir path. Use --debug to view lowerdir path")
		}

		return err
	}

	if string(out) != "" {
		log.Info("mount command output ", string(out))
	}

	// default
	return nil
}

// MountAllContainers mounts all the containers
func (e *explorer) MountAllContainers(ctx context.Context, mountpoint string, skipsupportcontainers bool) error {
	ctrs, err := e.ListContainers(ctx)
	if err != nil {
		return err
	}

	for _, ctr := range ctrs {
		// Skip Kubernetes suppot containers
		if skipsupportcontainers && ctr.SupportContainer {
			log.WithFields(log.Fields{
				"namespace":   ctr.Namespace,
				"containerid": ctr.ID,
			}).Info("skip mounting Kubernetes containers")

			continue
		}

		// Create a subdirectory within the specified mountpoint
		ctrmountpoint := filepath.Join(mountpoint, ctr.ID)
		if err := os.MkdirAll(ctrmountpoint, 0755); err != nil {
			log.WithFields(log.Fields{
				"namespace":   ctr.Namespace,
				"containerid": ctr.ID,
				"mountpoint":  mountpoint,
			}).Error("creating mount point for a container")

			log.WithField("containerid", ctr.ID).Warn("skipping container mount")
			continue
		}

		ctx = namespaces.WithNamespace(ctx, ctr.Namespace)
		if err := e.MountContainer(ctx, ctr.ID, ctrmountpoint); err != nil {
			return err
		}
	}

	// default
	return nil
}

// Close releases the internal resources
func (e *explorer) Close() error {
	return e.mdb.Close()
}

// convertToContainerExplorerContainer returns a Container object which is
// superset of containers.Container object.
func convertToContainerExplorerContainer(ns string, ctr containers.Container) explorers.Container {
	var hostname string
	if ctr.Spec != nil && ctr.Spec.Value != nil {
		var v spec.Spec
		json.Unmarshal(ctr.Spec.Value, &v)

		if v.Hostname != "" {
			hostname = v.Hostname
		} else {
			for _, kv := range v.Process.Env {
				if strings.HasPrefix(kv, "HOSTNAME=") {
					hostname = strings.TrimSpace(strings.Split(kv, "=")[1])
					break
				}
			}
		}
	}

	return explorers.Container{
		Namespace:        ns,
		Hostname:         hostname,
		SupportContainer: isKubernetesSupportContainer(ctr),
		Container:        ctr,
	}
}

// The support containers created by GKE are labeled as
// io.kubernetes.pod.namespace="kube-system"
const (
	k8sPodNamespace        = "io.kubernetes.pod.namespace"
	k8sSupportPodNamespace = "kube-system"
)

// isKubernetesSupportContainer returns true for a container that was created
// by Kubernetes to facilitate the management of containers.
//
// Example of such containers are kubeproxy, kube-dns etc.
func isKubernetesSupportContainer(ctr containers.Container) bool {
	// Checking for container label kube-system
	if val, found := ctr.Labels[k8sPodNamespace]; found {
		if val == k8sSupportPodNamespace {
			return true
		}
	}
	return isKubernetesSupportContainerImage(ctr.Image)
}

// isKubernetesSupportContainerImage returns true if the container image is
// used to create support container
//
// Example: gke.gcr.io/gke-metrics-agent:1.2.0-gke.0
func isKubernetesSupportContainerImage(imagename string) bool {
	supportcontainerimage := false
	imagebase := imagename

	if strings.Contains(imagebase, "@") {
		imagebase = strings.Split(imagebase, "@")[0]
	}

	if strings.Contains(imagebase, ":") {
		imagebase = strings.Split(imagebase, ":")[0]
	}

	if _, found := explorers.KubernetesSupportContainers[imagebase]; found {
		supportcontainerimage = true
	}

	log.WithFields(log.Fields{
		"imagebase":             imagebase,
		"supportcontainerimage": supportcontainerimage,
	}).Debug("Kubernetes support container image")

	return supportcontainerimage
}

// parseSpec parses containerd spec and returns the information as JSON.
func parseSpec(any *types.Any) (interface{}, error) {
	var v spec.Spec
	json.Unmarshal(any.Value, &v)
	return v, nil
}

// getTaskStatus returns task status
func getTaskStatus(cgrouppath string) (string, error) {
	populated, frozen, err := readCgroupEvents(cgrouppath)
	if err != nil {
		return "UNKNOWN", fmt.Errorf("reading group.events: %w", err)
	}

	if populated == 0 && frozen == 0 {
		return "STOPPED", nil
	} else if populated == 1 && frozen == 0 {
		return "RUNNING", nil
	} else if populated == 1 && frozen == 1 {
		return "PAUSED", nil
	}

	return "UNKNOWN", fmt.Errorf("unknown status with values populated: %d, frozen: %d", populated, frozen)
}

// getTaskPID returns process ID of the containers
func getTaskPID(path string) int {
	pidfile := filepath.Join(path, "cgroup.procs")
	if !pathExists(pidfile, true) {
		return -1
	}

	data, err := os.ReadFile(pidfile)
	if err != nil {
		log.WithField("path", pidfile).Error("reading cgroup.procs: ", err)
		return -1
	}

	pid, err := strconv.Atoi(strings.Split(string(data), "\n")[0])
	if err != nil {
		log.WithField("path", pidfile).Info("converting to int: ", err)
		return -1
	}
	return pid
}

// readCgroupEvents returns populated and frozen status
func readCgroupEvents(path string) (int, int, error) {
	data, err := os.ReadFile(filepath.Join(path, "cgroup.events"))
	if err != nil {
		return -1, -1, err
	}

	populated := -1
	frozen := -1

	for _, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, "populated ") {
			val := strings.Replace(line, "populated ", "", -1)
			val = strings.TrimSpace(val)

			populated, err = strconv.Atoi(val)
			if err != nil {
				populated = -1
			}
		}

		if strings.Contains(line, "frozen ") {
			val := strings.Replace(line, "frozen ", "", -1)
			val = strings.TrimSpace(val)

			frozen, err = strconv.Atoi(val)
			if err != nil {
				frozen = -1
			}
		}
	}
	return populated, frozen, nil
}

// pathExists returns true if the path exists
func pathExists(path string, isfile bool) bool {
	finfo, err := os.Stat(path)
	if err != nil {
		return false
	}

	if isfile {
		return !finfo.IsDir()
	}
	return finfo.IsDir()
}
