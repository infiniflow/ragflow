# Step-by-Step Tutorial: Mounting NAS Paperless Documents to RAGFlow

This tutorial walks you through the complete process of mounting your NAS Paperless documents to RAGFlow.

## Prerequisites

- RAGFlow already installed and running
- A NAS device with Paperless documents (or any document storage)
- Network connectivity between your Docker host and NAS
- Root/sudo access on your Docker host

## Tutorial Path

Choose your preferred method:
- **[Path A: Automated Setup](#path-a-automated-setup)** - Easiest, uses the helper script (5 minutes)
- **[Path B: Manual Setup](#path-b-manual-setup)** - More control, step-by-step (10 minutes)

---

## Path A: Automated Setup

### Step 1: Download and Run the Helper Script

```bash
cd /path/to/ragflow/docker
sudo ./mount_nas_paperless.sh
```

### Step 2: Follow the Interactive Prompts

The script will ask you:

1. **Protocol selection:**
   ```
   Select your NAS protocol:
   1) NFS
   2) SMB/CIFS
   Enter choice [1-2]: 1
   ```

2. **NAS details:**
   ```
   Enter NAS IP address: 192.168.1.100
   Enter NAS share path: /volume1/paperless
   Enter local mount point [/mnt/nas-paperless]: <press Enter>
   ```

3. **For SMB/CIFS only - credentials:**
   ```
   Enter username: your_username
   Enter password: ********
   ```

4. **Persistence:**
   ```
   Would you like to make this mount persistent (add to /etc/fstab)? [y/N]: y
   ```

5. **Docker configuration:**
   ```
   Would you like to automatically add this volume to docker-compose.yml? [y/N]: y
   ```

### Step 3: Restart RAGFlow

```bash
cd /path/to/ragflow/docker
docker compose down
docker compose up -d
```

### Step 4: Verify Access

```bash
# Check mount on host
ls -la /mnt/nas-paperless

# Check access from container
docker exec -it ragflow-cpu ls -la /ragflow/paperless
```

### Step 5: Use in RAGFlow

1. Open RAGFlow web interface (http://localhost or your configured address)
2. Navigate to **Knowledge Base**
3. Create or select a knowledge base
4. Click **Upload** → **Local Files**
5. Navigate to `/ragflow/paperless` directory
6. Select documents to import

**Done!** Your NAS documents are now accessible in RAGFlow.

---

## Path B: Manual Setup

### Step 1: Install Required Tools

**For NFS:**
```bash
# Ubuntu/Debian
sudo apt-get update
sudo apt-get install -y nfs-common

# RHEL/CentOS/Fedora
sudo yum install -y nfs-utils
```

**For SMB/CIFS:**
```bash
# Ubuntu/Debian
sudo apt-get update
sudo apt-get install -y cifs-utils

# RHEL/CentOS/Fedora
sudo yum install -y cifs-utils
```

### Step 2: Test NAS Connectivity

**For NFS:**
```bash
# Test if NAS is reachable
showmount -e 192.168.1.100

# Expected output shows available shares:
# Export list for 192.168.1.100:
# /volume1/paperless *
```

**For SMB/CIFS:**
```bash
# List available shares
smbclient -L //192.168.1.100 -U your_username

# Enter password when prompted
```

### Step 3: Create Mount Point

```bash
sudo mkdir -p /mnt/nas-paperless
```

### Step 4: Mount the NAS

**For NFS:**
```bash
sudo mount -t nfs 192.168.1.100:/volume1/paperless /mnt/nas-paperless

# Verify mount
mount | grep nas-paperless
ls -la /mnt/nas-paperless
```

**For SMB/CIFS:**
```bash
# Create credentials file (more secure)
sudo nano /root/.smbcredentials

# Add these lines:
username=your_username
password=your_password

# Save and exit (Ctrl+X, Y, Enter)

# Secure the file
sudo chmod 600 /root/.smbcredentials

# Mount using credentials file
sudo mount -t cifs //192.168.1.100/paperless /mnt/nas-paperless -o credentials=/root/.smbcredentials

# Verify mount
mount | grep nas-paperless
ls -la /mnt/nas-paperless
```

### Step 5: Make Mount Persistent (Optional but Recommended)

Add to `/etc/fstab` to automatically mount on system boot:

```bash
sudo nano /etc/fstab
```

**For NFS, add this line:**
```
192.168.1.100:/volume1/paperless  /mnt/nas-paperless  nfs  defaults,_netdev  0  0
```

**For SMB/CIFS, add this line:**
```
//192.168.1.100/paperless  /mnt/nas-paperless  cifs  credentials=/root/.smbcredentials,_netdev  0  0
```

Save and exit (Ctrl+X, Y, Enter)

**Test the fstab entry:**
```bash
# Unmount first
sudo umount /mnt/nas-paperless

# Mount using fstab
sudo mount -a

# Verify
mount | grep nas-paperless
```

### Step 6: Update docker-compose.yml

Open your docker-compose.yml file:

```bash
cd /path/to/ragflow/docker
nano docker-compose.yml
```

Find the `ragflow-cpu` (or `ragflow-gpu`) service, and add the NAS mount under `volumes:`:

**Before:**
```yaml
services:
  ragflow-cpu:
    # ... other configuration ...
    volumes:
      - ./ragflow-logs:/ragflow/logs
      - ./nginx/ragflow.conf:/etc/nginx/conf.d/ragflow.conf
      - ./nginx/proxy.conf:/etc/nginx/proxy.conf
      - ./nginx/nginx.conf:/etc/nginx/nginx.conf
      - ./service_conf.yaml.template:/ragflow/conf/service_conf.yaml.template
      - ./entrypoint.sh:/ragflow/entrypoint.sh
```

**After:**
```yaml
services:
  ragflow-cpu:
    # ... other configuration ...
    volumes:
      - ./ragflow-logs:/ragflow/logs
      - ./nginx/ragflow.conf:/etc/nginx/conf.d/ragflow.conf
      - ./nginx/proxy.conf:/etc/nginx/proxy.conf
      - ./nginx/nginx.conf:/etc/nginx/nginx.conf
      - ./service_conf.yaml.template:/ragflow/conf/service_conf.yaml.template
      - ./entrypoint.sh:/ragflow/entrypoint.sh
      # NAS Paperless documents mount (read-only)
      - /mnt/nas-paperless:/ragflow/paperless:ro
```

**Note:** The `:ro` flag makes it read-only. Remove `:ro` if RAGFlow needs write access.

Save and exit (Ctrl+X, Y, Enter)

### Step 7: Restart RAGFlow

```bash
cd /path/to/ragflow/docker
docker compose down
docker compose up -d
```

Wait for all containers to start (about 30-60 seconds).

### Step 8: Verify Everything Works

**Check mount on host:**
```bash
ls -la /mnt/nas-paperless
```

**Check access from container:**
```bash
docker exec -it ragflow-cpu ls -la /ragflow/paperless
```

**Check container logs (if issues):**
```bash
docker logs ragflow-cpu
```

### Step 9: Use in RAGFlow

1. Open RAGFlow web interface (http://localhost:9380 or your configured port)
2. Log in to RAGFlow
3. Navigate to **Knowledge Base**
4. Click **Create Knowledge Base** or select an existing one
5. Click **Upload** button
6. Select **Local Files**
7. Navigate to `/ragflow/paperless` directory
8. Select documents to import
9. Configure parsing settings as needed
10. Click **Upload** or **Start Parsing**

**Your documents are now being processed by RAGFlow!**

---

## Common Paperless-ngx Directory Structures

If you're using Paperless-ngx, here are common directory layouts and what to mount:

### Option 1: Mount Processed Documents Only
```yaml
volumes:
  - /mnt/nas-paperless/media/documents:/ragflow/paperless:ro
```
This gives you access to all organized, processed documents.

### Option 2: Mount Multiple Directories
```yaml
volumes:
  - /mnt/nas-paperless/media/documents:/ragflow/paperless/media:ro
  - /mnt/nas-paperless/archive:/ragflow/paperless/archive:ro
```
Access both processed documents and original files.

### Option 3: Mount Everything
```yaml
volumes:
  - /mnt/nas-paperless:/ragflow/paperless:ro
```
Full access to entire Paperless directory structure.

---

## Troubleshooting

### Problem: Mount Permission Denied

**Solution:**
```bash
# Check permissions on NAS share
ls -la /mnt/nas-paperless

# If needed, check NFS exports on NAS (on NAS device):
exportfs -v

# For SMB, verify credentials
testparm -s
```

### Problem: Container Can't See Files

**Solution:**
```bash
# Check if mount exists on host
mount | grep nas-paperless

# Check from inside container
docker exec -it ragflow-cpu ls -la /ragflow/paperless

# If empty, check Docker volume definition in docker-compose.yml
# Restart Docker daemon if needed
sudo systemctl restart docker
```

### Problem: "Stale file handle" Error

**Solution:**
```bash
# Unmount and remount
sudo umount -l /mnt/nas-paperless
sudo mount -a

# Or force unmount if stuck
sudo umount -f /mnt/nas-paperless
sudo mount -t nfs 192.168.1.100:/volume1/paperless /mnt/nas-paperless
```

### Problem: Slow Performance

**Solutions:**
```bash
# For NFS, try these mount options:
sudo mount -t nfs -o rw,async,hard,intr,rsize=8192,wsize=8192 \
  192.168.1.100:/volume1/paperless /mnt/nas-paperless

# For SMB, try:
sudo mount -t cifs //192.168.1.100/paperless /mnt/nas-paperless \
  -o credentials=/root/.smbcredentials,vers=3.0,cache=strict
```

### Problem: SELinux Blocking Access (RHEL/CentOS)

**Solution:**
```bash
# Check SELinux status
getenforce

# Fix context
sudo chcon -Rt svirt_sandbox_file_t /mnt/nas-paperless

# Or disable SELinux for Docker (not recommended for production)
sudo setenforce 0
```

---

## Next Steps

After successfully mounting your NAS:

1. **Import Documents**: Start importing documents to your knowledge base
2. **Configure Parsing**: Adjust parsing settings based on document types
3. **Create Assistants**: Build AI assistants using your document knowledge
4. **Test Retrieval**: Query your documents to verify accuracy
5. **Monitor Storage**: Keep an eye on RAGFlow storage usage

## Getting Help

- **Full Documentation**: [docs/guides/mount_nas_storage.md](../docs/guides/mount_nas_storage.md)
- **RAGFlow Docs**: https://ragflow.io/docs/
- **GitHub Issues**: https://github.com/infiniflow/ragflow/issues

---

**Congratulations!** You've successfully mounted your NAS Paperless documents to RAGFlow. Your documents are now ready to be processed and queried using RAGFlow's powerful RAG capabilities.
