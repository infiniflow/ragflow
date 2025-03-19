# Web3 Application

This is a React + Express application with authentication and MySQL database.

## Getting Started with Docker

### Prerequisites

- [Docker](https://docs.docker.com/get-docker/)
- [Docker Compose](https://docs.docker.com/compose/install/)

### Running the Application

1. Clone this repository
2. Navigate to the project directory:
   ```
   cd web3
   ```
3. Copy the example environment file:
   ```
   cp .env.example .env
   ```
4. Edit the `.env` file to update any environment variables (especially the JWT_SECRET)
5. Build and start the containers:
   ```
   docker-compose up -d
   ```
6. The application will be available at http://localhost:5001

### Default Test User

The application comes with a test user:
- Email: test@test.com
- Password: test123

### Development

For development, you can mount your local directories using volumes in the docker-compose.yml file.

To rebuild the application after changes:
```
docker-compose up -d --build
```

### Stopping the Application

```
docker-compose down
```

To stop and remove volumes (this will delete the database data):
```
docker-compose down -v
```

## Authentication Features

- User registration and login
- JWT token-based authentication
- Password reset via email (requires configuring EMAIL_* environment variables)
- Protected routes on the frontend

## Project Structure

- `server/` - Express backend API
  - `config/` - Database and configuration files
  - `routes/` - API routes
  - `utils/` - Utility functions
- `src/` - React frontend
  - `components/` - Reusable UI components
  - `contexts/` - React contexts for state management
  - `layouts/` - Layout components
  - `pages/` - Page components
  - `services/` - API services 