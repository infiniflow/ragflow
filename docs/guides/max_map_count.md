---
sidebar_position: 7
slug: /max_map_count
---

# Update maximum memory map areas

`vm.max_map_count` sets the the maximum number of memory map areas a process may have. The default value is 65530. Raising the limit may increase the memory consumption on the server. most applications need less than a thousand maps. Lowering the value can lead to problematic application behavior because the system will return out-of-memory errors when a process reaches the limit.

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

## Windows and macOS with Docker Desktop

You must set via docker-machine:

```bash
$ docker-machine ssh
$ sudo sysctl -w vm.max_map_count=262144
```

## Windows with Docker Desktop WSL 2 backend


"Windows Subsystem for Linux (WSL) 2 is a full Linux kernel built by Microsoft, which lets Linux distributions run without managing virtual machines. With Docker Desktop running on WSL 2, users can leverage Linux workspaces and avoid maintaining both Linux and Windows build scripts."

To manually set it every time you reboot, you must run the following commands in a command prompt or PowerShell window every time you restart Docker:

```bash
$ wsl -d docker-desktop -u root
$ sysctl -w vm.max_map_count=262144
```
If you are on these versions of WSL and you do not wish to have to run those commands each time you restart Docker, you can globally change every WSL distribution with this setting by modifying your %USERPROFILE%\.wslconfig as follows:

```bash
[wsl2]
kernelCommandLine = "sysctl.vm.max_map_count=262144"
```
This will cause all WSL2 virtual machines to have that setting assigned when they start.

If you are on Windows 11, or Windows 10 version 22H2 and have installed the Microsoft Store version of WSL, you can modify the /etc/sysctl.conf within the "docker-desktop" WSL distribution as follows:

```bash
$ wsl -d docker-desktop -u root
$ vi /etc/sysctl.conf
```
and appending a line which reads:

```bash
vm.max_map_count = 262144
```