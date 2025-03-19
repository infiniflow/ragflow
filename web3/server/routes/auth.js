const express = require('express');
const router = express.Router();
const { body, validationResult } = require('express-validator');
const bcrypt = require('bcryptjs');
const jwt = require('jsonwebtoken');
const { pool } = require('../config/db');
const { sendPasswordResetEmail } = require('../utils/email');

// Register User
router.post(
  '/register',
  [
    body('email', 'Please include a valid email').isEmail(),
    body('password', 'Password must be at least 6 characters').isLength({ min: 6 }),
    body('firstName', 'First name is required').not().isEmpty(),
    body('lastName', 'Last name is required').not().isEmpty()
  ],
  async (req, res) => {
    // Validate request data
    const errors = validationResult(req);
    if (!errors.isEmpty()) {
      return res.status(400).json({ errors: errors.array() });
    }

    const { email, password, firstName, lastName } = req.body;

    try {
      // BYPASS DATABASE FOR TESTING
      console.log('Registering test user:', { email, firstName, lastName });
      
      // Generate mock JWT token
      const payload = {
        user: {
          id: 1
        }
      };

      jwt.sign(
        payload,
        process.env.JWT_SECRET || 'fallback_secret_for_testing',
        { expiresIn: '24h' },
        (err, token) => {
          if (err) throw err;
          res.json({ 
            token,
            msg: 'Test user registered successfully',
            user: {
              id: 1,
              email,
              firstName,
              lastName
            }
          });
        }
      );
    } catch (err) {
      console.error(err.message);
      res.status(500).send('Server error');
    }
  }
);

// Login User
router.post(
  '/login',
  [
    body('email', 'Please include a valid email').isEmail(),
    body('password', 'Password is required').exists()
  ],
  async (req, res) => {
    // Validate request data
    const errors = validationResult(req);
    if (!errors.isEmpty()) {
      return res.status(400).json({ errors: errors.array() });
    }

    const { email, password } = req.body;

    try {
      // BYPASS DATABASE FOR TESTING
      console.log('Logging in test user:', email);
      
      // Check if this is our test user
      if (email === 'test@test.com' && password === 'test123') {
        // Generate JWT token
        const payload = {
          user: {
            id: 1
          }
        };

        jwt.sign(
          payload,
          process.env.JWT_SECRET || 'fallback_secret_for_testing',
          { expiresIn: '24h' },
          (err, token) => {
            if (err) throw err;
            res.json({ 
              token,
              user: {
                id: 1,
                email: 'test@test.com',
                firstName: 'Test',
                lastName: 'User'
              }
            });
          }
        );
      } else {
        return res.status(400).json({ msg: 'Invalid Credentials' });
      }
    } catch (err) {
      console.error(err.message);
      res.status(500).send('Server error');
    }
  }
);

// Request Password Reset
router.post(
  '/forgot-password',
  [
    body('email', 'Please include a valid email').isEmail()
  ],
  async (req, res) => {
    // Validate request data
    const errors = validationResult(req);
    if (!errors.isEmpty()) {
      return res.status(400).json({ errors: errors.array() });
    }

    const { email } = req.body;

    try {
      // Check if user exists
      const [users] = await pool.execute(
        'SELECT * FROM users WHERE email = ?',
        [email]
      );

      if (users.length === 0) {
        return res.status(400).json({ msg: 'User not found' });
      }

      const user = users[0];

      // Generate 6-digit PIN
      const pin = Math.floor(100000 + Math.random() * 900000).toString();
      
      // Set expiration time (15 minutes from now)
      const expiresAt = new Date();
      expiresAt.setMinutes(expiresAt.getMinutes() + 15);

      // Delete any existing tokens for this user
      await pool.execute(
        'DELETE FROM password_reset_tokens WHERE user_id = ?',
        [user.id]
      );

      // Store token in database
      await pool.execute(
        'INSERT INTO password_reset_tokens (user_id, token, expires_at) VALUES (?, ?, ?)',
        [user.id, pin, expiresAt]
      );

      // Send email with PIN
      await sendPasswordResetEmail(user.email, pin);

      res.json({ msg: 'Password reset PIN sent to your email' });
    } catch (err) {
      console.error(err.message);
      res.status(500).send('Server error');
    }
  }
);

// Verify PIN and Reset Password
router.post(
  '/reset-password',
  [
    body('email', 'Please include a valid email').isEmail(),
    body('pin', 'PIN is required').isLength({ min: 6, max: 6 }),
    body('password', 'Password must be at least 6 characters').isLength({ min: 6 })
  ],
  async (req, res) => {
    // Validate request data
    const errors = validationResult(req);
    if (!errors.isEmpty()) {
      return res.status(400).json({ errors: errors.array() });
    }

    const { email, pin, password } = req.body;

    try {
      // Check if user exists
      const [users] = await pool.execute(
        'SELECT * FROM users WHERE email = ?',
        [email]
      );

      if (users.length === 0) {
        return res.status(400).json({ msg: 'User not found' });
      }

      const user = users[0];

      // Check if token exists and is valid
      const [tokens] = await pool.execute(
        'SELECT * FROM password_reset_tokens WHERE user_id = ? AND token = ? AND expires_at > NOW()',
        [user.id, pin]
      );

      if (tokens.length === 0) {
        return res.status(400).json({ msg: 'Invalid or expired PIN' });
      }

      // Hash new password
      const salt = await bcrypt.genSalt(10);
      const hashedPassword = await bcrypt.hash(password, salt);

      // Update user password
      await pool.execute(
        'UPDATE users SET password = ? WHERE id = ?',
        [hashedPassword, user.id]
      );

      // Delete used token
      await pool.execute(
        'DELETE FROM password_reset_tokens WHERE user_id = ?',
        [user.id]
      );

      res.json({ msg: 'Password updated successfully' });
    } catch (err) {
      console.error(err.message);
      res.status(500).send('Server error');
    }
  }
);

module.exports = router; 