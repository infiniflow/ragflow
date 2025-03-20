/**
 * JWT Authentication Middleware
 * 
 * This middleware validates JWT tokens for protected routes.
 * It extracts the token from the request header, verifies it,
 * and attaches the user data to the request object for use in route handlers.
 * 
 * Validation process:
 * 1. Extract token from 'x-auth-token' header
 * 2. Verify token is present
 * 3. Decode and verify token using JWT_SECRET
 * 4. Attach decoded user data to request
 * 5. Allow request to proceed if token is valid
 * 
 * Error handling:
 * - 401 response if token is missing
 * - 401 response if token is invalid or expired
 */
const jwt = require('jsonwebtoken');
require('dotenv').config();

/**
 * Authentication Middleware Function
 * 
 * @param {Object} req - Express request object
 * @param {Object} res - Express response object
 * @param {Function} next - Express next middleware function
 * @returns {void} - Calls next() if authentication succeeds, otherwise returns error response
 */
module.exports = function(req, res, next) {
  // Extract JWT token from the request header
  // The client must include this header in protected route requests
  const token = req.header('x-auth-token');

  // If no token is provided, return 401 Unauthorized status
  // This prevents unauthenticated users from accessing protected routes
  if (!token) {
    return res.status(401).json({ msg: 'No token, authorization denied' });
  }

  try {
    // Verify and decode the JWT token using the secret key
    // This validates that the token was issued by our system and hasn't been tampered with
    const decoded = jwt.verify(token, process.env.JWT_SECRET);
    
    // Attach the decoded user data to the request object
    // This makes user information available to subsequent route handlers
    req.user = decoded.user;
    
    // Proceed to the next middleware or route handler
    next();
  } catch (err) {
    // If token verification fails (expired, invalid signature, etc.)
    // Return 401 Unauthorized status
    res.status(401).json({ msg: 'Token is not valid' });
  }
}; 