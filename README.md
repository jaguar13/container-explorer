# Container Explorer

Container Explorer (container-explorer) is a tool to explore containers of a disk image. Container Explorer supports exploring containers managed using containerd and docker container runtimes. Container Explorer attempts to provide the familiar output generated by tools like ctr and docker.

Container Explorer provides the following functionalities:

- Exploring namespaces
- Exploring containers
- Exploring images
- Exploring snapshots
- Exploring contents
- Mounting containers
- Support JSON output

## Usage

The figure below shows the output of the container-explorer --help command.

```console
NAME:
   container-explorer - A standalone utility to explore container details
 
USAGE:
   container-explorer [global options] command [command options] [arguments...]
 
VERSION:
   0.0.2
 
DESCRIPTION:
   A standalone utility to explore container details.
  
  Container explorer supports exploring containers managed using containerd and
  docker. The utility also supports exploring containers created and managed using
  Kubernetes.
  
 
COMMANDS:
   list, ls              Lists container related information
   info                  show internal information
   mount                 mount a container to a mount point
   mount-all, mount_all  mount all containers
   help, h               Shows a list of commands or help for one command
 
GLOBAL OPTIONS:
   --debug                                   enable debug messages
   --containerd-root value, -c value         specify containerd root directory
   --image-root value, -i value              specify mount point for a disk image
   --metadata-file value, -m value           specify the path to containerd metadata file i.e. meta.db
   --snapshot-metadata-file value, -s value  specify the path to containerd snapshot metadata file i.e. metadata.db.
   --namespace value, -n value               specify container namespace (default: "default")
   --docker-managed                          specify docker manages standalone or Kubernetes containers
   --docker-root value                       specify docker root directory. This is only used with flag --docker-managed
   --support-container-data value            a yaml file containing information about support containers
   --output value                            output format in json, table. Default is table (default: "table")
   --help, -h                                show help
   --version, -v                             print the version
```

Container Explorer helps you explore containers on a mounted disk image. Let's assume we have a clone of the Google Kubernetes Engine (GKE) node attached on a forensic VM as `/dev/sdb`. 


1. List the disk partition table.

```bash
sudo fdisk -l /dev/sdb
```

The output of the `fdisk` command.

```console
Disk /dev/sdb: 10 GiB, 10737418240 bytes, 20971520 sectors
Units: sectors of 1 * 512 = 512 bytes
Sector size (logical/physical): 512 bytes / 512 bytes
I/O size (minimum/optimal): 512 bytes / 512 bytes
Disklabel type: gpt
Disk identifier: 7C818738-EDF0-B246-960D-0E7EE8655B06
 
Device     Start      End  Sectors  Size Type
/dev/sdb1  8704000 20971486 12267487  5.8G Linux filesystem
/dev/sdb2    20480    53247    32768   16M ChromeOS kernel
/dev/sdb3  4509696  8703999  4194304    2G ChromeOS root fs
/dev/sdb4    53248    86015    32768   16M ChromeOS kernel
/dev/sdb5   315392  4509695  4194304    2G ChromeOS root fs
/dev/sdb6    16448    16448        1  512B ChromeOS kernel
/dev/sdb7    16449    16449        1  512B ChromeOS root fs
/dev/sdb8    86016   118783    32768   16M Linux filesystem
/dev/sdb9    16450    16450        1  512B ChromeOS reserved
/dev/sdb10   16451    16451        1  512B ChromeOS reserved
/dev/sdb11      64    16447    16384    8M BIOS boot
/dev/sdb12  249856   315391    65536   32M EFI System
```

2. Mount the `/dev/sdb1` as read-only disk on mount point `/mnt/case`.

```bash
sudo mount -o ro,noload,noexec /dev/sdb1 /mnt/case
```

3. Use `container-explorer` to explore the mounted image.

```bash
sudo container-explorer -i /mnt/case --support-container-data supportcontainer.yaml list containers
```

4. Mount an individual container or all containers

  - Mount a container to mount point `/mnt/container`.

```bash
sudo container-explorer -i /mnt/case –support-container-data supportcontainer.yaml -n k8s.io mount f3c910583a81e7441e2cbd209b72afa4740e676ff8d82f2c74fdc5c78e179c10 /container
```

  - Mount all containers to mount point `/mnt/container`. Mounting all containers will create sub-directories using container ID as directory name.

```bash
sudo container-explorer -i /mnt/case –support-container-data supportcontainer.yaml mount-all /mnt/container
```

5. List the mounted containers within `/mnt/container/`.

```bash
sudo ls -l /mnt/container
```

The output of the command.

```console
drwxr-xr-x 1 root root 4096 Feb  5 08:55 3544209cfda893703458d7d0a6a65970bfb46e9be6a60faa1e4e9d0adae11b55
drwxr-xr-x 1 root root 4096 Feb  5 08:54 3646fe81507be0510e9191d7e34adbeb751e7ecd86f7e1657289968828c5c8e3
drwxr-xr-x 1 root root 4096 Feb  5 08:54 68a04caa81f9a4265e53a83b50874faca5a7c8400ee0c064d40d81cde6f03b86
drwxr-xr-x 1 root root 4096 Feb  5 09:14 6f68aeae9c0288c2412f793d3a7b85efac189786ed8da2bdce9f88d39827fb80
drwxr-xr-x 1 root root 4096 Feb  5 08:55 7227972ec83761790a65c137239c48817a26b8ad85be74b1ecf751656a2a61be
drwxr-xr-x 1 root root 4096 Feb  5 09:13 cc9bc4f6c6b35b8a3616d8b4586741d8dc148c62b394d276dfab7572ee5aa542
drwxr-xr-x 1 root root 4096 Feb  5 09:13 d3d1ff8c4ef39acbdf0a44bee6c326786309e408942d6a2d42cbaa1661bac77f
drwxr-xr-x 1 root root 4096 Feb  5 08:54 f3c910583a81e7441e2cbd209b72afa4740e676ff8d82f2c74fdc5c78e179c10
```

6. Use your favorite forensic tool to process mounted containers.

## Mounting Disk Image

Let's assume you have a GKE node disk image as `clone-gke-wp-cluster-default-pool-b4e5d97b-btxm.img`.

1. List the partition table.

```bash
sudo fdisk -l clone-gke-wp-cluster-default-pool-b4e5d97b-btxm.img
```

The output of the `fdisk -l` command.

```console
Disk clone-gke-wp-cluster-default-pool-b4e5d97b-btxm.img: 10 GiB, 10737418240 bytes, 20971520 sectors
Units: sectors of 1 * 512 = 512 bytes
Sector size (logical/physical): 512 bytes / 512 bytes
I/O size (minimum/optimal): 512 bytes / 512 bytes
Disklabel type: gpt
Disk identifier: 7C818738-EDF0-B246-960D-0E7EE8655B06
 
Device                                                  Start      End  Sectors  Size Type
clone-gke-wp-cluster-default-pool-b4e5d97b-btxm.img1  8704000 20971486 12267487  5.8G Linux filesystem
clone-gke-wp-cluster-default-pool-b4e5d97b-btxm.img2    20480    53247    32768   16M ChromeOS kernel
clone-gke-wp-cluster-default-pool-b4e5d97b-btxm.img3  4509696  8703999  4194304    2G ChromeOS root fs
clone-gke-wp-cluster-default-pool-b4e5d97b-btxm.img4    53248    86015    32768   16M ChromeOS kernel
clone-gke-wp-cluster-default-pool-b4e5d97b-btxm.img5   315392  4509695  4194304    2G ChromeOS root fs
clone-gke-wp-cluster-default-pool-b4e5d97b-btxm.img6    16448    16448        1  512B ChromeOS kernel
clone-gke-wp-cluster-default-pool-b4e5d97b-btxm.img7    16449    16449        1  512B ChromeOS root fs
clone-gke-wp-cluster-default-pool-b4e5d97b-btxm.img8    86016   118783    32768   16M Linux filesystem
clone-gke-wp-cluster-default-pool-b4e5d97b-btxm.img9    16450    16450        1  512B ChromeOS reserved
clone-gke-wp-cluster-default-pool-b4e5d97b-btxm.img10   16451    16451        1  512B ChromeOS reserved
clone-gke-wp-cluster-default-pool-b4e5d97b-btxm.img11      64    16447    16384    8M BIOS boot
clone-gke-wp-cluster-default-pool-b4e5d97b-btxm.img12  249856   315391    65536   32M EFI System
```

2. Mount the first partition (Linux Filesystem)

```bash
sudo mount -o ro,noload,noexec,offset=$((8704000*512)) clone-gke-wp-cluster-default-pool-b4e5d97b-btxm.img /mnt/case
```

## Docker Containers

Container Explorer supports exploring Docker managed containers. Use `--docker-managed` global flag to explore Docker containers.

```bash
sudo container-explorer -i /mnt/case --support-container-data supportcontainer.yaml --docker-managed list containers
```

Container Explorer supports the following operation on Docker containers:

- Listing containers
- Listing images
- Mounting an individual container
- Mounting all containers
- Excluding containers by image, hostname, and labels

## Excluding Containers

When a GKE cluster is created, several containers are created to support the Kubernetes. These clusters are used to support Kubernetes only and may not be interesting for the investigation.

The Kubernetes support containers are hidden by default when the global flag `--support-container-data=supportcontainer.yaml` is used.

The `supportcontainer.yaml` contains the commonly known hostname, image, and labels used to identify the support containers.

When `--support-container-data` is used, the `list` and `mount-all` commands automatically ignores the known support containers where applicable. You can use `--show-support-containers` and `--mount-support-containers` to display and mount the support containers.

# Compiling Container Explorer

Follow the steps below to compile the Container Explorer.

1. Verify Golang version is 1.7 or above

```bash
go version
```
	
2. Clone Container Explorer github project

```bash
git clone https://github.com/google/container-explorer
```

3. Compile the code

```bash
cd container-explorer
go build -ldflags '-s -w' -o $HOME/container-explorer cmd/main.go
```

3. Run container-explorer

```bash
$HOME/container-explorer -h
```

