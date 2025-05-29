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
from croniter import croniter
from datetime import datetime, timedelta
from api.db.db_models import ScheduleAgent
from api.db.services.common_service import CommonService
from api.utils import current_timestamp


class ScheduleAgentService(CommonService):
    model = ScheduleAgent

    @classmethod
    def create_schedule(cls, **kwargs):
        """Create a new schedule with detailed logging"""
        schedule_info = "Name:{}, Canvas:{}, Type:{}".format(kwargs.get("name", "Unknown"), kwargs.get("canvas_id", "Unknown"), kwargs.get("frequency_type", "Unknown"))

        logging.info("[SCHEDULE_SERVICE] 📅 Creating new schedule - {}".format(schedule_info))

        try:
            # Generate cron expression based on frequency type
            logging.debug("[SCHEDULE_SERVICE] Generating cron expression for schedule: {}".format(schedule_info))
            cron_expr = cls.generate_cron_expression(**kwargs)
            if cron_expr:
                kwargs["cron_expression"] = cron_expr
                logging.debug("[SCHEDULE_SERVICE] Generated cron expression: {}".format(cron_expr))
            else:
                logging.debug("[SCHEDULE_SERVICE] No cron expression generated for {} frequency".format(kwargs.get("frequency_type")))

            # Validate cron expression
            if kwargs.get("cron_expression"):
                logging.debug("[SCHEDULE_SERVICE] Validating cron expression: {}".format(kwargs["cron_expression"]))
                if not cls.validate_cron_expression(kwargs["cron_expression"]):
                    error_msg = "Invalid cron expression: {}".format(kwargs["cron_expression"])
                    logging.error("[SCHEDULE_SERVICE] ❌ {}".format(error_msg))
                    raise ValueError(error_msg)
                logging.debug("[SCHEDULE_SERVICE] ✅ Cron expression validation passed")

            # Calculate next run time
            logging.debug("[SCHEDULE_SERVICE] Calculating next run time for schedule: {}".format(schedule_info))
            if kwargs.get("cron_expression"):
                next_run = cls.calculate_next_run_time(kwargs["cron_expression"])
                kwargs["next_run_time"] = next_run
                if next_run:
                    next_run_date = datetime.fromtimestamp(next_run)
                    logging.info("[SCHEDULE_SERVICE] Next run time calculated: {}".format(next_run_date.isoformat()))
                else:
                    logging.warning("[SCHEDULE_SERVICE] ⚠️  Failed to calculate next run time from cron expression")

            elif kwargs.get("frequency_type") == "once" and kwargs.get("execute_date"):
                # For one-time execution, use the specific date
                execute_datetime = kwargs["execute_date"]
                if kwargs.get("execute_time"):
                    time_parts = kwargs["execute_time"].split(":")
                    execute_datetime = execute_datetime.replace(hour=int(time_parts[0]), minute=int(time_parts[1]), second=int(time_parts[2]) if len(time_parts) > 2 else 0)
                kwargs["next_run_time"] = int(execute_datetime.timestamp())
                logging.info("[SCHEDULE_SERVICE] One-time execution scheduled for: {}".format(execute_datetime.isoformat()))

            # Save the schedule
            logging.debug("[SCHEDULE_SERVICE] Saving schedule to database: {}".format(schedule_info))
            result = cls.save(**kwargs)

            if result:
                # Get the created schedule object
                schedule_id = kwargs.get("id")
                if schedule_id:
                    e, schedule_obj = cls.get_by_id(schedule_id)
                    if e and schedule_obj:
                        logging.info("[SCHEDULE_SERVICE] ✅ Successfully created schedule - ID:{}, {}".format(schedule_obj.id, schedule_info))
                        logging.debug("[SCHEDULE_SERVICE] Schedule details - Enabled:{}, Next run:{}".format(schedule_obj.enabled, schedule_obj.next_run_time))
                        return schedule_obj
                    else:
                        logging.error("[SCHEDULE_SERVICE] ❌ Failed to retrieve created schedule with ID: {}".format(schedule_id))
                        raise Exception("Failed to retrieve created schedule with ID: {}".format(schedule_id))
                else:
                    logging.error("[SCHEDULE_SERVICE] ❌ No schedule ID provided for creation")
                    raise Exception("No schedule ID provided for creation")
            else:
                logging.error("[SCHEDULE_SERVICE] ❌ Failed to save schedule to database: {}".format(schedule_info))
                raise Exception("Failed to save schedule to database: {}".format(schedule_info))

        except Exception as e:
            logging.error("[SCHEDULE_SERVICE] ❌ Error creating schedule: {}".format(schedule_info))
            logging.error("[SCHEDULE_SERVICE] Error type: {}".format(type(e).__name__))
            logging.error("[SCHEDULE_SERVICE] Error message: {}".format(str(e)))
            logging.exception("[SCHEDULE_SERVICE] Exception details: {}".format(e))
            raise

    @classmethod
    def get_schedules_paginated(cls, created_by=None, canvas_id=None, keywords="", page=1, page_size=20):
        """Get schedules with pagination and filtering with detailed logging"""
        logging.debug("[SCHEDULE_SERVICE] 📋 Fetching schedules - User:{}, Canvas:{}, Keywords:'{}', Page:{}, Size:{}".format(created_by, canvas_id, keywords, page, page_size))

        try:
            from peewee import fn

            # Build query conditions
            conditions = []

            if created_by:
                conditions.append(cls.model.created_by == created_by)
                logging.debug("[SCHEDULE_SERVICE] Added user filter: {}".format(created_by))

            if canvas_id:
                conditions.append(cls.model.canvas_id == canvas_id)
                logging.debug("[SCHEDULE_SERVICE] Added canvas filter: {}".format(canvas_id))

            if keywords:
                conditions.append((cls.model.name.contains(keywords)) | (cls.model.description.contains(keywords)))
                logging.debug("[SCHEDULE_SERVICE] Added keywords filter: '{}'".format(keywords))

            # Add status filter (only valid schedules)
            conditions.append(cls.model.status == "1")

            # Build base query
            query = cls.model.select()
            if conditions:
                query = query.where(*conditions)

            # Get total count
            total = query.count()
            logging.debug("[SCHEDULE_SERVICE] Total matching schedules: {}".format(total))

            # Apply pagination and ordering
            schedules = query.order_by(cls.model.create_time.desc()).paginate(page, page_size)

            schedule_list = list(schedules)
            logging.info("[SCHEDULE_SERVICE] ✅ Retrieved {} schedules (page {}/{})".format(len(schedule_list), page, ((total - 1) // page_size) + 1))

            return schedule_list, total

        except Exception as e:
            logging.error("[SCHEDULE_SERVICE] ❌ Error fetching schedules - User:{}, Canvas:{}".format(created_by, canvas_id))
            logging.error("[SCHEDULE_SERVICE] Error type: {}".format(type(e).__name__))
            logging.error("[SCHEDULE_SERVICE] Error message: {}".format(str(e)))
            logging.exception("[SCHEDULE_SERVICE] Exception details: {}".format(e))
            raise

    @classmethod
    def get_pending_schedules(cls, tolerance_minutes=5):
        """Get schedules that are ready to run with detailed logging and specific logic for each frequency type"""
        logging.debug("[SCHEDULE_SERVICE] 🔍 Checking for pending schedules (tolerance: {} minutes)".format(tolerance_minutes))

        try:
            current_time = current_timestamp()
            current_datetime = datetime.now()

            logging.debug("[SCHEDULE_SERVICE] Current timestamp: {} ({})".format(current_time, current_datetime.isoformat()))

            # Get all enabled schedules
            all_schedules: List[ScheduleAgent] = cls.query(enabled=True, status="1")

            logging.debug("[SCHEDULE_SERVICE] Found {} enabled schedules to check".format(len(all_schedules)))

            valid_schedules = []
            disabled_count = 0

            for schedule in all_schedules:
                schedule_info = "ID:{}, Name:{}, Type:{}".format(schedule.id, schedule.name, schedule.frequency_type)

                try:
                    # Validate timestamps
                    if schedule.next_run_time and (schedule.next_run_time < 0 or schedule.next_run_time > 2147483647):
                        logging.warning("[SCHEDULE_SERVICE] ⚠️  Invalid next_run_time for {}: {}".format(schedule_info, schedule.next_run_time))
                        cls.update_by_id(schedule.id, {"next_run_time": None})
                        continue

                    should_run = False

                    # Check each frequency type with specific conditions
                    if schedule.frequency_type == "once":
                        # Once: current time > execute_date + execute_time AND run_count = 0
                        if schedule.run_count == 0 and schedule.execute_date and schedule.execute_time:
                            execute_datetime = schedule.execute_date
                            time_parts = schedule.execute_time.split(":")
                            execute_datetime = execute_datetime.replace(hour=int(time_parts[0]), minute=int(time_parts[1]), second=int(time_parts[2]) if len(time_parts) > 2 else 0, microsecond=0)

                            if current_datetime >= execute_datetime:
                                should_run = True
                                logging.debug("[SCHEDULE_SERVICE] ✅ Once schedule ready: {}".format(schedule_info))
                            else:
                                logging.debug("[SCHEDULE_SERVICE] Once schedule not ready: {} - Execute at: {}".format(schedule_info, execute_datetime.isoformat()))

                        elif schedule.run_count > 0:
                            # Disable completed one-time schedules
                            logging.info("[SCHEDULE_SERVICE] 🔒 Disabling completed one-time schedule: {}".format(schedule_info))
                            cls.update_by_id(schedule.id, {"enabled": False})
                            disabled_count += 1
                            continue

                    elif schedule.frequency_type == "daily":
                        # Daily: current time > execute_time today
                        if schedule.execute_time:
                            time_parts = schedule.execute_time.split(":")
                            today_execute_time = current_datetime.replace(hour=int(time_parts[0]), minute=int(time_parts[1]), second=int(time_parts[2]) if len(time_parts) > 2 else 0, microsecond=0)

                            # Check if we've passed today's execution time
                            if current_datetime >= today_execute_time:
                                # Check if we haven't run today yet
                                if schedule.last_run_time:
                                    last_run_date = datetime.fromtimestamp(schedule.last_run_time).date()
                                    if last_run_date < current_datetime.date():
                                        should_run = True
                                        logging.debug("[SCHEDULE_SERVICE] ✅ Daily schedule ready: {}".format(schedule_info))
                                    else:
                                        logging.debug("[SCHEDULE_SERVICE] Daily schedule already ran today: {}".format(schedule_info))
                                else:
                                    # Never run before
                                    should_run = True
                                    logging.debug("[SCHEDULE_SERVICE] ✅ Daily schedule ready (first run): {}".format(schedule_info))
                            else:
                                logging.debug("[SCHEDULE_SERVICE] Daily schedule not ready: {} - Execute at: {}".format(schedule_info, today_execute_time.strftime("%H:%M:%S")))

                    elif schedule.frequency_type == "weekly":
                        # Weekly: current time > execute_time AND current day in days_of_week
                        if schedule.execute_time and schedule.days_of_week:
                            current_weekday = current_datetime.weekday() + 1  # Convert to 1=Monday format

                            if current_weekday in schedule.days_of_week:
                                time_parts = schedule.execute_time.split(":")
                                today_execute_time = current_datetime.replace(hour=int(time_parts[0]), minute=int(time_parts[1]), second=int(time_parts[2]) if len(time_parts) > 2 else 0, microsecond=0)

                                if current_datetime >= today_execute_time:
                                    # Check if we haven't run today yet
                                    if schedule.last_run_time:
                                        last_run_date = datetime.fromtimestamp(schedule.last_run_time).date()
                                        if last_run_date < current_datetime.date():
                                            should_run = True
                                            logging.debug("[SCHEDULE_SERVICE] ✅ Weekly schedule ready: {}".format(schedule_info))
                                        else:
                                            logging.debug("[SCHEDULE_SERVICE] Weekly schedule already ran today: {}".format(schedule_info))
                                    else:
                                        # Never run before
                                        should_run = True
                                        logging.debug("[SCHEDULE_SERVICE] ✅ Weekly schedule ready (first run): {}".format(schedule_info))
                                else:
                                    logging.debug("[SCHEDULE_SERVICE] Weekly schedule not ready: {} - Execute at: {}".format(schedule_info, today_execute_time.strftime("%H:%M:%S")))
                            else:
                                logging.debug("[SCHEDULE_SERVICE] Weekly schedule not for today: {} - Current day: {}, Target days: {}".format(schedule_info, current_weekday, schedule.days_of_week))

                    elif schedule.frequency_type == "monthly":
                        # Monthly: current time > execute_time AND current day = day_of_month
                        if schedule.execute_time and schedule.day_of_month:
                            current_day = current_datetime.day

                            if current_day == schedule.day_of_month:
                                time_parts = schedule.execute_time.split(":")
                                today_execute_time = current_datetime.replace(hour=int(time_parts[0]), minute=int(time_parts[1]), second=int(time_parts[2]) if len(time_parts) > 2 else 0, microsecond=0)

                                if current_datetime >= today_execute_time:
                                    # Check if we haven't run this month yet
                                    if schedule.last_run_time:
                                        last_run_datetime = datetime.fromtimestamp(schedule.last_run_time)
                                        if last_run_datetime.year < current_datetime.year or last_run_datetime.month < current_datetime.month:
                                            should_run = True
                                            logging.debug("[SCHEDULE_SERVICE] ✅ Monthly schedule ready: {}".format(schedule_info))
                                        else:
                                            logging.debug("[SCHEDULE_SERVICE] Monthly schedule already ran this month: {}".format(schedule_info))
                                    else:
                                        # Never run before
                                        should_run = True
                                        logging.debug("[SCHEDULE_SERVICE] ✅ Monthly schedule ready (first run): {}".format(schedule_info))
                                else:
                                    logging.debug("[SCHEDULE_SERVICE] Monthly schedule not ready: {} - Execute at: {}".format(schedule_info, today_execute_time.strftime("%H:%M:%S")))
                            else:
                                logging.debug("[SCHEDULE_SERVICE] Monthly schedule not for today: {} - Current day: {}, Target day: {}".format(schedule_info, current_day, schedule.day_of_month))

                    if should_run:
                        # Log schedule details
                        try:
                            last_run_str = datetime.fromtimestamp(schedule.last_run_time).isoformat() if schedule.last_run_time else "Never"
                        except (ValueError, OSError):
                            last_run_str = "Invalid"

                        logging.debug("[SCHEDULE_SERVICE] ✅ Valid pending schedule: {}".format(schedule_info))
                        logging.debug("[SCHEDULE_SERVICE]   Last run: {}, Run count: {}".format(last_run_str, schedule.run_count))

                        valid_schedules.append(schedule)

                except Exception as schedule_error:
                    logging.error("[SCHEDULE_SERVICE] ❌ Error processing schedule {}: {}".format(schedule_info, schedule_error))
                    continue

            if disabled_count > 0:
                logging.info("[SCHEDULE_SERVICE] Disabled {} completed one-time schedules".format(disabled_count))

            logging.info("[SCHEDULE_SERVICE] ✅ Found {} valid pending schedules".format(len(valid_schedules)))

            return valid_schedules

        except Exception as e:
            logging.error("[SCHEDULE_SERVICE] ❌ Error getting pending schedules")
            logging.error("[SCHEDULE_SERVICE] Error type: {}".format(type(e).__name__))
            logging.error("[SCHEDULE_SERVICE] Error message: {}".format(str(e)))
            logging.exception("[SCHEDULE_SERVICE] Exception details: {}".format(e))
            return []

    @classmethod
    def update_after_run(cls, schedule_id, success=True):
        """Update schedule after execution with detailed logging"""
        status_str = "✅ SUCCESS" if success else "❌ FAILED"
        logging.info("[SCHEDULE_SERVICE] 🔄 Updating schedule {} after execution - Status: {}".format(schedule_id, status_str))

        try:
            e, schedule_obj = cls.get_by_id(schedule_id)
            if not e:
                logging.error("[SCHEDULE_SERVICE] ❌ Schedule {} not found for update".format(schedule_id))
                return False

            schedule_info = "ID:{}, Name:{}, Type:{}".format(schedule_obj.id, schedule_obj.name, schedule_obj.frequency_type)
            current_time = current_timestamp()

            update_data = {"last_run_time": current_time, "run_count": schedule_obj.run_count + 1}

            logging.debug("[SCHEDULE_SERVICE] Updating run statistics for {} - Run count: {} -> {}".format(schedule_info, schedule_obj.run_count, update_data["run_count"]))

            # Calculate next run time based on frequency type
            if schedule_obj.frequency_type == "once":
                # Disable one-time schedules after execution
                update_data["enabled"] = False
                update_data["next_run_time"] = None
                logging.info("[SCHEDULE_SERVICE] 🔒 Disabling one-time schedule after execution: {}".format(schedule_info))

            elif schedule_obj.frequency_type == "weekly":
                # For weekly schedules, calculate next run time for next week
                logging.debug("[SCHEDULE_SERVICE] Calculating next weekly run time for {}".format(schedule_info))

                if schedule_obj.cron_expression:
                    next_run = cls.calculate_next_run_time(schedule_obj.cron_expression)
                    update_data["next_run_time"] = next_run
                    if next_run:
                        next_run_date = datetime.fromtimestamp(next_run)
                        logging.info("[SCHEDULE_SERVICE] Next weekly run scheduled for: {}".format(next_run_date.isoformat()))
                    else:
                        logging.warning("[SCHEDULE_SERVICE] ⚠️  Failed to calculate next weekly run time using cron")
                else:
                    # Fallback: calculate next week manually
                    logging.debug("[SCHEDULE_SERVICE] Using manual calculation for weekly schedule {}".format(schedule_info))
                    current_datetime = datetime.now()
                    current_weekday = current_datetime.weekday() + 1  # Convert to 1=Monday format

                    if schedule_obj.days_of_week and current_weekday in schedule_obj.days_of_week:
                        # Find next occurrence of any day in days_of_week
                        days_ahead = 7  # Default to next week
                        for day in sorted(schedule_obj.days_of_week):
                            if day > current_weekday:
                                days_ahead = day - current_weekday
                                break

                        next_run_date = current_datetime + timedelta(days=days_ahead)

                        # Set the time
                        if schedule_obj.execute_time:
                            time_parts = schedule_obj.execute_time.split(":")
                            next_run_date = next_run_date.replace(hour=int(time_parts[0]), minute=int(time_parts[1]), second=int(time_parts[2]) if len(time_parts) > 2 else 0, microsecond=0)

                        update_data["next_run_time"] = int(next_run_date.timestamp())
                        logging.info("[SCHEDULE_SERVICE] Next weekly run manually calculated for: {}".format(next_run_date.isoformat()))

            elif schedule_obj.cron_expression:
                # Use cron expression for other recurring schedules
                logging.debug("[SCHEDULE_SERVICE] Calculating next run time using cron for {}".format(schedule_info))
                next_run = cls.calculate_next_run_time(schedule_obj.cron_expression)
                update_data["next_run_time"] = next_run
                if next_run:
                    next_run_date = datetime.fromtimestamp(next_run)
                    logging.info("[SCHEDULE_SERVICE] Next run scheduled for: {}".format(next_run_date.isoformat()))
                else:
                    logging.warning("[SCHEDULE_SERVICE] ⚠️  Failed to calculate next run time using cron")

            # Update the schedule
            logging.debug("[SCHEDULE_SERVICE] Saving updates to database for {}".format(schedule_info))
            result = cls.update_by_id(schedule_id, update_data)

            if result:
                logging.info("[SCHEDULE_SERVICE] ✅ Successfully updated schedule {} after execution".format(schedule_info))
                if update_data.get("next_run_time"):
                    next_run_str = datetime.fromtimestamp(update_data["next_run_time"]).isoformat()
                    logging.debug("[SCHEDULE_SERVICE] Next execution: {}".format(next_run_str))
            else:
                logging.error("[SCHEDULE_SERVICE] ❌ Failed to update schedule {} in database".format(schedule_info))

            return result

        except Exception as e:
            logging.error("[SCHEDULE_SERVICE] ❌ Error updating schedule {} after execution".format(schedule_id))
            logging.error("[SCHEDULE_SERVICE] Error type: {}".format(type(e).__name__))
            logging.error("[SCHEDULE_SERVICE] Error message: {}".format(str(e)))
            logging.exception("[SCHEDULE_SERVICE] Exception details: {}".format(e))
            return False

    @classmethod
    def generate_cron_expression(cls, **kwargs):
        """Generate cron expression based on frequency type and settings with logging"""
        frequency_type = kwargs.get("frequency_type", "once")
        execute_time = kwargs.get("execute_time", "00:00:00")

        logging.debug("[SCHEDULE_SERVICE] 🕐 Generating cron expression - Type: {}, Time: {}".format(frequency_type, execute_time))

        try:
            # Parse time
            time_parts = execute_time.split(":")
            hour = int(time_parts[0]) if len(time_parts) > 0 else 0
            minute = int(time_parts[1]) if len(time_parts) > 1 else 0
            second = int(time_parts[2]) if len(time_parts) > 2 else 0

            logging.debug("[SCHEDULE_SERVICE] Parsed time - Hour: {}, Minute: {}, Second: {}".format(hour, minute, second))

            if frequency_type == "once":
                # For one-time execution, we'll handle this separately
                logging.debug("[SCHEDULE_SERVICE] One-time execution - no cron expression needed")
                return None

            elif frequency_type == "daily":
                # Daily at specific time: "second minute hour * * *"
                cron_expr = "{} {} {} * * *".format(second, minute, hour)
                logging.debug("[SCHEDULE_SERVICE] Generated daily cron: {}".format(cron_expr))
                return cron_expr

            elif frequency_type == "weekly":
                # Weekly on specific days: "second minute hour * * day1,day2,..."
                days_of_week = kwargs.get("days_of_week", [1])  # Default to Monday
                if not days_of_week:
                    days_of_week = [1]

                logging.debug("[SCHEDULE_SERVICE] Weekly days: {}".format(days_of_week))

                # Convert from our format (1=Monday) to cron format (1=Monday, 7=Sunday)
                cron_days = []
                for day in days_of_week:
                    if day == 7:  # Sunday
                        cron_days.append("0")
                    else:
                        cron_days.append(str(day))

                days_str = ",".join(cron_days)
                cron_expr = "{} {} {} * * {}".format(second, minute, hour, days_str)
                logging.debug("[SCHEDULE_SERVICE] Generated weekly cron: {}".format(cron_expr))
                return cron_expr

            elif frequency_type == "monthly":
                # Monthly on specific day: "second minute hour day * *"
                day_of_month = kwargs.get("day_of_month", 1)
                cron_expr = "{} {} {} {} * *".format(second, minute, hour, day_of_month)
                logging.debug("[SCHEDULE_SERVICE] Generated monthly cron: {}".format(cron_expr))
                return cron_expr

            logging.warning("[SCHEDULE_SERVICE] ⚠️  Unknown frequency type: {}".format(frequency_type))
            return None

        except Exception as e:
            logging.error("[SCHEDULE_SERVICE] ❌ Error generating cron expression for {}".format(frequency_type))
            logging.error("[SCHEDULE_SERVICE] Error type: {}".format(type(e).__name__))
            logging.error("[SCHEDULE_SERVICE] Error message: {}".format(str(e)))
            logging.exception("[SCHEDULE_SERVICE] Exception details: {}".format(e))
            return None

    @classmethod
    def validate_cron_expression(cls, cron_expr):
        """Validate cron expression with logging"""
        logging.debug("[SCHEDULE_SERVICE] 🔍 Validating cron expression: {}".format(cron_expr))

        try:
            croniter(cron_expr)
            logging.debug("[SCHEDULE_SERVICE] ✅ Cron expression validation passed")
            return True
        except Exception as e:
            logging.warning("[SCHEDULE_SERVICE] ❌ Invalid cron expression '{}': {}".format(cron_expr, str(e)))
            return False

    @classmethod
    def calculate_next_run_time(cls, cron_expr):
        """Calculate next run time from cron expression with logging"""
        logging.debug("[SCHEDULE_SERVICE] ⏰ Calculating next run time from cron: {}".format(cron_expr))

        try:
            cron = croniter(cron_expr, datetime.now())
            next_time = cron.get_next(datetime)
            next_timestamp = int(next_time.timestamp())

            logging.debug("[SCHEDULE_SERVICE] ✅ Next run time calculated: {} (timestamp: {})".format(next_time.isoformat(), next_timestamp))
            return next_timestamp
        except Exception as e:
            logging.error("[SCHEDULE_SERVICE] ❌ Error calculating next run time from cron '{}': {}".format(cron_expr, str(e)))
            return None

    @classmethod
    def validate_schedule_data(cls, **kwargs):
        """Validate schedule data based on frequency type with detailed logging"""
        frequency_type = kwargs.get("frequency_type", "once")

        logging.debug("[SCHEDULE_SERVICE] 📝 Validating schedule data for frequency type: {}".format(frequency_type))

        try:
            if frequency_type == "once":
                if not kwargs.get("execute_date"):
                    error_msg = "Execute date is required for one-time schedules"
                    logging.error("[SCHEDULE_SERVICE] ❌ Validation failed: {}".format(error_msg))
                    raise ValueError(error_msg)
                logging.debug("[SCHEDULE_SERVICE] ✅ One-time schedule validation passed")

            elif frequency_type == "weekly":
                days_of_week = kwargs.get("days_of_week", [])
                if not days_of_week or not all(1 <= day <= 7 for day in days_of_week):
                    error_msg = "Valid days of week (1-7) are required for weekly schedules"
                    logging.error("[SCHEDULE_SERVICE] ❌ Validation failed: {} - Provided: {}".format(error_msg, days_of_week))
                    raise ValueError(error_msg)
                logging.debug("[SCHEDULE_SERVICE] ✅ Weekly schedule validation passed - Days: {}".format(days_of_week))

            elif frequency_type == "monthly":
                day_of_month = kwargs.get("day_of_month")
                if not day_of_month or not (1 <= day_of_month <= 31):
                    error_msg = "Valid day of month (1-31) is required for monthly schedules"
                    logging.error("[SCHEDULE_SERVICE] ❌ Validation failed: {} - Provided: {}".format(error_msg, day_of_month))
                    raise ValueError(error_msg)
                logging.debug("[SCHEDULE_SERVICE] ✅ Monthly schedule validation passed - Day: {}".format(day_of_month))

            # Validate time format
            execute_time = kwargs.get("execute_time", "00:00:00")
            if execute_time:
                try:
                    time_parts = execute_time.split(":")
                    hour = int(time_parts[0])
                    minute = int(time_parts[1])
                    second = int(time_parts[2]) if len(time_parts) > 2 else 0

                    if not (0 <= hour <= 23 and 0 <= minute <= 59 and 0 <= second <= 59):
                        error_msg = "Invalid time format - hour, minute, second out of range"
                        logging.error("[SCHEDULE_SERVICE] ❌ Time validation failed: {} - {}".format(error_msg, execute_time))
                        raise ValueError(error_msg)

                    logging.debug("[SCHEDULE_SERVICE] ✅ Time format validation passed: {}".format(execute_time))

                except (ValueError, IndexError) as e:
                    error_msg = "Time must be in HH:MM:SS format"
                    logging.error("[SCHEDULE_SERVICE] ❌ Time format validation failed: {} - {}".format(error_msg, execute_time))
                    raise ValueError(error_msg)

            logging.info("[SCHEDULE_SERVICE] ✅ All schedule data validation passed for {} schedule".format(frequency_type))
            return True

        except Exception as e:
            logging.error("[SCHEDULE_SERVICE] ❌ Schedule data validation failed for {}".format(frequency_type))
            logging.error("[SCHEDULE_SERVICE] Error type: {}".format(type(e).__name__))
            logging.error("[SCHEDULE_SERVICE] Error message: {}".format(str(e)))
            raise
