---
sidebar_position: 20
slug: /migrate_to_single_bucket_mode
---

# Migrate from multi-Bucket to single-bucket mode

By default, RAGFlow creates one bucket per Knowledge Base (dataset) and one bucket per user folder. This can be problematic when:

- Your cloud provider charges per bucket
- Your IAM policy restricts bucket creation
- You want all data organized in a single bucket with directory structure

The **Single Bucket Mode** allows you to configure RAGFlow to use a single bucket with a directory structure instead of multiple buckets.

:::info KUDOS
This document is contributed by our community contributor [arogan178](https://github.com/arogan178). We may not actively maintain this document.
:::

## How It Works

### Default Mode (Multiple Buckets)

```
bucket: kb_12345/
  └── document_1.pdf
bucket: kb_67890/
  └── document_2.pdf
bucket: folder_abc/
  └── file_3.txt
```

### Single Bucket Mode (with prefix_path)

```
bucket: ragflow-bucket/
  └── ragflow/
      ├── kb_12345/
      │   └── document_1.pdf
      ├── kb_67890/
      │   └── document_2.pdf
      └── folder_abc/
          └── file_3.txt
```

## Configuration

### MinIO Configuration

Edit your `service_conf.yaml` or set environment variables:

```yaml
minio:
  user: "your-access-key"
  password: "your-secret-key"
  host: "minio.example.com:443"
  bucket: "ragflow-bucket" # Default bucket name
  prefix_path: "ragflow" # Optional prefix path
```

Or using environment variables:

```bash
export MINIO_USER=your-access-key
export MINIO_PASSWORD=your-secret-key
export MINIO_HOST=minio.example.com:443
export MINIO_BUCKET=ragflow-bucket
export MINIO_PREFIX_PATH=ragflow
```

### S3 Configuration (already supported)

```yaml
s3:
  access_key: "your-access-key"
  secret_key: "your-secret-key"
  endpoint_url: "https://s3.amazonaws.com"
  bucket: "my-ragflow-bucket"
  prefix_path: "production"
  region: "us-east-1"
```

## IAM Policy Example

When using single bucket mode, you only need permissions for one bucket:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["s3:*"],
      "Resource": [
        "arn:aws:s3:::ragflow-bucket",
        "arn:aws:s3:::ragflow-bucket/*"
      ]
    }
  ]
}
```

## Migration from Multi-Bucket to Single Bucket

If you're migrating from multi-bucket mode to single-bucket mode:

1. **Set environment variables** for the new configuration
2. **Restart RAGFlow** services
3. **Migrate existing data** (optional):

```bash
# Example using mc (MinIO Client)
mc alias set old-minio http://old-minio:9000 ACCESS_KEY SECRET_KEY
mc alias set new-minio https://new-minio:443 ACCESS_KEY SECRET_KEY

# List all knowledge base buckets
mc ls old-minio/ | grep kb_ | while read -r line; do
    bucket=$(echo $line | awk '{print $5}')
    # Copy each bucket to the new structure
    mc cp --recursive old-minio/$bucket/ new-minio/ragflow-bucket/ragflow/$bucket/
done
```

## Toggle Between Modes

### Enable Single Bucket Mode

```yaml
minio:
  bucket: "my-single-bucket"
  prefix_path: "ragflow"
```

### Disable (Use Multi-Bucket Mode)

```yaml
minio:
  # Leave bucket and prefix_path empty or commented out
  # bucket: ''
  # prefix_path: ''
```

## Troubleshooting

### Issue: Access Denied errors

**Solution**: Ensure your IAM policy grants access to the bucket specified in the configuration.

### Issue: Files not found after switching modes

**Solution**: The path structure changes between modes. You'll need to migrate existing data.

### Issue: Connection fails with HTTPS

**Solution**: Ensure `secure: True` is set in the MinIO connection (automatically handled for port 443).

## Storage Backends Supported

- ✅ **MinIO** - Full support with single bucket mode
- ✅ **AWS S3** - Full support with single bucket mode
- ✅ **Alibaba OSS** - Full support with single bucket mode
- ✅ **Azure Blob** - Uses container-based structure (different paradigm)
- ⚠️ **OpenDAL** - Depends on underlying storage backend

## Performance Considerations

- **Single bucket mode** may have slightly better performance for bucket listing operations
- **Multi-bucket mode** provides better isolation and organization for large deployments
- Choose based on your specific requirements and infrastructure constraints
