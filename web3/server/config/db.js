/**
 * Database Configuration Module
 * 
 * This module handles the MySQL database connection and schema initialization.
 * It creates a connection pool for efficient database operations and
 * provides functions to initialize the database schema.
 */

const mysql = require('mysql2/promise');
require('dotenv').config();

/**
 * MySQL Connection Pool
 * 
 * Creates a pool of connections to the MySQL database using configuration from environment variables.
 * Using a connection pool improves performance by reusing connections rather than creating new ones.
 * 
 * Configuration:
 * - host: Database server hostname (from DB_HOST env or defaults to 'localhost')
 * - user: Database username (from DB_USER env or defaults to 'root')
 * - password: Database password (from DB_PASSWORD env or defaults to 'password')
 * - database: Database name (from DB_NAME env or defaults to 'logen_db')
 * - waitForConnections: Whether to wait for connections when the pool is full
 * - connectionLimit: Maximum number of connections in the pool
 * - queueLimit: Maximum number of connection requests to queue
 */
const pool = mysql.createPool({
  host: process.env.DB_HOST || 'localhost',
  user: process.env.DB_USER || 'root',
  password: process.env.DB_PASSWORD || 'password',
  database: process.env.DB_NAME || 'logen_db',
  waitForConnections: true,
  connectionLimit: 10,
  queueLimit: 0,
  connectTimeout: 30000, // Increased timeout for connections
  trace: true, // Stack traces for debugging
  multipleStatements: true // Allow multiple statements in one query
});

/**
 * Test Database Connection
 * 
 * Tests the database connection and logs the connection status.
 * 
 * @returns {Promise<boolean>} - Resolves with true if connection is successful
 */
async function testConnection() {
  try {
    const conn = await pool.getConnection();
    console.log('Database connection successful');
    conn.release();
    return true;
  } catch (error) {
    console.error('Database connection failed:', error.message);
    console.error('Please check your database configuration and ensure the MySQL server is running.');
    return false;
  }
}

/**
 * Initialize Database Schema
 * 
 * Creates the necessary database tables if they don't already exist.
 * This function is called when the server starts up.
 * 
 * Tables created:
 * 1. users - Stores user account information
 * 2. password_reset_tokens - Stores tokens for password reset functionality
 * 
 * @returns {Promise<void>}
 */
async function initializeDb() {
  try {
    // First test the connection
    const connected = await testConnection();
    if (!connected) {
      console.log('Skipping database initialization due to connection issues');
      return;
    }

    // Create users table if it doesn't exist
    // This table stores user authentication and profile information
    // Fields:
    // - id: Unique identifier (auto-incremented)
    // - email: User's email address (must be unique)
    // - password: Bcrypt-hashed password
    // - first_name, last_name: User's name
    // - created_at, updated_at: Timestamps for record creation and updates
    await pool.execute(`
      CREATE TABLE IF NOT EXISTS users (
        id INT AUTO_INCREMENT PRIMARY KEY,
        email VARCHAR(255) NOT NULL UNIQUE,
        password VARCHAR(255) NOT NULL,
        first_name VARCHAR(100),
        last_name VARCHAR(100),
        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
        updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
      )
    `);

    // Create password_reset_tokens table if it doesn't exist
    // This table stores temporary tokens for password reset functionality
    // Fields:
    // - id: Unique identifier (auto-incremented)
    // - user_id: Foreign key to users table (cascade deletes)
    // - token: 6-digit PIN for password reset verification
    // - expires_at: Token expiration timestamp
    // - created_at: Token creation timestamp
    await pool.execute(`
      CREATE TABLE IF NOT EXISTS password_reset_tokens (
        id INT AUTO_INCREMENT PRIMARY KEY,
        user_id INT NOT NULL,
        token VARCHAR(6) NOT NULL,
        expires_at TIMESTAMP NOT NULL,
        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
      )
    `);

    console.log('Database initialized successfully');
  } catch (error) {
    console.error('Database initialization error:', error);
    console.error('The server will continue to run, but database functionality will be limited.');
  }
}

// Export the connection pool and initialization function
module.exports = {
  pool,
  initializeDb,
  testConnection
}; 