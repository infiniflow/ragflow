#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
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

import base64
import ipaddress
import json
import re
import socket
from urllib.parse import urlparse
import aiosmtplib
from email.mime.text import MIMEText
from email.header import Header
from common import settings
from quart import render_template_string
from api.utils.email_templates import EMAIL_TEMPLATES
from selenium import webdriver
from selenium.common.exceptions import TimeoutException
from selenium.webdriver.chrome.options import Options
from selenium.webdriver.chrome.service import Service
from selenium.webdriver.common.by import By
from selenium.webdriver.support.expected_conditions import staleness_of
from selenium.webdriver.support.ui import WebDriverWait
from webdriver_manager.chrome import ChromeDriverManager


OTP_LENGTH = 4
OTP_TTL_SECONDS = 5 * 60 # valid for 5 minutes
ATTEMPT_LIMIT = 5 # maximum attempts
ATTEMPT_LOCK_SECONDS = 30 * 60 # lock for 30 minutes
RESEND_COOLDOWN_SECONDS = 60 # cooldown for 1 minute


CONTENT_TYPE_MAP = {
    # Office
    "docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
    "doc": "application/msword",
    "pdf": "application/pdf",
    "csv": "text/csv",
    "xls": "application/vnd.ms-excel",
    "xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
    # Text/code
    "txt": "text/plain",
    "py": "text/plain",
    "js": "text/plain",
    "java": "text/plain",
    "c": "text/plain",
    "cpp": "text/plain",
    "h": "text/plain",
    "php": "text/plain",
    "go": "text/plain",
    "ts": "text/plain",
    "sh": "text/plain",
    "cs": "text/plain",
    "kt": "text/plain",
    "sql": "text/plain",
    # Web
    "md": "text/markdown",
    "markdown": "text/markdown",
    "mdx": "text/markdown",
    "htm": "text/html",
    "html": "text/html",
    "json": "application/json",
    # Image formats
    "png": "image/png",
    "jpg": "image/jpeg",
    "jpeg": "image/jpeg",
    "gif": "image/gif",
    "bmp": "image/bmp",
    "tiff": "image/tiff",
    "tif": "image/tiff",
    "webp": "image/webp",
    "svg": "image/svg+xml",
    "ico": "image/x-icon",
    "avif": "image/avif",
    "heic": "image/heic",
    # PPTX
    "ppt": "application/vnd.ms-powerpoint",
    "pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
}


def html2pdf(
    source: str,
    timeout: int = 2,
    install_driver: bool = True,
    print_options: dict = {},
):
    result = __get_pdf_from_html(source, timeout, install_driver, print_options)
    return result


def __send_devtools(driver, cmd, params={}):
    resource = "/session/%s/chromium/send_command_and_get_result" % driver.session_id
    url = driver.command_executor._url + resource
    body = json.dumps({"cmd": cmd, "params": params})
    response = driver.command_executor._request("POST", url, body)

    if not response:
        raise Exception(response.get("value"))

    return response.get("value")


def __get_pdf_from_html(path: str, timeout: int, install_driver: bool, print_options: dict):
    webdriver_options = Options()
    webdriver_prefs = {}
    webdriver_options.add_argument("--headless")
    webdriver_options.add_argument("--disable-gpu")
    webdriver_options.add_argument("--no-sandbox")
    webdriver_options.add_argument("--disable-dev-shm-usage")
    webdriver_options.experimental_options["prefs"] = webdriver_prefs

    webdriver_prefs["profile.default_content_settings"] = {"images": 2}

    if install_driver:
        service = Service(ChromeDriverManager().install())
        driver = webdriver.Chrome(service=service, options=webdriver_options)
    else:
        driver = webdriver.Chrome(options=webdriver_options)

    driver.get(path)

    try:
        WebDriverWait(driver, timeout).until(staleness_of(driver.find_element(by=By.TAG_NAME, value="html")))
    except TimeoutException:
        calculated_print_options = {
            "landscape": False,
            "displayHeaderFooter": False,
            "printBackground": True,
            "preferCSSPageSize": True,
        }
        calculated_print_options.update(print_options)
        result = __send_devtools(driver, "Page.printToPDF", calculated_print_options)
        driver.quit()
        return base64.b64decode(result["data"])


def is_private_ip(ip: str) -> bool:
    try:
        ip_obj = ipaddress.ip_address(ip)
        return ip_obj.is_private
    except ValueError:
        return False


def is_valid_url(url: str) -> bool:
    if not re.match(r"(https?)://[-A-Za-z0-9+&@#/%?=~_|!:,.;]+[-A-Za-z0-9+&@#/%=~_|]", url):
        return False
    parsed_url = urlparse(url)
    hostname = parsed_url.hostname

    if not hostname:
        return False
    try:
        ip = socket.gethostbyname(hostname)
        if is_private_ip(ip):
            return False
    except socket.gaierror:
        return False
    return True


def safe_json_parse(data: str | dict) -> dict:
    if isinstance(data, dict):
        return data
    try:
        return json.loads(data) if data else {}
    except (json.JSONDecodeError, TypeError):
        return {}


def get_float(req: dict, key: str, default: float | int = 10.0) -> float:
    try:
        parsed = float(req.get(key, default))
        return parsed if parsed > 0 else default
    except (TypeError, ValueError):
        return default
    

async def send_email_html(to_email: str, subject: str, template_key: str, **context):

    body = await render_template_string(EMAIL_TEMPLATES.get(template_key), **context)
    msg = MIMEText(body, "plain", "utf-8")
    msg["Subject"] = Header(subject, "utf-8")
    msg["From"] = f"{settings.MAIL_DEFAULT_SENDER[0]} <{settings.MAIL_DEFAULT_SENDER[1]}>"
    msg["To"] = to_email

    smtp = aiosmtplib.SMTP(
        hostname=settings.MAIL_SERVER,
        port=settings.MAIL_PORT,
        use_tls=True,
        timeout=10,
    )

    await smtp.connect()
    await smtp.login(settings.MAIL_USERNAME, settings.MAIL_PASSWORD)
    await smtp.send_message(msg)
    await smtp.quit()


async def send_invite_email(to_email, invite_url, tenant_id, inviter):
    # Reuse the generic HTML sender with 'invite' template
    await send_email_html(
        to_email=to_email,
        subject="RAGFlow Invitation",
        template_key="invite",
        email=to_email,
        invite_url=invite_url,
        tenant_id=tenant_id,
        inviter=inviter,
    )


def otp_keys(email: str):
    email = (email or "").strip().lower()
    return (
        f"otp:{email}",
        f"otp_attempts:{email}",
        f"otp_last_sent:{email}",
        f"otp_lock:{email}",
    )


def hash_code(code: str, salt: bytes) -> str:
    import hashlib
    import hmac 
    return hmac.new(salt, (code or "").encode("utf-8"), hashlib.sha256).hexdigest()
    

def captcha_key(email: str) -> str:
    return f"captcha:{email}"
    
