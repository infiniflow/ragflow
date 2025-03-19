const nodemailer = require('nodemailer');
require('dotenv').config();

// Configure nodemailer
const transporter = nodemailer.createTransport({
  service: process.env.EMAIL_SERVICE,
  auth: {
    user: process.env.EMAIL_USER,
    pass: process.env.EMAIL_PASSWORD
  }
});

/**
 * Send password reset email with PIN
 * @param {string} to - Recipient email
 * @param {string} pin - 6-digit PIN for password reset
 * @returns {Promise} - Nodemailer result
 */
async function sendPasswordResetEmail(to, pin) {
  const mailOptions = {
    from: process.env.EMAIL_FROM,
    to,
    subject: 'Logen AI - Password Reset',
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
    const info = await transporter.sendMail(mailOptions);
    console.log('Email sent: %s', info.messageId);
    return info;
  } catch (error) {
    console.error('Error sending email:', error);
    throw error;
  }
}

module.exports = {
  sendPasswordResetEmail
}; 