#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#

from abc import ABC
import json
import smtplib
import logging
from email.mime.text import MIMEText
from email.mime.multipart import MIMEMultipart
from email.header import Header
from email.utils import formataddr
from agent.component.base import ComponentBase, ComponentParamBase

class EmailParam(ComponentParamBase):
    """
    Define the Email component parameters.
    """
    def __init__(self):
        super().__init__()
        # Fixed configuration parameters
        self.smtp_server = ""  # SMTP server address
        self.smtp_port = 465  # SMTP port
        self.email = ""  # Sender email
        self.password = ""  # Email authorization code
        self.sender_name = ""  # Sender name

    def check(self):
        # Check required parameters
        self.check_empty(self.smtp_server, "SMTP Server")
        self.check_empty(self.email, "Email")
        self.check_empty(self.password, "Password")
        self.check_empty(self.sender_name, "Sender Name")

class Email(ComponentBase, ABC):
    component_name = "Email"
    
    def _run(self, history, **kwargs):
        # Get upstream component output and parse JSON
        ans = self.get_input()
        content = "".join(ans["content"]) if "content" in ans else ""
        if not content:
            return Email.be_output("No content to send")

        success = False
        try:
            # Parse JSON string passed from upstream
            email_data = json.loads(content)
            
            # Validate required fields
            if "to_email" not in email_data:
                return Email.be_output("Missing required field: to_email")

            # Create email object
            msg = MIMEMultipart('alternative')
            
            # Properly handle sender name encoding
            msg['From'] = formataddr((str(Header(self._param.sender_name,'utf-8')), self._param.email))
            msg['To'] = email_data["to_email"]
            if "cc_email" in email_data and email_data["cc_email"]:
                msg['Cc'] = email_data["cc_email"]
            msg['Subject'] = Header(email_data.get("subject", "No Subject"), 'utf-8').encode()

            # Use content from email_data or default content
            email_content = email_data.get("content", "No content provided")
            # msg.attach(MIMEText(email_content, 'plain', 'utf-8'))
            msg.attach(MIMEText(email_content, 'html', 'utf-8'))

            # Connect to SMTP server and send
            logging.info(f"Connecting to SMTP server {self._param.smtp_server}:{self._param.smtp_port}")
            
            context = smtplib.ssl.create_default_context()
            with smtplib.SMTP_SSL(self._param.smtp_server, self._param.smtp_port, context=context) as server:
                # Login
                logging.info(f"Attempting to login with email: {self._param.email}")
                server.login(self._param.email, self._param.password)
                
                # Get all recipient list
                recipients = [email_data["to_email"]]
                if "cc_email" in email_data and email_data["cc_email"]:
                    recipients.extend(email_data["cc_email"].split(','))
                
                # Send email
                logging.info(f"Sending email to recipients: {recipients}")
                try:
                    server.send_message(msg, self._param.email, recipients)
                    success = True
                except Exception as e:
                    logging.error(f"Error during send_message: {str(e)}")
                    # Try alternative method
                    server.sendmail(self._param.email, recipients, msg.as_string())
                    success = True
                
                try:
                    server.quit()
                except Exception as e:
                    # Ignore errors when closing connection
                    logging.warning(f"Non-fatal error during connection close: {str(e)}")

            if success:
                return Email.be_output("Email sent successfully")

        except json.JSONDecodeError:
            error_msg = "Invalid JSON format in input"
            logging.error(error_msg)
            return Email.be_output(error_msg)
            
        except smtplib.SMTPAuthenticationError:
            error_msg = "SMTP Authentication failed. Please check your email and authorization code."
            logging.error(error_msg)
            return Email.be_output(f"Failed to send email: {error_msg}")
            
        except smtplib.SMTPConnectError:
            error_msg = f"Failed to connect to SMTP server {self._param.smtp_server}:{self._param.smtp_port}"
            logging.error(error_msg)
            return Email.be_output(f"Failed to send email: {error_msg}")
            
        except smtplib.SMTPException as e:
            error_msg = f"SMTP error occurred: {str(e)}"
            logging.error(error_msg)
            return Email.be_output(f"Failed to send email: {error_msg}")
            
        except Exception as e:
            error_msg = f"Unexpected error: {str(e)}"
            logging.error(error_msg)
            return Email.be_output(f"Failed to send email: {error_msg}") 