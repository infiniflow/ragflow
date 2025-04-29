# Ragflow - Jurix


## Instalación y Ejecución


3. Construir la imagen Docker:

   ```bash
   docker build -t ragflow-jurix .
   ```

4. Exportar la variable de entorno para usar la imagen:

   ```bash
   export RAGFLOW_IMAGE=ragflow-jurix
   ```

5. Iniciar todos los servicios:

   ```bash
   docker compose \
     -f docker/docker-compose-base.yml \
     -f docker/docker-compose.yml \
     up -d
   ```

6. Acceder a la aplicación:
   - Interfaz web: http://localhost
   - API: http://localhost:9380

