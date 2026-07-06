---
sidebar_position: 1
slug: /config_ssl_cert
sidebar_custom_props: {
  categoryIcon: LucideCog
}
---
# Configure SSL certificates

Configure SSL certificates for a RAGFlow instance deployed via Docker.

---

This guide details how to configure SSL certificates for a RAGFlow instance deployed via Docker, using the container name `docker-ragflow-cpu-1` as an example.

## 1. Prepare certificate files

Ensure you have Nginx-formatted certificate files ready:

- **Public Key**: Usually named `fullchain.pem` or `server.crt`.
- **Private Key**: Usually named `privkey.pem` or `server.key`.

If necessary, rename your files to match the standard:

```bash
# Rename bundle to fullchain.pem
cp XXXXX_bundle.pem fullchain.pem
# Rename private key to privkey.pem
cp XXXXX.key privkey.pem
```

## 2. Confirm container status

Verify that your container is running:

```bash
docker ps
```

## 3. Copy certificates to the container

Transfer the files from your host machine to the container's temporary directory:

```bash
docker cp ./fullchain.pem docker-ragflow-cpu-1:/tmp/fullchain.pem
docker cp ./privkey.pem docker-ragflow-cpu-1:/tmp/privkey.pem
```

## 4. Deploy certificates inside the container

Enter the container's interactive terminal:

```bash
docker exec -it docker-ragflow-cpu-1 /bin/bash
```

Once inside, move the files and set appropriate permissions:

```bash
mkdir -p /etc/nginx/ssl
mv /tmp/fullchain.pem /etc/nginx/ssl/
mv /tmp/privkey.pem /etc/nginx/ssl/

# Set permissions: 644 for public key, 600 for private key
chmod 644 /etc/nginx/ssl/fullchain.pem
chmod 600 /etc/nginx/ssl/privkey.pem
```

## 5. Switch Nginx to HTTPS configuration

Replace the default HTTP configuration with the HTTPS template:

1. Navigate to the configuration directory: `cd /etc/nginx/conf.d/`.
2. Back up the original configuration: `mv ragflow.conf ragflow.conf.bak`.
3. Enable the HTTPS template: `cp /etc/nginx/ragflow.https.conf ./ragflow.conf`.

## 6. Edit the HTTPS template

1. Open the configuration file: `vi ragflow.conf`.
2. Ensure `ssl_certificate` and `ssl_certificate_key` paths point to your files in `/etc/nginx/ssl/`.
3. Verify the Nginx syntax: `nginx -t`.

## 7. Apply the configuration

Reload Nginx to apply changes:

```bash
nginx -s reload
```

If the changes do not take effect, exit the container and restart it:

```bash
exit
docker restart docker-ragflow-cpu-1
```

## Configuration persistence

:::tip IMPORTANT
Changes made via `docker cp` and `docker exec` are lost if the container is removed or stopped via `docker-compose down`.
**Recommendation**: After a successful test, store the certificates on the host machine and use `volumes` in your `docker-compose.yaml` to mount the certificates and `ragflow.conf` permanently.
:::