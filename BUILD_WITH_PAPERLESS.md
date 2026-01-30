# How to Build RAGFlow with Paperless-ngx Integration

## Issue
The Paperless-ngx data source is not visible in the Docker deployment because you're using a pre-built Docker image that doesn't include the latest frontend changes.

## Solution
You need to build the Docker image locally to include the Paperless-ngx UI integration.

## Steps to Build and Run

### 1. Build the Docker Image Locally

Navigate to the RAGFlow repository root and build the image:

```bash
cd /path/to/ragflow
docker build --platform linux/amd64 -f Dockerfile -t infiniflow/ragflow:dev-paperless .
```

This will take several minutes as it:
- Installs dependencies
- Builds the React frontend (includes Paperless-ngx changes)
- Packages everything into the Docker image

### 2. Update docker/.env

Edit `docker/.env` and change the `RAGFLOW_IMAGE` variable to use your locally built image:

```bash
# Change from:
# RAGFLOW_IMAGE=infiniflow/ragflow:v0.23.1

# To:
RAGFLOW_IMAGE=infiniflow/ragflow:dev-paperless
```

### 3. Restart Docker Compose

```bash
cd docker
docker compose down
docker compose up -d
```

### 4. Verify Paperless-ngx is Visible

1. Wait for the services to start (check with `docker compose logs -f ragflow-cpu`)
2. Navigate to the RAGFlow UI in your browser
3. Go to Settings → Data Sources
4. Paperless-ngx should now appear in position 3 (between S3 and Notion)

## Alternative: Quick Development Setup

If you're actively developing, you can also run the frontend in development mode:

### Frontend Development Server

```bash
cd web
npm install
npm run dev
```

This will start the frontend on port 8000 with hot-reload, but you'll still need the backend services running.

## Verification

After rebuilding and restarting, you should see:

1. **Confluence**
2. **S3** 
3. **Paperless-ngx** ✅ (NEW!)
4. **Notion**
5. **Discord**
... (other sources)

## Troubleshooting

### Build Fails
- Make sure you have sufficient disk space (image is ~2GB)
- Check Docker has enough memory allocated (recommended: 8GB+)
- If behind a proxy, add build args (see README.md)

### Image Not Updating
- Run `docker compose down -v` to remove volumes
- Check `.env` file has correct `RAGFLOW_IMAGE` value
- Verify image was built: `docker images | grep ragflow`

### Paperless-ngx Still Not Showing
- Clear browser cache
- Check browser console for errors
- Verify you're running the correct image: `docker inspect ragflow-cpu | grep Image`

## Production Deployment

For production, wait for the official RAGFlow release that includes Paperless-ngx, or build and push your own image to a registry:

```bash
docker build -t your-registry/ragflow:paperless .
docker push your-registry/ragflow:paperless
```

Then update `RAGFLOW_IMAGE` in `.env` to point to your registry image.
