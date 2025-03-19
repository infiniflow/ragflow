# Base Node.js image
FROM node:20-alpine as base

# Create app directory
WORKDIR /app

# Environment variables
ENV NODE_ENV=production
ENV PORT=5001

# Install dependencies
FROM base as dependencies
COPY package*.json ./
RUN npm ci

# Build frontend
FROM dependencies as build-frontend
COPY . .
RUN npm run build

# Final production image
FROM base as production

# Install production dependencies only
COPY package*.json ./
RUN npm ci --only=production

# Install runtime dependencies for authentication
RUN apk add --no-cache \
    # For bcrypt compilation
    python3 \
    make \
    g++ \
    # For MySQL
    mysql-client

# Copy built frontend assets
COPY --from=build-frontend /app/dist ./dist
# Copy server files
COPY --from=build-frontend /app/server ./server
# Copy .env file template (will be configured at runtime)
COPY --from=build-frontend /app/.env.example ./.env.example
# Copy public assets if any
COPY --from=build-frontend /app/public ./public

# Create volume for MySQL data
VOLUME /app/data

# Expose port
EXPOSE 5001

# Create a healthcheck
HEALTHCHECK --interval=30s --timeout=30s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:5001/ || exit 1

# Start the application
CMD ["node", "server/index.js"] 