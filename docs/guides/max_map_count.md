---
sidebar_position: 7
slug: /max_map_count
---

# Update maximum memory map areas

This guide describes how to update `vm.max_map_count`. This value sets the the maximum number of memory map areas a process may have. Its default value is 65530. While most applications require fewer than a thousand maps, reducing this value can result in abmornal behaviors, and the system will throw out-of-memory errors when a process reaches the limitation. 

RAGFlow v0.7.0 uses Elasticsearch for multiple recall. Setting the value of `vm.max_map_count` correctly is crucial to the proper functioning the Elasticsearch component.

## Linux

This section describes how to update `vm.max_map_count` on Linux:

1. Check the value of `vm.max_map_count`:

   ```bash
   $ sysctl vm.max_map_count
   ```

2. Reset `vm.max_map_count` to a value at least 262144 if it is not.

   ```bash
   # In this case, we set it to 262144:
   $ sudo sysctl -w vm.max_map_count=262144
   ```

   :::caution WARNING
   This change will be reset after a system reboot. If you forget to update the value the next time you start up the server, you may get a `Can't connect to ES cluster` exception.
   :::
   
3. To ensure your change remains permanent, add or update the `vm.max_map_count` value in **/etc/sysctl.conf** accordingly:

   ```bash
   vm.max_map_count=262144
   ```

## macOS

If you are on macOS with Docker Desktop, then you *must* use docker-machine to update `vm.max_map_count`:

```bash
$ docker-machine ssh
$ sudo sysctl -w vm.max_map_count=262144
```

:::caution WARNING
This change will be reset after a system reboot. If you forget to update the value the next time you start up the server, you may get a `Can't connect to ES cluster` exception.
:::

## Windows

This section provides guidance on updating `vm.max_map_count` on Windows. 

- If you are on Windows with Docker Desktop, then you *must* use docker-machine to set `vm.max_map_count`:

   ```bash
   $ docker-machine ssh
   $ sudo sysctl -w vm.max_map_count=262144
   ```
- If you are on Windows with Docker Desktop WSL 2 backend, then use docker-desktop to set `vm.max_map_count`:

   1. Run the following in WSL: 
   ```bash
   $ wsl -d docker-desktop -u root
   $ sysctl -w vm.max_map_count=262144
   ```

   :::caution WARNING
   This change will be reset after you restart Docker. If you forget to update the value the next time you start up the server, you may get a `Can't connect to ES cluster` exception.
   :::

   2. If you do not wish to have to run those commands each time you restart Docker, you can update your `%USERPROFILE%\.wslconfig` as follows to keep your change permanent and globally for all WSL distributions:

   ```bash
   [wsl2]
    kernelCommandLine = "sysctl.vm.max_map_count=262144"
   ```
   *This causes all WSL2 virtual machines to have that setting assigned when they start.*

   :::note
   If you are on Windows 11 or Windows 10 version 22H2, and have installed the Microsoft Store version of WSL, you can also update the **/etc/sysctl.conf** within the docker-desktop WSL distribution to keep your change permanent:

   ```bash
   $ wsl -d docker-desktop -u root
   $ vi /etc/sysctl.conf
   ```

   ```bash
   # Append a line, which reads: 
   vm.max_map_count = 262144
   ```
   :::