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
    定义邮件组件参数。
    """
    def __init__(self):
        super().__init__()
        # 固定配置参数
        self.smtp_server = ""  # SMTP服务器地址
        self.smtp_port = 465  # SMTP端口
        self.email = ""  # 发件人邮箱
        self.password = ""  # 邮箱授权码
        self.sender_name = ""  # 发件人名称

    def check(self):
        # 检查必填参数
        self.check_empty(self.smtp_server, "SMTP Server")
        self.check_empty(self.email, "Email")
        self.check_empty(self.password, "Password")
        self.check_empty(self.sender_name, "Sender Name")

class Email(ComponentBase, ABC):
    component_name = "Email"
    
    def _run(self, history, **kwargs):
        # 获取上游组件输出并解析JSON
        ans = self.get_input()
        content = "".join(ans["content"]) if "content" in ans else ""
        if not content:
            return Email.be_output("没有内容可发送")
        success = False
        try:
            # 解析从上游传递的JSON字符串
            email_data = json.loads(content)
            # 创建邮件对象
            msg = MIMEMultipart('alternative')
            # 正确处理发件人名称编码
            msg['From'] = formataddr((str(Header(self._param.sender_name,'utf-8')), self._param.email))
            msg['To'] = email_data["to_email"]
            if "cc_email" in email_data and email_data["cc_email"]:
                msg['Cc'] = email_data["cc_email"]
            msg['Subject'] = Header(email_data.get("subject", "无主题"), 'utf-8').encode()
            # 使用email_data中的内容或默认内容
            email_content = email_data.get("content", "未提供内容")
            msg.attach(MIMEText(email_content, 'html', 'utf-8'))
            # 连接SMTP服务器并发送
            logging.info(f"正在连接SMTP服务器 {self._param.smtp_server}:{self._param.smtp_port}")
            context = smtplib.ssl.create_default_context()
            with smtplib.SMTP_SSL(self._param.smtp_server, self._param.smtp_port, context=context) as server:
                # 登录
                logging.info(f"尝试使用邮箱登录: {self._param.email}")
                server.login(self._param.email, self._param.password)
                # 获取所有收件人列表
                recipients = [email_data["to_email"]]
                if "cc_email" in email_data and email_data["cc_email"]:
                    recipients.extend(email_data["cc_email"].split(','))
                # 发送邮件
                logging.info(f"正在发送邮件给收件人: {recipients}")
                try:
                    server.send_message(msg, self._param.email, recipients)
                    success = True
                except Exception as e:
                    logging.error(f"发送消息时出错: {str(e)}")
                    # 尝试替代方法
                    server.sendmail(self._param.email, recipients, msg.as_string())
                    success = True
                try:
                    server.quit()
                except Exception as e:
                    # 关闭连接时忽略错误
                    logging.warning(f"关闭连接时的非致命错误: {str(e)}")
            if success:
                return Email.be_output(True)
        except json.JSONDecodeError:
            # 101 输入的JSON格式无效
            error_msg = "101"
            logging.error(error_msg)
            return Email.be_output(error_msg)
        except smtplib.SMTPAuthenticationError:
            # 102 SMTP认证失败。请检查您的邮箱和授权码。
            error_msg = "102"
            logging.error(error_msg)
            return Email.be_output(error_msg)
        except smtplib.SMTPConnectError:
            # 103 无法连接到SMTP服务器
            error_msg = "103"
            logging.error(error_msg)
            return Email.be_output(error_msg)
        except smtplib.SMTPException as e:
            # 104 发生SMTP错误
            error_msg = "104"
            logging.error(error_msg)
            return Email.be_output(error_msg)
        except Exception as e:
            # 105 发生意外错误
            error_msg = "105"
            logging.error(error_msg)
            return Email.be_output(error_msg)