# Ragflow - Jurix

## Instalación y Ejecución

1. Construir la imagen Docker:

   ```bash
   docker build -t ragflow-jurix .
   ```

2. Exportar la variable de entorno para usar la imagen:

   ```bash
   export RAGFLOW_IMAGE=ragflow-jurix
   ```

3. Iniciar todos los servicios:
   CPU

   ```bash
   docker compose \
     -f docker/docker-compose.yml \
     up -d
   ```

   NVIDIA

   ```bash
   docker compose \
   -f docker/docker-compose-gpu.yml \
   up -d
   ```

4. Acceder a la aplicación:
   - Interfaz web: http://localhost
   - API: http://localhost:9380
