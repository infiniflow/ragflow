/**
 * Email Utility Module
 * 
 * This module provides email functionality for the application,
 * specifically for sending password reset emails with verification PINs.
 * 
 * It utilizes Nodemailer to configure and send emails through the
 * email service defined in environment variables.
 */

const nodemailer = require('nodemailer');
require('dotenv').config();

/**
 * Nodemailer Transporter
 * 
 * Configures the email transport mechanism using environment variables:
 * - EMAIL_SERVICE: The email service provider (e.g., 'gmail', 'outlook')
 * - EMAIL_USER: The sender's email address
 * - EMAIL_PASSWORD: The sender's email password or app-specific password
 * 
 * For Gmail, you typically need to use an app-specific password
 * rather than your account password due to security restrictions.
 */
const transporter = nodemailer.createTransport({
  service: process.env.EMAIL_SERVICE,
  auth: {
    user: process.env.EMAIL_USER,
    pass: process.env.EMAIL_PASSWORD
  }
});

/**
 * Send Password Reset Email
 * 
 * Sends an email containing a 6-digit PIN for password reset verification.
 * The email includes instructions and formatting for a professional appearance.
 * 
 * @param {string} to - Recipient's email address
 * @param {string} pin - 6-digit PIN for password reset verification
 * @returns {Promise<object>} - Resolves with Nodemailer info object on success
 * @throws {Error} - Throws if email sending fails
 */
async function sendPasswordResetEmail(to, pin) {
  // Define email content and options
  const mailOptions = {
    from: process.env.EMAIL_FROM, // Sender address from environment variables
    to, // Recipient address
    subject: 'Logen AI - Password Reset',
    // HTML email body with styling for better presentation
    html: `
      <div style="font-family: Arial, sans-serif; max-width: 600px; margin: 0 auto;">
        <h2 style="color: #4A5568;">Password Reset Request</h2>
        <p>We received a request to reset your password for your Logen AI account.</p>
        <p>Your password reset PIN is:</p>
        <div style="background-color: #EDF2F7; padding: 12px; font-size: 24px; font-weight: bold; letter-spacing: 2px; text-align: center; margin: 16px 0;">
          ${pin}
        </div>
        <p>This PIN will expire in 15 minutes.</p>
        <p>If you did not request a password reset, please ignore this email or contact support if you have concerns.</p>
        <p style="margin-top: 24px;">Regards,<br>The Logen AI Team</p>
      </div>
    `
  };

  try {
    // Attempt to send the email using the configured transporter
    const info = await transporter.sendMail(mailOptions);
    // Log success information
    console.log('Email sent: %s', info.messageId);
    return info;
  } catch (error) {
    // Log detailed error information for debugging
    console.error('Error sending email:', error);
    // Re-throw the error for handling by the calling function
    throw error;
  }
}

// Export functions for use in other modules
module.exports = {
  sendPasswordResetEmail
}; 