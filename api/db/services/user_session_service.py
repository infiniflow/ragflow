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
    """User session service class, supports multiple login sessions"""
    model = UserSession

    @classmethod
    @DB.connection_context()
    def create_session(cls, user_id, device_name=None, ip_address=None, expires_in=2592000):
        """
        Create a new user session
        
        Args:
            user_id: User ID
            device_name: Device name or browser information
            ip_address: IP address
            expires_in: Session expiration time (seconds), default 30 days (2592000 seconds)
            
        Returns:
            (success, session_dict): Success flag and session information dictionary
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
            logging.exception(f"Failed to create session: {e}")
            return False, {"error": str(e)}

    @classmethod
    @DB.connection_context()
    def get_session_by_token(cls, access_token):
        """
        Get session by access_token
        
        Args:
            access_token: Access token
            
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
            
            # Check expiration: based on last activity time + 30 days (rolling expiration)
            if session.last_activity_time:
                current_time = current_timestamp()
                # Expires after 30 days of inactivity
                inactivity_timeout = 2592000  # 30 days
                if current_time - session.last_activity_time > inactivity_timeout:
                    # Session expired, mark as inactive
                    cls.logout_session(access_token)
                    return None
            
            return session.to_dict()
        except Exception as e:
            logging.exception(f"Failed to get session: {e}")
            return None

    @classmethod
    @DB.connection_context()
    def get_user_sessions(cls, user_id, active_only=True):
        """
        Get all sessions for a user
        
        Args:
            user_id: User ID
            active_only: Whether to return only active sessions
            
        Returns:
            list of session dicts
        """
        try:
            query = cls.model.select().where(cls.model.user_id == user_id)
            
            if active_only:
                query = query.where(cls.model.is_active == "1")
                
            sessions = list(query.order_by(cls.model.last_activity_time.desc()).dicts())
            
            # Filter expired sessions: based on last activity time
            current_time = current_timestamp()
            inactivity_timeout = 2592000  # 30 days
            valid_sessions = []
            for session in sessions:
                if session.get('last_activity_time'):
                    if current_time - session['last_activity_time'] > inactivity_timeout:
                        # Mark as inactive
                        cls.logout_session(session['access_token'])
                        continue
                valid_sessions.append(session)
            
            return valid_sessions
        except Exception as e:
            logging.exception(f"Failed to get user session list: {e}")
            return []

    @classmethod
    @DB.connection_context()
    def logout_session(cls, access_token):
        """
        Logout a specific session
        
        Args:
            access_token: Access token
            
        Returns:
            bool: Success status
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
            logging.exception(f"Failed to logout session: {e}")
            return False

    @classmethod
    @DB.connection_context()
    def logout_all_sessions(cls, user_id):
        """
        Logout all sessions for a user
        
        Args:
            user_id: User ID
            
        Returns:
            int: Number of sessions logged out
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
            logging.exception(f"Failed to logout all sessions: {e}")
            return 0

    @classmethod
    @DB.connection_context()
    def update_last_activity(cls, access_token):
        """
        Update the last activity time of a session
        
        Args:
            access_token: Access token
            
        Returns:
            bool: Success status
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
            logging.exception(f"Failed to update session activity time: {e}")
            return False

    @classmethod
    @DB.connection_context()
    def remove_expired_sessions(cls, user_id=None):
        """
        Clean up expired sessions (based on last activity time)
        
        Args:
            user_id: User ID (optional), if provided only clean up expired sessions for that user
            
        Returns:
            int: Number of sessions cleaned up
        """
        try:
            current_time = current_timestamp()
            inactivity_timeout = 2592000  # 30 days
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
            logging.exception(f"Failed to clean up expired sessions: {e}")
            return 0
