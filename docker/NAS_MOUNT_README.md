# Mounting NAS Paperless Documents to RAGFlow

This directory contains resources to help you mount Network Attached Storage (NAS) containing Paperless documents or other external document repositories to RAGFlow.

## Quick Start

### Option 1: Automated Setup (Recommended)

Use the helper script to mount your NAS and configure RAGFlow:

```bash
cd docker
sudo ./mount_nas_paperless.sh
```

The script will:
1. Guide you through mounting your NAS (NFS or SMB/CIFS)
2. Create the necessary mount point
3. Optionally add the mount to `/etc/fstab` for persistence
4. Help update your `docker-compose.yml`

### Option 2: Manual Setup

1. **Mount your NAS on the host system:**

   For NFS:
   ```bash
   sudo mkdir -p /mnt/nas-paperless
   sudo mount -t nfs <NAS_IP>:/path/to/paperless /mnt/nas-paperless
   ```

   For SMB/CIFS:
   ```bash
   sudo mkdir -p /mnt/nas-paperless
   sudo mount -t cifs //<NAS_IP>/paperless /mnt/nas-paperless -o username=<USER>,password=<PASS>
   ```

2. **Update docker-compose.yml:**

   Add to the volumes section of your RAGFlow service:
   ```yaml
   volumes:
     # ... existing volumes ...
     - /mnt/nas-paperless:/ragflow/paperless:ro
   ```

3. **Restart RAGFlow:**
   ```bash
   docker compose down
   docker compose up -d
   ```

4. **Verify:**
   ```bash
   docker exec -it ragflow-cpu ls -la /ragflow/paperless
   ```

## Files in This Directory

- **`mount_nas_paperless.sh`** - Interactive script to help mount NAS and configure RAGFlow
- **`docker-compose.example-nas-mount.yml`** - Example docker-compose configuration showing various NAS mount options
- **`../docs/guides/mount_nas_storage.md`** - Comprehensive documentation with troubleshooting

## Typical Use Cases

### 1. Paperless-ngx Documents

If you're running Paperless-ngx, mount the documents directory:

```yaml
volumes:
  - /mnt/nas-paperless/media/documents:/ragflow/paperless/media:ro
  - /mnt/nas-paperless/archive:/ragflow/paperless/archive:ro
```

### 2. General Document Storage

For any shared document repository:

```yaml
volumes:
  - /mnt/network-drive/documents:/ragflow/documents:ro
```

### 3. Multiple NAS Sources

You can mount multiple NAS locations:

```yaml
volumes:
  - /mnt/nas1/paperless:/ragflow/paperless:ro
  - /mnt/nas2/archives:/ragflow/archives:ro
  - /mnt/nas3/reports:/ragflow/reports:ro
```

## Important Notes

### Read-Only vs Read-Write

- Use `:ro` (read-only) when RAGFlow only needs to read documents
- Omit `:ro` if RAGFlow needs to write to the mounted location
- Read-only is recommended for safety and performance

### Permissions

The mounted directories must be readable by the user running inside the Docker container. If you encounter permission issues:

```bash
# Check permissions
ls -la /mnt/nas-paperless

# Adjust if needed (be careful with this)
sudo chmod -R 755 /mnt/nas-paperless
```

### Network Considerations

- Ensure your NAS is accessible from the Docker host
- NAS access will be slower than local storage
- Consider network bandwidth when processing large documents

## Accessing Documents in RAGFlow

Once mounted, documents are available in RAGFlow:

1. **Via UI:**
   - Go to Knowledge Base
   - Click "Upload" → "Local Files"
   - Navigate to `/ragflow/paperless` (or your custom mount path)
   - Select documents to import

2. **Via API:**
   - Use RAGFlow's API to reference the mounted path
   - Example: `/ragflow/paperless/document.pdf`

## Troubleshooting

### Mount Fails

```bash
# Check if NAS is reachable
ping <NAS_IP>

# Test NFS
showmount -e <NAS_IP>

# Test SMB
smbclient -L //<NAS_IP> -U <USERNAME>
```

### Permission Denied

```bash
# Check mount inside container
docker exec -it ragflow-cpu ls -la /ragflow/paperless

# Check SELinux (RHEL/CentOS)
sudo chcon -Rt svirt_sandbox_file_t /mnt/nas-paperless
```

### Files Not Visible

```bash
# Verify mount on host
mount | grep nas-paperless

# Restart RAGFlow
docker compose down && docker compose up -d
```

## Support

For more detailed documentation, see:
- [docs/guides/mount_nas_storage.md](../docs/guides/mount_nas_storage.md) - Full guide
- [docker-compose.example-nas-mount.yml](./docker-compose.example-nas-mount.yml) - Complete example

For issues specific to RAGFlow, check the main documentation at https://ragflow.io/docs/

## Security Recommendations

1. Use read-only mounts when possible (`:ro`)
2. Store credentials in secure files, not in docker-compose.yml
3. Use encryption for SMB3 or encrypted NFS
4. Limit network access to NAS with firewall rules
5. Regularly audit access logs
