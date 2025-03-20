/**
 * Main Server Entry Point
 * This file initializes and configures the Express server, establishes routes,
 * connects to the database, and starts the server listening on the configured port.
 */

const express = require('express');
const cors = require('cors');
const { initializeDb } = require('./config/db');
const authRoutes = require('./routes/auth');

// Load environment variables from .env file
require('dotenv').config();

// Initialize Express application
const app = express();
const PORT = process.env.PORT || 5001;

// Middleware Configuration
// Enable Cross-Origin Resource Sharing (CORS) for all routes
app.use(cors());
// Parse incoming JSON request bodies
app.use(express.json());

// Initialize Database Connection and Schema
// Creates required tables if they don't exist
initializeDb();

// Route Registration
// Mount authentication routes under /api/auth prefix
app.use('/api/auth', authRoutes);

// Test Route - Simple health check endpoint
app.get('/', (req, res) => {
  res.send('API is running...');
});

// Start Server
app.listen(PORT, () => {
  console.log(`Server running on port ${PORT}`);
});

module.exports = app; // Export for testing 