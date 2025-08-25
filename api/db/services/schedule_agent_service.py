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

import logging
from typing import List
from datetime import datetime, timedelta
from api.db.db_models import ScheduleAgent, ScheduleAgentRun
from api.db.services.common_service import CommonService
from api.utils import datetime_format, get_uuid


class ScheduleAgentService(CommonService):
    model = ScheduleAgent

    @classmethod
    def create_schedule(cls, **kwargs):
        """Create a new schedule"""
        logging.info(f"Creating schedule: {kwargs.get('name', 'Unknown')}")

        try:
            cls.validate_schedule_data(**kwargs)
            result = cls.save(**kwargs)

            if not result:
                raise Exception("Failed to save schedule")

            schedule_id = kwargs.get("id")
            if not schedule_id:
                raise Exception("No schedule ID provided")

            e, schedule_obj = cls.get_by_id(schedule_id)
            if not (e and schedule_obj):
                raise Exception(f"Failed to retrieve created schedule: {schedule_id}")

            logging.info(f"Successfully created schedule: {schedule_obj.id}")
            return schedule_obj

        except Exception as e:
            logging.error(f"Error creating schedule: {e}")
            raise

    @classmethod
    def get_schedules_paginated(cls, created_by=None, canvas_id=None, keywords="", page=1, page_size=20):
        """Get schedules with pagination and filtering"""
        try:
            conditions = [cls.model.status == "1"]

            if created_by:
                conditions.append(cls.model.created_by == created_by)
            if canvas_id:
                conditions.append(cls.model.canvas_id == canvas_id)
            if keywords:
                conditions.append((cls.model.name.contains(keywords)) | (cls.model.description.contains(keywords)))

            query = cls.model.select().where(*conditions)
            total = query.count()

            schedules = query.order_by(cls.model.create_time.desc()).paginate(page, page_size)
            return list(schedules), total

        except Exception as e:
            logging.error(f"Error fetching schedules: {e}")
            raise

    @classmethod
    def get_pending_schedules(cls) -> List[ScheduleAgent]:
        """Get schedules ready to run"""
        try:
            current_datetime = datetime.now()
            all_schedules = cls.query(enabled=True, status="1")

            if not all_schedules:
                return []

            valid_schedules = []
            disabled_count = 0

            for schedule in all_schedules:
                try:
                    if cls._is_currently_running(schedule.id):
                        continue

                    should_run = cls._should_schedule_run(schedule, current_datetime)

                    if should_run:
                        valid_schedules.append(schedule)
                    elif schedule.frequency_type == "once" and cls._has_successful_run(schedule.id):
                        cls.update_by_id(schedule.id, {"enabled": False})
                        disabled_count += 1

                except Exception as e:
                    logging.error(f"Error processing schedule {schedule.id}: {e}")
                    continue

            if disabled_count > 0:
                logging.info(f"Disabled {disabled_count} completed one-time schedules")

            logging.info(f"Found {len(valid_schedules)} pending schedules")
            return valid_schedules

        except Exception as e:
            logging.error(f"Error getting pending schedules: {e}")
            return []

    @classmethod
    def _should_schedule_run(cls, schedule, current_datetime):
        """Check if schedule should run based on type"""
        schedule_checks = {"once": cls._should_run_once, "daily": lambda s, dt: cls._should_run_recurring(s, dt, cls._has_run_today), "weekly": cls._should_run_weekly, "monthly": cls._should_run_monthly}

        try:
            check_func = schedule_checks.get(schedule.frequency_type)
            if check_func:
                return check_func(schedule, current_datetime)
            return False
        except Exception as e:
            logging.error(f"Error checking schedule {schedule.id}: {e}")
            return False

    @classmethod
    def _should_run_once(cls, schedule: ScheduleAgent, current_datetime):
        """Check if one-time schedule should run"""
        if not schedule.execute_date or not schedule.execute_time:
            return False

        execute_datetime = cls._get_execute_datetime(schedule.execute_date, schedule.execute_time)
        return current_datetime >= execute_datetime and not cls._has_any_run(schedule.id)

    @classmethod
    def _should_run_recurring(cls, schedule: ScheduleAgent, current_datetime, check_already_run_func):
        """Generic check for recurring schedules"""
        if not schedule.execute_time:
            return False

        today_execute_time = cls._get_today_execute_time(current_datetime, schedule.execute_time)
        return current_datetime >= today_execute_time and not check_already_run_func(schedule.id)

    @classmethod
    def _should_run_weekly(cls, schedule: ScheduleAgent, current_datetime):
        """Check if weekly schedule should run"""
        if not schedule.execute_time or not schedule.days_of_week:
            return False

        current_weekday = current_datetime.weekday() + 1
        if current_weekday not in schedule.days_of_week:
            return False

        return cls._should_run_recurring(schedule, current_datetime, cls._has_run_today)

    @classmethod
    def _should_run_monthly(cls, schedule: ScheduleAgent, current_datetime):
        """Check if monthly schedule should run"""
        if not schedule.execute_time or not schedule.day_of_month:
            return False

        if current_datetime.day != schedule.day_of_month:
            return False

        return cls._should_run_recurring(schedule, current_datetime, cls._has_run_this_month)

    @classmethod
    def start_execution(cls, schedule_id):
        """Start execution tracking"""
        run_id = get_uuid()
        ScheduleAgentRun.create(id=run_id, schedule_id=schedule_id, started_at=datetime_format(datetime.now()))
        return run_id

    @classmethod
    def finish_execution(cls, run_id, success=True, error_message=None, conversation_id=None):
        """Finish execution tracking"""
        try:
            run = ScheduleAgentRun.get_by_id(run_id)
            finish_time = datetime_format(datetime.now())

            ScheduleAgentRun.update(finished_at=finish_time, success=success, error_message=error_message, conversation_id=conversation_id).where(ScheduleAgentRun.id == run_id).execute()

            # Disable one-time schedules after successful execution
            if success:
                schedule = ScheduleAgent.get_by_id(run.schedule_id)
                if schedule.frequency_type == "once":
                    cls.update_by_id(run.schedule_id, {"enabled": False})

            return True
        except Exception as e:
            logging.error(f"Error finishing execution {run_id}: {e}")
            return False

    @classmethod
    def _run_exists(cls, schedule_id, success_filter=None, time_range=None):
        """Generic method to check if runs exist with optional filters"""
        try:
            query = ScheduleAgentRun.select().where(ScheduleAgentRun.schedule_id == schedule_id)

            if success_filter is not None:
                query = query.where(ScheduleAgentRun.success == success_filter)

            if time_range:
                start_dt, end_dt = time_range
                query = query.where((ScheduleAgentRun.started_at >= start_dt) & (ScheduleAgentRun.started_at <= end_dt))

            return query.exists()
        except Exception:
            return False

    @classmethod
    def _is_currently_running(cls, schedule_id):
        """Check if schedule is currently running"""
        try:
            return ScheduleAgentRun.select().where((ScheduleAgentRun.schedule_id == schedule_id) & (ScheduleAgentRun.finished_at.is_null(True))).exists()
        except Exception:
            return False

    @classmethod
    def _has_successful_run(cls, schedule_id):
        """Check if schedule has any successful run"""
        return cls._run_exists(schedule_id, success_filter=True)

    @classmethod
    def _has_any_run(cls, schedule_id):
        """Check if schedule has any run"""
        return cls._run_exists(schedule_id)

    @classmethod
    def _has_run_today(cls, schedule_id):
        """Check if schedule ran successfully today"""
        today_start = datetime.now().replace(hour=0, minute=0, second=0, microsecond=0)
        today_end = today_start.replace(hour=23, minute=59, second=59, microsecond=999999)
        time_range = (today_start, today_end)
        return cls._run_exists(schedule_id, success_filter=True, time_range=time_range)

    @classmethod
    def _has_run_this_month(cls, schedule_id):
        """Check if schedule ran successfully this month"""
        current_date = datetime.now()
        month_start = current_date.replace(day=1, hour=0, minute=0, second=0, microsecond=0)

        if current_date.month == 12:
            next_month = current_date.replace(year=current_date.year + 1, month=1, day=1)
        else:
            next_month = current_date.replace(month=current_date.month + 1, day=1)
        month_end = next_month - timedelta(microseconds=1)

        time_range = (month_start, month_end)
        return cls._run_exists(schedule_id, success_filter=True, time_range=time_range)

    @classmethod
    def _get_last_successful_run(cls, schedule_id):
        """Get the last successful run for a schedule"""
        try:
            return ScheduleAgentRun.select().where((ScheduleAgentRun.schedule_id == schedule_id) & (ScheduleAgentRun.success)).order_by(ScheduleAgentRun.started_at.desc()).first()
        except Exception:
            return None

    @classmethod
    def _get_run_count(cls, schedule_id, success_only=True):
        """Get run count for a schedule"""
        try:
            query = ScheduleAgentRun.select().where(ScheduleAgentRun.schedule_id == schedule_id)
            if success_only:
                query = query.where(ScheduleAgentRun.success)
            return query.count()
        except Exception:
            return 0

    @classmethod
    def get_schedule_execution_history(cls, schedule_id, limit=10):
        """Get execution history for a schedule"""
        try:
            return list(ScheduleAgentRun.select().where(ScheduleAgentRun.schedule_id == schedule_id).order_by(ScheduleAgentRun.started_at.desc()).limit(limit))
        except Exception:
            return []

    @classmethod
    def validate_schedule_data(cls, **kwargs):
        """Validate schedule data"""
        frequency_type = kwargs.get("frequency_type", "once")
        validation_rules = {"once": lambda k: k.get("execute_date"), "weekly": lambda k: k.get("days_of_week") and all(1 <= day <= 7 for day in k.get("days_of_week", [])), "monthly": lambda k: k.get("day_of_month") and 1 <= k.get("day_of_month", 0) <= 31}

        try:
            # Validate frequency-specific requirements
            if frequency_type in validation_rules:
                if not validation_rules[frequency_type](kwargs):
                    raise ValueError(f"Invalid data for {frequency_type} schedule")

            # Validate time format
            execute_time = kwargs.get("execute_time", "00:00:00")
            if execute_time:
                cls._validate_time_format(execute_time)

            return True

        except Exception as e:
            logging.error(f"Schedule validation failed: {e}")
            raise

    @classmethod
    def _validate_time_format(cls, execute_time):
        """Validate time format"""
        try:
            time_parts = execute_time.split(":")
            hour, minute = int(time_parts[0]), int(time_parts[1])
            second = int(time_parts[2]) if len(time_parts) > 2 else 0

            if not (0 <= hour <= 23 and 0 <= minute <= 59 and 0 <= second <= 59):
                raise ValueError("Invalid time format")
        except (ValueError, IndexError):
            raise ValueError("Time must be in HH:MM:SS format")

    @classmethod
    def _get_execute_datetime(cls, execute_date, execute_time):
        """Convert execute_date and execute_time to datetime"""
        if isinstance(execute_date, str):
            date_obj = datetime.strptime(execute_date, "%Y-%m-%d").date()
        else:
            date_obj = execute_date

        if isinstance(execute_time, str):
            time_parts = execute_time.split(":")
            hour, minute = int(time_parts[0]), int(time_parts[1])
            second = int(time_parts[2]) if len(time_parts) > 2 else 0
            time_obj = datetime.min.time().replace(hour=hour, minute=minute, second=second)
        else:
            time_obj = execute_time

        return datetime.combine(date_obj, time_obj)

    @classmethod
    def _get_today_execute_time(cls, current_datetime, execute_time):
        """Get today's execution time"""
        if isinstance(execute_time, str):
            time_parts = execute_time.split(":")
            hour, minute = int(time_parts[0]), int(time_parts[1])
            second = int(time_parts[2]) if len(time_parts) > 2 else 0
        else:
            hour, minute, second = execute_time.hour, execute_time.minute, execute_time.second

        return current_datetime.replace(hour=hour, minute=minute, second=second, microsecond=0)
