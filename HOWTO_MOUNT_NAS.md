# How to Mount NAS Paperless Documents to RAGFlow

**Quick guide to get your NAS documents accessible in RAGFlow**

## Problem Solved

You have documents stored on a NAS (Network Attached Storage) running Paperless or any other document management system, and you want RAGFlow to access and process these documents without having to copy them.

## The Solution

Mount your NAS storage to the RAGFlow Docker container so it can directly access your documents.

---

## 🎯 Choose Your Path

### Path 1: Easy Automated Setup (5 minutes) ⭐ RECOMMENDED

Use our interactive script that does everything for you:

```bash
cd docker
sudo ./mount_nas_paperless.sh
```

The script will:
- Ask for your NAS details (IP, share path)
- Choose between NFS or SMB/CIFS
- Mount your NAS automatically
- Update docker-compose.yml for you
- Optionally make it persistent (survive reboots)

After the script completes:
```bash
docker compose down
docker compose up -d
```

**That's it! Your documents are now accessible in RAGFlow.**

---

### Path 2: Manual Setup (10 minutes)

**Step 1: Mount your NAS**

For NFS:
```bash
sudo mkdir -p /mnt/nas-paperless
sudo mount -t nfs <YOUR_NAS_IP>:/path/to/paperless /mnt/nas-paperless
```

For SMB/CIFS:
```bash
sudo mkdir -p /mnt/nas-paperless
sudo mount -t cifs //<YOUR_NAS_IP>/paperless /mnt/nas-paperless -o username=USER,password=PASS
```

**Step 2: Edit docker-compose.yml**

Add this line under the `volumes:` section of `ragflow-cpu` or `ragflow-gpu`:
```yaml
- /mnt/nas-paperless:/ragflow/paperless:ro
```

**Step 3: Restart RAGFlow**
```bash
cd docker
docker compose down
docker compose up -d
```

**Step 4: Verify**
```bash
docker exec -it ragflow-cpu ls -la /ragflow/paperless
```

---

## 📚 Where to Find Documents in RAGFlow

After mounting:

1. Open RAGFlow web interface (usually http://localhost:9380)
2. Go to **Knowledge Base**
3. Click **Upload** → **Local Files**
4. Navigate to `/ragflow/paperless` directory
5. Select documents to import

---

## 📖 Documentation Files

We've created comprehensive documentation to help you:

| File | Purpose | When to Use |
|------|---------|-------------|
| [docker/NAS_MOUNT_README.md](docker/NAS_MOUNT_README.md) | Quick reference | First place to start |
| [docs/guides/mount_nas_paperless_tutorial.md](docs/guides/mount_nas_paperless_tutorial.md) | Step-by-step tutorial | Need detailed walkthrough |
| [docs/guides/mount_nas_storage.md](docs/guides/mount_nas_storage.md) | Complete reference | Advanced configurations |
| [docker/docker-compose.example-nas-mount.yml](docker/docker-compose.example-nas-mount.yml) | Configuration examples | Need example configs |
| [docker/mount_nas_paperless.sh](docker/mount_nas_paperless.sh) | Setup script | Automated setup |

---

## 🔧 Common Scenarios

### Scenario 1: Paperless-ngx on Synology NAS

```bash
# Your Paperless is at /volume1/paperless
cd docker
sudo ./mount_nas_paperless.sh

# When asked:
# NAS IP: 192.168.1.100
# Share path: /volume1/paperless
# Protocol: Choose 1 (NFS) or 2 (SMB)
```

### Scenario 2: Network Drive with PDFs

```bash
# Manual mount
sudo mkdir -p /mnt/network-docs
sudo mount -t cifs //fileserver/documents /mnt/network-docs -o username=admin,password=secret

# Add to docker-compose.yml:
# - /mnt/network-docs:/ragflow/documents:ro
```

### Scenario 3: Multiple NAS Sources

```yaml
# In docker-compose.yml, add multiple mounts:
volumes:
  - /mnt/nas1/paperless:/ragflow/paperless:ro
  - /mnt/nas2/archives:/ragflow/archives:ro
  - /mnt/nas3/reports:/ragflow/reports:ro
```

---

## ⚠️ Troubleshooting

### Can't mount NAS

```bash
# Test connectivity first
ping <NAS_IP>

# For NFS, check if share is exported
showmount -e <NAS_IP>

# For SMB, list shares
smbclient -L //<NAS_IP> -U username
```

### Container can't see files

```bash
# Verify mount on host
mount | grep nas

# Check from container
docker exec -it ragflow-cpu ls -la /ragflow/paperless

# If empty, restart Docker
sudo systemctl restart docker
docker compose up -d
```

### Permission denied

```bash
# Check ownership
ls -la /mnt/nas-paperless

# May need to adjust NAS share permissions
```

---

## 🔒 Security Tips

1. **Use read-only mounts** when possible (`:ro` flag)
2. **Store credentials securely** - never in docker-compose.yml directly
3. **Use encrypted protocols** - SMB3 or encrypted NFS when available
4. **Limit network access** - firewall rules for NAS access

---

## ❓ FAQ

**Q: Will this copy all my documents to RAGFlow?**  
A: No! Documents stay on your NAS. RAGFlow just accesses them over the network.

**Q: Can I use this with any NAS brand?**  
A: Yes! Works with Synology, QNAP, TrueNAS, or any NFS/SMB server.

**Q: What if I'm not using Paperless?**  
A: That's fine! This works with any network storage containing documents.

**Q: Is it safe to mount read-only?**  
A: Yes! `:ro` flag prevents RAGFlow from modifying your documents. Recommended.

**Q: Will this survive system reboots?**  
A: Yes, if you add the mount to `/etc/fstab` (script can do this automatically).

**Q: Can RAGFlow process documents directly from NAS?**  
A: Yes! RAGFlow will read and process documents directly from the mounted location.

---

## 🎓 Learn More

- **Paperless-ngx**: https://docs.paperless-ngx.com/
- **RAGFlow Documentation**: https://ragflow.io/docs/
- **Docker Volumes**: https://docs.docker.com/storage/volumes/
- **NFS Guide**: Linux NFS documentation
- **SMB/CIFS Guide**: Samba documentation

---

## ✅ Success Checklist

After setup, you should be able to:

- [ ] Mount point exists on host: `ls /mnt/nas-paperless`
- [ ] Files visible from container: `docker exec -it ragflow-cpu ls /ragflow/paperless`
- [ ] RAGFlow UI shows documents in upload dialog
- [ ] Documents can be imported to knowledge base
- [ ] RAGFlow can process and search the documents

---

**Need Help?**

1. Check [docker/NAS_MOUNT_README.md](docker/NAS_MOUNT_README.md) for quick fixes
2. Read [docs/guides/mount_nas_paperless_tutorial.md](docs/guides/mount_nas_paperless_tutorial.md) for detailed steps
3. See [docs/guides/mount_nas_storage.md](docs/guides/mount_nas_storage.md) for advanced topics
4. Ask in RAGFlow community channels

---

**Last Updated**: February 2026  
**Compatibility**: RAGFlow v0.23.1+, Docker Compose v2+
