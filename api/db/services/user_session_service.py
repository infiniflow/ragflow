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
import logging
from datetime import datetime

from api.db.db_models import DB, UserSession
from api.db.services.common_service import CommonService
from common.misc_utils import get_uuid
from common.time_utils import current_timestamp, datetime_format


class UserSessionService(CommonService):
    """用户会话服务类，支持多点登录"""
    model = UserSession

    @classmethod
    @DB.connection_context()
    def create_session(cls, user_id, device_name=None, ip_address=None, expires_in=2592000):
        """
        创建新的用户会话
        
        Args:
            user_id: 用户ID
            device_name: 设备名称或浏览器信息
            ip_address: IP地址
            expires_in: 会话过期时间（秒），默认30天（2592000秒）
            
        Returns:
            (success, session_dict): 成功标志和会话信息字典
        """
        try:
            session_id = get_uuid()
            access_token = get_uuid()
            current_time = current_timestamp()
            expires_at = current_time + expires_in if expires_in else None
            
            session = cls.model(
                id=session_id,
                user_id=user_id,
                access_token=access_token,
                device_name=device_name,
                ip_address=ip_address,
                is_active="1",
                last_activity_time=current_time,
                expires_at=expires_at,
                create_time=current_time,
                create_date=datetime_format(datetime.now()),
                update_time=current_time,
                update_date=datetime_format(datetime.now())
            )
            session.save(force_insert=True)
            
            return True, session.to_dict()
        except Exception as e:
            logging.exception(f"创建会话失败: {e}")
            return False, {"error": str(e)}

    @classmethod
    @DB.connection_context()
    def get_session_by_token(cls, access_token):
        """
        通过access_token获取会话
        
        Args:
            access_token: 访问令牌
            
        Returns:
            session dict or None
        """
        try:
            if not access_token or not str(access_token).strip():
                return None
            
            session = cls.model.select().where(
                (cls.model.access_token == access_token) &
                (cls.model.is_active == "1")
            ).first()
            
            if not session:
                return None
            
            # 检查是否过期：基于最后活动时间 + 30天（滚动过期）
            if session.last_activity_time:
                current_time = current_timestamp()
                # 30天未活动才过期
                inactivity_timeout = 2592000  # 30天
                if current_time - session.last_activity_time > inactivity_timeout:
                    # 会话已过期，标记为非活跃
                    cls.logout_session(access_token)
                    return None
            
            return session.to_dict()
        except Exception as e:
            logging.exception(f"获取会话失败: {e}")
            return None

    @classmethod
    @DB.connection_context()
    def get_user_sessions(cls, user_id, active_only=True):
        """
        获取用户的所有会话
        
        Args:
            user_id: 用户ID
            active_only: 是否只返回活跃会话
            
        Returns:
            list of session dicts
        """
        try:
            query = cls.model.select().where(cls.model.user_id == user_id)
            
            if active_only:
                query = query.where(cls.model.is_active == "1")
                
            sessions = list(query.order_by(cls.model.last_activity_time.desc()).dicts())
            
            # 过滤过期的会话：基于最后活动时间
            current_time = current_timestamp()
            inactivity_timeout = 2592000  # 30天
            valid_sessions = []
            for session in sessions:
                if session.get('last_activity_time'):
                    if current_time - session['last_activity_time'] > inactivity_timeout:
                        # 标记为非活跃
                        cls.logout_session(session['access_token'])
                        continue
                valid_sessions.append(session)
            
            return valid_sessions
        except Exception as e:
            logging.exception(f"获取用户会话列表失败: {e}")
            return []

    @classmethod
    @DB.connection_context()
    def logout_session(cls, access_token):
        """
        登出指定会话
        
        Args:
            access_token: 访问令牌
            
        Returns:
            bool: 是否成功
        """
        try:
            updated = cls.model.update(
                is_active="0",
                update_time=current_timestamp(),
                update_date=datetime_format(datetime.now())
            ).where(
                cls.model.access_token == access_token
            ).execute()
            return updated > 0
        except Exception as e:
            logging.exception(f"登出会话失败: {e}")
            return False

    @classmethod
    @DB.connection_context()
    def logout_all_sessions(cls, user_id):
        """
        登出用户的所有会话
        
        Args:
            user_id: 用户ID
            
        Returns:
            int: 登出的会话数量
        """
        try:
            updated = cls.model.update(
                is_active="0",
                update_time=current_timestamp(),
                update_date=datetime_format(datetime.now())
            ).where(
                (cls.model.user_id == user_id) &
                (cls.model.is_active == "1")
            ).execute()
            return updated
        except Exception as e:
            logging.exception(f"登出所有会话失败: {e}")
            return 0

    @classmethod
    @DB.connection_context()
    def update_last_activity(cls, access_token):
        """
        更新会话的最后活动时间
        
        Args:
            access_token: 访问令牌
            
        Returns:
            bool: 是否成功
        """
        try:
            updated = cls.model.update(
                last_activity_time=current_timestamp(),
                update_time=current_timestamp(),
                update_date=datetime_format(datetime.now())
            ).where(
                (cls.model.access_token == access_token) &
                (cls.model.is_active == "1")
            ).execute()
            return updated > 0
        except Exception as e:
            logging.exception(f"更新会话活动时间失败: {e}")
            return False

    @classmethod
    @DB.connection_context()
    def remove_expired_sessions(cls, user_id=None):
        """
        清理过期的会话（基于最后活动时间）
        
        Args:
            user_id: 用户ID（可选），如果提供则只清理该用户的过期会话
            
        Returns:
            int: 清理的会话数量
        """
        try:
            current_time = current_timestamp()
            inactivity_timeout = 2592000  # 30天
            expiry_threshold = current_time - inactivity_timeout
            
            query = cls.model.update(
                is_active="0",
                update_time=current_time,
                update_date=datetime_format(datetime.now())
            ).where(
                (cls.model.last_activity_time < expiry_threshold) &
                (cls.model.is_active == "1")
            )
            
            if user_id:
                query = query.where(cls.model.user_id == user_id)
            
            removed = query.execute()
            return removed
        except Exception as e:
            logging.exception(f"清理过期会话失败: {e}")
            return 0
