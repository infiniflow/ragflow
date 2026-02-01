---
sidebar_position: 10
slug: /mount-nas-storage
---

# Mount NAS Storage for Paperless Documents

This guide explains how to mount Network Attached Storage (NAS) containing Paperless documents or other external document repositories to RAGFlow.

## Overview

RAGFlow can access documents from external storage locations like NAS drives containing Paperless-ngx archives, shared network drives, or other file servers. By mounting these storage locations as Docker volumes, RAGFlow can directly access and process your documents without needing to copy them.

## Prerequisites

- RAGFlow installed and running via Docker Compose
- Network access to your NAS device
- Appropriate permissions to access the NAS share
- NFS or SMB/CIFS support on your host system

## Method 1: Mount NAS on Host, Then Bind to Container

This is the **recommended approach** as it's simpler and more portable.

### Step 1: Mount NAS on your host system

#### For NFS shares:

```bash
# Create a mount point on your host
sudo mkdir -p /mnt/nas-paperless

# Mount the NFS share
sudo mount -t nfs <NAS_IP>:/path/to/paperless/documents /mnt/nas-paperless

# Verify the mount
ls -la /mnt/nas-paperless
```

To make the mount persistent across reboots, add to `/etc/fstab`:

```bash
<NAS_IP>:/path/to/paperless/documents  /mnt/nas-paperless  nfs  defaults,_netdev  0  0
```

#### For SMB/CIFS shares:

```bash
# Create a mount point on your host
sudo mkdir -p /mnt/nas-paperless

# Install cifs-utils if not already installed
# Ubuntu/Debian:
sudo apt-get install cifs-utils
# RHEL/CentOS:
sudo yum install cifs-utils

# Mount the SMB share
sudo mount -t cifs //<NAS_IP>/paperless /mnt/nas-paperless -o username=<USERNAME>,password=<PASSWORD>

# Verify the mount
ls -la /mnt/nas-paperless
```

For persistent mount, add to `/etc/fstab`:

```bash
//<NAS_IP>/paperless  /mnt/nas-paperless  cifs  username=<USERNAME>,password=<PASSWORD>,_netdev  0  0
```

**Security tip**: Use a credentials file instead of storing passwords in `/etc/fstab`:

```bash
# Create credentials file
sudo nano /root/.smbcredentials

# Add these lines:
username=<USERNAME>
password=<PASSWORD>

# Secure the file
sudo chmod 600 /root/.smbcredentials

# Update /etc/fstab to use credentials file:
//<NAS_IP>/paperless  /mnt/nas-paperless  cifs  credentials=/root/.smbcredentials,_netdev  0  0
```

### Step 2: Update docker-compose.yml

Add the mounted directory to your RAGFlow container volumes:

```yaml
services:
  ragflow-cpu:  # or ragflow-gpu depending on your profile
    # ... existing configuration ...
    volumes:
      # Existing volumes
      - ./ragflow-logs:/ragflow/logs
      - ./nginx/ragflow.conf:/etc/nginx/conf.d/ragflow.conf
      - ./nginx/proxy.conf:/etc/nginx/proxy.conf
      - ./nginx/nginx.conf:/etc/nginx/nginx.conf
      - ./service_conf.yaml.template:/ragflow/conf/service_conf.yaml.template
      - ./entrypoint.sh:/ragflow/entrypoint.sh
      
      # Add your NAS Paperless documents mount
      - /mnt/nas-paperless:/ragflow/paperless:ro  # :ro for read-only access
```

**Path explanation:**
- `/mnt/nas-paperless` - The host mount point where your NAS is mounted
- `/ragflow/paperless` - The path inside the RAGFlow container where documents will be accessible
- `:ro` - Optional read-only flag to prevent accidental modifications

### Step 3: Restart RAGFlow

```bash
cd docker
docker compose down
docker compose up -d
```

### Step 4: Verify access

```bash
# Check if documents are accessible from within the container
docker exec -it ragflow-cpu ls -la /ragflow/paperless
```

## Method 2: Direct NFS Mount in Docker (Advanced)

Docker also supports direct NFS mounts in the compose file:

```yaml
services:
  ragflow-cpu:
    # ... existing configuration ...
    volumes:
      # Existing volumes
      - ./ragflow-logs:/ragflow/logs
      # ... other existing volumes ...
      
      # Direct NFS mount
      - type: volume
        source: paperless-docs
        target: /ragflow/paperless
        read_only: true
        volume:
          nocopy: true

volumes:
  paperless-docs:
    driver: local
    driver_opts:
      type: nfs
      o: addr=<NAS_IP>,ro,nfsvers=4
      device: ":/path/to/paperless/documents"
```

## Paperless-ngx Specific Configuration

If you're using Paperless-ngx, typical directory structure is:

```
/path/to/paperless/
├── consume/      # Documents waiting to be processed
├── media/        # Processed documents (organized by date)
│   └── documents/
│       └── 2024/
│           └── 01/
├── export/       # Exported documents
└── archive/      # Original documents
```

You can mount specific subdirectories:

```yaml
volumes:
  # Mount only the media/documents directory
  - /mnt/nas-paperless/media/documents:/ragflow/paperless/documents:ro
  
  # Or mount multiple directories
  - /mnt/nas-paperless/media/documents:/ragflow/paperless/media:ro
  - /mnt/nas-paperless/archive:/ragflow/paperless/archive:ro
```

## Using Documents in RAGFlow

After mounting your NAS storage:

1. **Access via RAGFlow UI**:
   - Navigate to the Knowledge Base section
   - Click "Upload" → "Local Files"
   - Browse to `/ragflow/paperless` directory
   - Select documents to add to your knowledge base

2. **Bulk import via API**:
   - Use RAGFlow's API to batch import documents
   - Point to the mounted directory path `/ragflow/paperless`

## Troubleshooting

### Permission Issues

If you encounter permission errors:

```bash
# Check ownership of the mounted directory
ls -la /mnt/nas-paperless

# The RAGFlow container typically runs as a specific user
# You may need to adjust permissions or use bindfs

# Install bindfs
sudo apt-get install bindfs

# Remount with specific user/group
sudo bindfs -u $(id -u) -g $(id -g) /mnt/nas-paperless /mnt/nas-paperless-mapped

# Then use /mnt/nas-paperless-mapped in docker-compose.yml
```

### Connection Issues

```bash
# Test NFS connectivity
showmount -e <NAS_IP>

# Test SMB connectivity
smbclient -L //<NAS_IP> -U <USERNAME>

# Check mount status
mount | grep nas-paperless

# View NFS mount statistics
nfsstat -m
```

### Container Can't See Files

```bash
# Verify the mount inside the container
docker exec -it ragflow-cpu ls -la /ragflow/paperless

# Check SELinux context (if using RHEL/CentOS)
ls -Z /mnt/nas-paperless

# Fix SELinux context if needed
sudo chcon -Rt svirt_sandbox_file_t /mnt/nas-paperless
```

## Performance Considerations

- **Read-only mounts**: Use `:ro` flag to prevent accidental writes and improve performance
- **Network latency**: NAS access will be slower than local storage; consider this when processing large documents
- **Caching**: Enable caching in your NFS mount options for better performance:
  ```bash
  sudo mount -t nfs -o rw,async,hard,intr <NAS_IP>:/path /mnt/nas-paperless
  ```

## Security Best Practices

1. **Use read-only mounts** when RAGFlow only needs to read documents
2. **Limit network access** to the NAS using firewall rules
3. **Use credentials files** instead of storing passwords in configuration files
4. **Enable encryption** for SMB (SMB3+) or use encrypted NFS
5. **Regular audits** of access logs on both RAGFlow and NAS

## Example: Complete docker-compose.yml with NAS Mount

```yaml
include:
  - ./docker-compose-base.yml

services:
  ragflow-cpu:
    depends_on:
      mysql:
        condition: service_healthy
    profiles:
      - cpu
    image: ${RAGFLOW_IMAGE}
    command:
      - --enable-adminserver
    ports:
      - ${SVR_WEB_HTTP_PORT}:80
      - ${SVR_WEB_HTTPS_PORT}:443
      - ${SVR_HTTP_PORT}:9380
      - ${ADMIN_SVR_HTTP_PORT}:9381
      - ${SVR_MCP_PORT}:9382
    volumes:
      # Standard RAGFlow volumes
      - ./ragflow-logs:/ragflow/logs
      - ./nginx/ragflow.conf:/etc/nginx/conf.d/ragflow.conf
      - ./nginx/proxy.conf:/etc/nginx/proxy.conf
      - ./nginx/nginx.conf:/etc/nginx/nginx.conf
      - ./service_conf.yaml.template:/ragflow/conf/service_conf.yaml.template
      - ./entrypoint.sh:/ragflow/entrypoint.sh
      
      # NAS Paperless documents mount
      - /mnt/nas-paperless/media/documents:/ragflow/paperless/documents:ro
      - /mnt/nas-paperless/archive:/ragflow/paperless/archive:ro
      
    env_file: .env
    networks:
      - ragflow
    restart: unless-stopped
    extra_hosts:
      - "host.docker.internal:host-gateway"
```

## Alternative: Using Docker Volume Plugins

For more advanced setups, consider using Docker volume plugins:

- **Netshare plugin**: Supports NFS and CIFS
- **REX-Ray**: Enterprise-grade storage orchestration
- **Convoy**: Snapshot and backup support

Example with netshare:

```bash
# Install netshare plugin
docker plugin install vieux/netshare

# Create volume
docker volume create --driver vieux/netshare:nfs \
  --opt share=<NAS_IP>:/path/to/paperless \
  paperless-docs

# Use in docker-compose.yml
volumes:
  - paperless-docs:/ragflow/paperless:ro
```

## Summary

Mounting NAS storage for Paperless documents in RAGFlow involves:

1. Mounting the NAS share on your host system (NFS or SMB/CIFS)
2. Adding the mount as a volume in `docker-compose.yml`
3. Restarting the RAGFlow containers
4. Accessing documents through the RAGFlow UI or API

The recommended approach is to mount on the host first, then bind to the container, as it provides better portability and easier troubleshooting.
