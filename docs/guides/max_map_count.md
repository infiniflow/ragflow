---
sidebar_position: 7
slug: /max_map_count
---

# Update vm.max_map_count

## Linux

To check the value of `vm.max_map_count`:

```bash
$ sysctl vm.max_map_count
```

Reset `vm.max_map_count` to a value at least 262144 if it is not.

```bash
# In this case, we set it to 262144:
$ sudo sysctl -w vm.max_map_count=262144
```

This change will be reset after a system reboot. To ensure your change remains permanent, add or update the `vm.max_map_count` value in **/etc/sysctl.conf** accordingly:

```bash
vm.max_map_count=262144
```

## Mac

```bash
$ screen ~/Library/Containers/com.docker.docker/Data/vms/0/tty
$ sysctl -w vm.max_map_count=262144
```
To exit the screen session, type Ctrl a d.

## Windows and macOS with Docker Desktop

The vm.max_map_count setting must be set via docker-machine:

```bash
$ docker-machine ssh
$ sudo sysctl -w vm.max_map_count=262144
```

## Windows with Docker Desktop WSL 2 backend

To manually set it every time you reboot, you must run the following commands in a command prompt or PowerShell window every time you restart Docker:

```bash
$ wsl -d docker-desktop -u root
$ sysctl -w vm.max_map_count=262144
```
If you are on these versions of WSL and you do not want to have to run those commands every time you restart Docker, you can globally change every WSL distribution with this setting by modifying your %USERPROFILE%\.wslconfig as follows:

```bash
[wsl2]
kernelCommandLine = "sysctl.vm.max_map_count=262144"
```
This will cause all WSL2 VMs to have that setting assigned when they start.

If you are on Windows 11, or Windows 10 version 22H2 and have installed the Microsoft Store version of WSL, you can modify the /etc/sysctl.conf within the "docker-desktop" WSL distribution, perhaps with commands like this:

```bash
$ wsl -d docker-desktop -u root
$ vi /etc/sysctl.conf
```
and appending a line which reads:
```bash
vm.max_map_count = 262144
```