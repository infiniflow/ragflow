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
import re
import time
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
        schedule_info = f"Name:{kwargs.get('name', 'Unknown')}, Canvas:{kwargs.get('canvas_id', 'Unknown')}, Type:{kwargs.get('frequency_type', 'Unknown')}"
        
        logging.info(f"[SCHEDULE_SERVICE] 📅 Creating new schedule - {schedule_info}")
        
        try:
            # Generate cron expression based on frequency type
            logging.debug(f"[SCHEDULE_SERVICE] Generating cron expression for schedule: {schedule_info}")
            cron_expr = cls.generate_cron_expression(**kwargs)
            if cron_expr:
                kwargs['cron_expression'] = cron_expr
                logging.debug(f"[SCHEDULE_SERVICE] Generated cron expression: {cron_expr}")
            else:
                logging.debug(f"[SCHEDULE_SERVICE] No cron expression generated for {kwargs.get('frequency_type')} frequency")
            
            # Validate cron expression
            if kwargs.get('cron_expression'):
                logging.debug(f"[SCHEDULE_SERVICE] Validating cron expression: {kwargs['cron_expression']}")
                if not cls.validate_cron_expression(kwargs['cron_expression']):
                    error_msg = f"Invalid cron expression: {kwargs['cron_expression']}"
                    logging.error(f"[SCHEDULE_SERVICE] ❌ {error_msg}")
                    raise ValueError(error_msg)
                logging.debug(f"[SCHEDULE_SERVICE] ✅ Cron expression validation passed")
            
            # Calculate next run time
            logging.debug(f"[SCHEDULE_SERVICE] Calculating next run time for schedule: {schedule_info}")
            if kwargs.get('cron_expression'):
                next_run = cls.calculate_next_run_time(kwargs['cron_expression'])
                kwargs['next_run_time'] = next_run
                if next_run:
                    next_run_date = datetime.fromtimestamp(next_run)
                    logging.info(f"[SCHEDULE_SERVICE] Next run time calculated: {next_run_date.isoformat()}")
                else:
                    logging.warning(f"[SCHEDULE_SERVICE] ⚠️  Failed to calculate next run time from cron expression")
                    
            elif kwargs.get('frequency_type') == 'once' and kwargs.get('execute_date'):
                # For one-time execution, use the specific date
                execute_datetime = kwargs['execute_date']
                if kwargs.get('execute_time'):
                    time_parts = kwargs['execute_time'].split(':')
                    execute_datetime = execute_datetime.replace(
                        hour=int(time_parts[0]),
                        minute=int(time_parts[1]),
                        second=int(time_parts[2]) if len(time_parts) > 2 else 0
                    )
                kwargs['next_run_time'] = int(execute_datetime.timestamp())
                logging.info(f"[SCHEDULE_SERVICE] One-time execution scheduled for: {execute_datetime.isoformat()}")
            
            # Save the schedule
            logging.debug(f"[SCHEDULE_SERVICE] Saving schedule to database: {schedule_info}")
            result = cls.save(**kwargs)
            
            if result:
                logging.info(f"[SCHEDULE_SERVICE] ✅ Successfully created schedule - ID:{result.id}, {schedule_info}")
                logging.debug(f"[SCHEDULE_SERVICE] Schedule details - Enabled:{result.enabled}, Next run:{result.next_run_time}")
            else:
                logging.error(f"[SCHEDULE_SERVICE] ❌ Failed to save schedule to database: {schedule_info}")
                
            return result
            
        except Exception as e:
            logging.error(f"[SCHEDULE_SERVICE] ❌ Error creating schedule: {schedule_info}")
            logging.error(f"[SCHEDULE_SERVICE] Error type: {type(e).__name__}")
            logging.error(f"[SCHEDULE_SERVICE] Error message: {str(e)}")
            logging.exception(f"[SCHEDULE_SERVICE] Exception details: {e}")
            raise

    @classmethod
    def get_schedules_paginated(cls, created_by=None, canvas_id=None, keywords='', page=1, page_size=20):
        """Get schedules with pagination and filtering with detailed logging"""
        logging.debug(f"[SCHEDULE_SERVICE] 📋 Fetching schedules - User:{created_by}, Canvas:{canvas_id}, Keywords:'{keywords}', Page:{page}, Size:{page_size}")
        
        try:
            from peewee import fn
            
            # Build query conditions
            conditions = []
            
            if created_by:
                conditions.append(cls.model.created_by == created_by)
                logging.debug(f"[SCHEDULE_SERVICE] Added user filter: {created_by}")
            
            if canvas_id:
                conditions.append(cls.model.canvas_id == canvas_id)
                logging.debug(f"[SCHEDULE_SERVICE] Added canvas filter: {canvas_id}")
            
            if keywords:
                conditions.append(
                    (cls.model.name.contains(keywords)) |
                    (cls.model.description.contains(keywords))
                )
                logging.debug(f"[SCHEDULE_SERVICE] Added keywords filter: '{keywords}'")
            
            # Add status filter (only valid schedules)
            conditions.append(cls.model.status == "1")
            
            # Build base query
            query = cls.model.select()
            if conditions:
                query = query.where(*conditions)
            
            # Get total count
            total = query.count()
            logging.debug(f"[SCHEDULE_SERVICE] Total matching schedules: {total}")
            
            # Apply pagination and ordering
            schedules = (query
                        .order_by(cls.model.create_time.desc())
                        .paginate(page, page_size))
            
            schedule_list = list(schedules)
            logging.info(f"[SCHEDULE_SERVICE] ✅ Retrieved {len(schedule_list)} schedules (page {page}/{((total-1)//page_size)+1})")
            
            return schedule_list, total
            
        except Exception as e:
            logging.error(f"[SCHEDULE_SERVICE] ❌ Error fetching schedules - User:{created_by}, Canvas:{canvas_id}")
            logging.error(f"[SCHEDULE_SERVICE] Error type: {type(e).__name__}")
            logging.error(f"[SCHEDULE_SERVICE] Error message: {str(e)}")
            logging.exception(f"[SCHEDULE_SERVICE] Exception details: {e}")
            raise

    @classmethod
    def get_pending_schedules(cls, tolerance_minutes=5):
        """Get schedules that are ready to run with detailed logging and specific logic for each frequency type"""
        logging.debug(f"[SCHEDULE_SERVICE] 🔍 Checking for pending schedules (tolerance: {tolerance_minutes} minutes)")
        
        try:
            current_time = current_timestamp()
            current_datetime = datetime.now()
            
            logging.debug(f"[SCHEDULE_SERVICE] Current timestamp: {current_time} ({current_datetime.isoformat()})")
            
            # Get all enabled schedules
            all_schedules: List[ScheduleAgent] = cls.query(
                enabled=True,
                status="1"
            )
            
            logging.debug(f"[SCHEDULE_SERVICE] Found {len(all_schedules)} enabled schedules to check")
            
            valid_schedules = []
            disabled_count = 0
            
            for schedule in all_schedules:
                schedule_info = f"ID:{schedule.id}, Name:{schedule.name}, Type:{schedule.frequency_type}"
                
                try:
                    # Validate timestamps
                    if schedule.next_run_time and (schedule.next_run_time < 0 or schedule.next_run_time > 2147483647):
                        logging.warning(f"[SCHEDULE_SERVICE] ⚠️  Invalid next_run_time for {schedule_info}: {schedule.next_run_time}")
                        cls.update_by_id(schedule.id, {'next_run_time': None})
                        continue
                    
                    should_run = False
                    
                    # Check each frequency type with specific conditions
                    if schedule.frequency_type == 'once':
                        # Once: current time > execute_date + execute_time AND run_count = 0
                        if schedule.run_count == 0 and schedule.execute_date and schedule.execute_time:
                            execute_datetime = schedule.execute_date
                            time_parts = schedule.execute_time.split(':')
                            execute_datetime = execute_datetime.replace(
                                hour=int(time_parts[0]),
                                minute=int(time_parts[1]),
                                second=int(time_parts[2]) if len(time_parts) > 2 else 0,
                                microsecond=0
                            )
                            
                            if current_datetime >= execute_datetime:
                                should_run = True
                                logging.debug(f"[SCHEDULE_SERVICE] ✅ Once schedule ready: {schedule_info}")
                            else:
                                logging.debug(f"[SCHEDULE_SERVICE] Once schedule not ready: {schedule_info} - Execute at: {execute_datetime.isoformat()}")
                        
                        elif schedule.run_count > 0:
                            # Disable completed one-time schedules
                            logging.info(f"[SCHEDULE_SERVICE] 🔒 Disabling completed one-time schedule: {schedule_info}")
                            cls.update_by_id(schedule.id, {'enabled': False})
                            disabled_count += 1
                            continue
                    
                    elif schedule.frequency_type == 'daily':
                        # Daily: current time > execute_time today
                        if schedule.execute_time:
                            time_parts = schedule.execute_time.split(':')
                            today_execute_time = current_datetime.replace(
                                hour=int(time_parts[0]),
                                minute=int(time_parts[1]),
                                second=int(time_parts[2]) if len(time_parts) > 2 else 0,
                                microsecond=0
                            )
                            
                            # Check if we've passed today's execution time
                            if current_datetime >= today_execute_time:
                                # Check if we haven't run today yet
                                if schedule.last_run_time:
                                    last_run_date = datetime.fromtimestamp(schedule.last_run_time).date()
                                    if last_run_date < current_datetime.date():
                                        should_run = True
                                        logging.debug(f"[SCHEDULE_SERVICE] ✅ Daily schedule ready: {schedule_info}")
                                    else:
                                        logging.debug(f"[SCHEDULE_SERVICE] Daily schedule already ran today: {schedule_info}")
                                else:
                                    # Never run before
                                    should_run = True
                                    logging.debug(f"[SCHEDULE_SERVICE] ✅ Daily schedule ready (first run): {schedule_info}")
                            else:
                                logging.debug(f"[SCHEDULE_SERVICE] Daily schedule not ready: {schedule_info} - Execute at: {today_execute_time.strftime('%H:%M:%S')}")
                    
                    elif schedule.frequency_type == 'weekly':
                        # Weekly: current time > execute_time AND current day in days_of_week
                        if schedule.execute_time and schedule.days_of_week:
                            current_weekday = current_datetime.weekday() + 1  # Convert to 1=Monday format
                            
                            if current_weekday in schedule.days_of_week:
                                time_parts = schedule.execute_time.split(':')
                                today_execute_time = current_datetime.replace(
                                    hour=int(time_parts[0]),
                                    minute=int(time_parts[1]),
                                    second=int(time_parts[2]) if len(time_parts) > 2 else 0,
                                    microsecond=0
                                )
                                
                                if current_datetime >= today_execute_time:
                                    # Check if we haven't run today yet
                                    if schedule.last_run_time:
                                        last_run_date = datetime.fromtimestamp(schedule.last_run_time).date()
                                        if last_run_date < current_datetime.date():
                                            should_run = True
                                            logging.debug(f"[SCHEDULE_SERVICE] ✅ Weekly schedule ready: {schedule_info}")
                                        else:
                                            logging.debug(f"[SCHEDULE_SERVICE] Weekly schedule already ran today: {schedule_info}")
                                    else:
                                        # Never run before
                                        should_run = True
                                        logging.debug(f"[SCHEDULE_SERVICE] ✅ Weekly schedule ready (first run): {schedule_info}")
                                else:
                                    logging.debug(f"[SCHEDULE_SERVICE] Weekly schedule not ready: {schedule_info} - Execute at: {today_execute_time.strftime('%H:%M:%S')}")
                            else:
                                logging.debug(f"[SCHEDULE_SERVICE] Weekly schedule not for today: {schedule_info} - Current day: {current_weekday}, Target days: {schedule.days_of_week}")
                    
                    elif schedule.frequency_type == 'monthly':
                        # Monthly: current time > execute_time AND current day = day_of_month
                        if schedule.execute_time and schedule.day_of_month:
                            current_day = current_datetime.day
                            
                            if current_day == schedule.day_of_month:
                                time_parts = schedule.execute_time.split(':')
                                today_execute_time = current_datetime.replace(
                                    hour=int(time_parts[0]),
                                    minute=int(time_parts[1]),
                                    second=int(time_parts[2]) if len(time_parts) > 2 else 0,
                                    microsecond=0
                                )
                                
                                if current_datetime >= today_execute_time:
                                    # Check if we haven't run this month yet
                                    if schedule.last_run_time:
                                        last_run_datetime = datetime.fromtimestamp(schedule.last_run_time)
                                        if (last_run_datetime.year < current_datetime.year or 
                                            last_run_datetime.month < current_datetime.month):
                                            should_run = True
                                            logging.debug(f"[SCHEDULE_SERVICE] ✅ Monthly schedule ready: {schedule_info}")
                                        else:
                                            logging.debug(f"[SCHEDULE_SERVICE] Monthly schedule already ran this month: {schedule_info}")
                                    else:
                                        # Never run before
                                        should_run = True
                                        logging.debug(f"[SCHEDULE_SERVICE] ✅ Monthly schedule ready (first run): {schedule_info}")
                                else:
                                    logging.debug(f"[SCHEDULE_SERVICE] Monthly schedule not ready: {schedule_info} - Execute at: {today_execute_time.strftime('%H:%M:%S')}")
                            else:
                                logging.debug(f"[SCHEDULE_SERVICE] Monthly schedule not for today: {schedule_info} - Current day: {current_day}, Target day: {schedule.day_of_month}")
                    
                    if should_run:
                        # Log schedule details
                        try:
                            last_run_str = datetime.fromtimestamp(schedule.last_run_time).isoformat() if schedule.last_run_time else "Never"
                        except (ValueError, OSError):
                            last_run_str = "Invalid"
                        
                        logging.debug(f"[SCHEDULE_SERVICE] ✅ Valid pending schedule: {schedule_info}")
                        logging.debug(f"[SCHEDULE_SERVICE]   Last run: {last_run_str}, Run count: {schedule.run_count}")
                        
                        valid_schedules.append(schedule)
                    
                except Exception as schedule_error:
                    logging.error(f"[SCHEDULE_SERVICE] ❌ Error processing schedule {schedule_info}: {schedule_error}")
                    continue
            
            if disabled_count > 0:
                logging.info(f"[SCHEDULE_SERVICE] Disabled {disabled_count} completed one-time schedules")
            
            logging.info(f"[SCHEDULE_SERVICE] ✅ Found {len(valid_schedules)} valid pending schedules")
            
            return valid_schedules
            
        except Exception as e:
            logging.error(f"[SCHEDULE_SERVICE] ❌ Error getting pending schedules")
            logging.error(f"[SCHEDULE_SERVICE] Error type: {type(e).__name__}")
            logging.error(f"[SCHEDULE_SERVICE] Error message: {str(e)}")
            logging.exception(f"[SCHEDULE_SERVICE] Exception details: {e}")
            return []

    @classmethod
    def update_after_run(cls, schedule_id, success=True):
        """Update schedule after execution with detailed logging"""
        status_str = "✅ SUCCESS" if success else "❌ FAILED"
        logging.info(f"[SCHEDULE_SERVICE] 🔄 Updating schedule {schedule_id} after execution - Status: {status_str}")
        
        try:
            e, schedule_obj = cls.get_by_id(schedule_id)
            if not e:
                logging.error(f"[SCHEDULE_SERVICE] ❌ Schedule {schedule_id} not found for update")
                return False
            
            schedule_info = f"ID:{schedule_obj.id}, Name:{schedule_obj.name}, Type:{schedule_obj.frequency_type}"
            current_time = current_timestamp()
            
            update_data = {
                'last_run_time': current_time,
                'run_count': schedule_obj.run_count + 1
            }
            
            logging.debug(f"[SCHEDULE_SERVICE] Updating run statistics for {schedule_info} - Run count: {schedule_obj.run_count} -> {update_data['run_count']}")
            
            # Calculate next run time based on frequency type
            if schedule_obj.frequency_type == 'once':
                # Disable one-time schedules after execution
                update_data['enabled'] = False
                update_data['next_run_time'] = None
                logging.info(f"[SCHEDULE_SERVICE] 🔒 Disabling one-time schedule after execution: {schedule_info}")
                
            elif schedule_obj.frequency_type == 'weekly':
                # For weekly schedules, calculate next run time for next week
                logging.debug(f"[SCHEDULE_SERVICE] Calculating next weekly run time for {schedule_info}")
                
                if schedule_obj.cron_expression:
                    next_run = cls.calculate_next_run_time(schedule_obj.cron_expression)
                    update_data['next_run_time'] = next_run
                    if next_run:
                        next_run_date = datetime.fromtimestamp(next_run)
                        logging.info(f"[SCHEDULE_SERVICE] Next weekly run scheduled for: {next_run_date.isoformat()}")
                    else:
                        logging.warning(f"[SCHEDULE_SERVICE] ⚠️  Failed to calculate next weekly run time using cron")
                else:
                    # Fallback: calculate next week manually
                    logging.debug(f"[SCHEDULE_SERVICE] Using manual calculation for weekly schedule {schedule_info}")
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
                            time_parts = schedule_obj.execute_time.split(':')
                            next_run_date = next_run_date.replace(
                                hour=int(time_parts[0]),
                                minute=int(time_parts[1]),
                                second=int(time_parts[2]) if len(time_parts) > 2 else 0,
                                microsecond=0
                            )
                        
                        update_data['next_run_time'] = int(next_run_date.timestamp())
                        logging.info(f"[SCHEDULE_SERVICE] Next weekly run manually calculated for: {next_run_date.isoformat()}")
                        
            elif schedule_obj.cron_expression:
                # Use cron expression for other recurring schedules
                logging.debug(f"[SCHEDULE_SERVICE] Calculating next run time using cron for {schedule_info}")
                next_run = cls.calculate_next_run_time(schedule_obj.cron_expression)
                update_data['next_run_time'] = next_run
                if next_run:
                    next_run_date = datetime.fromtimestamp(next_run)
                    logging.info(f"[SCHEDULE_SERVICE] Next run scheduled for: {next_run_date.isoformat()}")
                else:
                    logging.warning(f"[SCHEDULE_SERVICE] ⚠️  Failed to calculate next run time using cron")
            
            # Update the schedule
            logging.debug(f"[SCHEDULE_SERVICE] Saving updates to database for {schedule_info}")
            result = cls.update_by_id(schedule_id, update_data)
            
            if result:
                logging.info(f"[SCHEDULE_SERVICE] ✅ Successfully updated schedule {schedule_info} after execution")
                if update_data.get('next_run_time'):
                    next_run_str = datetime.fromtimestamp(update_data['next_run_time']).isoformat()
                    logging.debug(f"[SCHEDULE_SERVICE] Next execution: {next_run_str}")
            else:
                logging.error(f"[SCHEDULE_SERVICE] ❌ Failed to update schedule {schedule_info} in database")
                
            return result
            
        except Exception as e:
            logging.error(f"[SCHEDULE_SERVICE] ❌ Error updating schedule {schedule_id} after execution")
            logging.error(f"[SCHEDULE_SERVICE] Error type: {type(e).__name__}")
            logging.error(f"[SCHEDULE_SERVICE] Error message: {str(e)}")
            logging.exception(f"[SCHEDULE_SERVICE] Exception details: {e}")
            return False

    @classmethod
    def generate_cron_expression(cls, **kwargs):
        """Generate cron expression based on frequency type and settings with logging"""
        frequency_type = kwargs.get('frequency_type', 'once')
        execute_time = kwargs.get('execute_time', '00:00:00')
        
        logging.debug(f"[SCHEDULE_SERVICE] 🕐 Generating cron expression - Type: {frequency_type}, Time: {execute_time}")
        
        try:
            # Parse time
            time_parts = execute_time.split(':')
            hour = int(time_parts[0]) if len(time_parts) > 0 else 0
            minute = int(time_parts[1]) if len(time_parts) > 1 else 0
            second = int(time_parts[2]) if len(time_parts) > 2 else 0
            
            logging.debug(f"[SCHEDULE_SERVICE] Parsed time - Hour: {hour}, Minute: {minute}, Second: {second}")
            
            if frequency_type == 'once':
                # For one-time execution, we'll handle this separately
                logging.debug(f"[SCHEDULE_SERVICE] One-time execution - no cron expression needed")
                return None
            
            elif frequency_type == 'daily':
                # Daily at specific time: "second minute hour * * *"
                cron_expr = f"{second} {minute} {hour} * * *"
                logging.debug(f"[SCHEDULE_SERVICE] Generated daily cron: {cron_expr}")
                return cron_expr
            
            elif frequency_type == 'weekly':
                # Weekly on specific days: "second minute hour * * day1,day2,..."
                days_of_week = kwargs.get('days_of_week', [1])  # Default to Monday
                if not days_of_week:
                    days_of_week = [1]
                
                logging.debug(f"[SCHEDULE_SERVICE] Weekly days: {days_of_week}")
                
                # Convert from our format (1=Monday) to cron format (1=Monday, 7=Sunday)
                cron_days = []
                for day in days_of_week:
                    if day == 7:  # Sunday
                        cron_days.append('0')
                    else:
                        cron_days.append(str(day))
                
                days_str = ','.join(cron_days)
                cron_expr = f"{second} {minute} {hour} * * {days_str}"
                logging.debug(f"[SCHEDULE_SERVICE] Generated weekly cron: {cron_expr}")
                return cron_expr
            
            elif frequency_type == 'monthly':
                # Monthly on specific day: "second minute hour day * *"
                day_of_month = kwargs.get('day_of_month', 1)
                cron_expr = f"{second} {minute} {hour} {day_of_month} * *"
                logging.debug(f"[SCHEDULE_SERVICE] Generated monthly cron: {cron_expr}")
                return cron_expr
            
            logging.warning(f"[SCHEDULE_SERVICE] ⚠️  Unknown frequency type: {frequency_type}")
            return None
            
        except Exception as e:
            logging.error(f"[SCHEDULE_SERVICE] ❌ Error generating cron expression for {frequency_type}")
            logging.error(f"[SCHEDULE_SERVICE] Error type: {type(e).__name__}")
            logging.error(f"[SCHEDULE_SERVICE] Error message: {str(e)}")
            logging.exception(f"[SCHEDULE_SERVICE] Exception details: {e}")
            return None

    @classmethod
    def validate_cron_expression(cls, cron_expr):
        """Validate cron expression with logging"""
        logging.debug(f"[SCHEDULE_SERVICE] 🔍 Validating cron expression: {cron_expr}")
        
        try:
            croniter(cron_expr)
            logging.debug(f"[SCHEDULE_SERVICE] ✅ Cron expression validation passed")
            return True
        except Exception as e:
            logging.warning(f"[SCHEDULE_SERVICE] ❌ Invalid cron expression '{cron_expr}': {str(e)}")
            return False

    @classmethod
    def calculate_next_run_time(cls, cron_expr):
        """Calculate next run time from cron expression with logging"""
        logging.debug(f"[SCHEDULE_SERVICE] ⏰ Calculating next run time from cron: {cron_expr}")
        
        try:
            cron = croniter(cron_expr, datetime.now())
            next_time = cron.get_next(datetime)
            next_timestamp = int(next_time.timestamp())
            
            logging.debug(f"[SCHEDULE_SERVICE] ✅ Next run time calculated: {next_time.isoformat()} (timestamp: {next_timestamp})")
            return next_timestamp
        except Exception as e:
            logging.error(f"[SCHEDULE_SERVICE] ❌ Error calculating next run time from cron '{cron_expr}': {str(e)}")
            return None

    @classmethod
    def validate_schedule_data(cls, **kwargs):
        """Validate schedule data based on frequency type with detailed logging"""
        frequency_type = kwargs.get('frequency_type', 'once')
        
        logging.debug(f"[SCHEDULE_SERVICE] 📝 Validating schedule data for frequency type: {frequency_type}")
        
        try:
            if frequency_type == 'once':
                if not kwargs.get('execute_date'):
                    error_msg = "Execute date is required for one-time schedules"
                    logging.error(f"[SCHEDULE_SERVICE] ❌ Validation failed: {error_msg}")
                    raise ValueError(error_msg)
                logging.debug(f"[SCHEDULE_SERVICE] ✅ One-time schedule validation passed")
            
            elif frequency_type == 'weekly':
                days_of_week = kwargs.get('days_of_week', [])
                if not days_of_week or not all(1 <= day <= 7 for day in days_of_week):
                    error_msg = "Valid days of week (1-7) are required for weekly schedules"
                    logging.error(f"[SCHEDULE_SERVICE] ❌ Validation failed: {error_msg} - Provided: {days_of_week}")
                    raise ValueError(error_msg)
                logging.debug(f"[SCHEDULE_SERVICE] ✅ Weekly schedule validation passed - Days: {days_of_week}")
            
            elif frequency_type == 'monthly':
                day_of_month = kwargs.get('day_of_month')
                if not day_of_month or not (1 <= day_of_month <= 31):
                    error_msg = "Valid day of month (1-31) is required for monthly schedules"
                    logging.error(f"[SCHEDULE_SERVICE] ❌ Validation failed: {error_msg} - Provided: {day_of_month}")
                    raise ValueError(error_msg)
                logging.debug(f"[SCHEDULE_SERVICE] ✅ Monthly schedule validation passed - Day: {day_of_month}")
            
            # Validate time format
            execute_time = kwargs.get('execute_time', '00:00:00')
            if execute_time:
                try:
                    time_parts = execute_time.split(':')
                    hour = int(time_parts[0])
                    minute = int(time_parts[1])
                    second = int(time_parts[2]) if len(time_parts) > 2 else 0
                    
                    if not (0 <= hour <= 23 and 0 <= minute <= 59 and 0 <= second <= 59):
                        error_msg = "Invalid time format - hour, minute, second out of range"
                        logging.error(f"[SCHEDULE_SERVICE] ❌ Time validation failed: {error_msg} - {execute_time}")
                        raise ValueError(error_msg)
                        
                    logging.debug(f"[SCHEDULE_SERVICE] ✅ Time format validation passed: {execute_time}")
                    
                except (ValueError, IndexError) as e:
                    error_msg = "Time must be in HH:MM:SS format"
                    logging.error(f"[SCHEDULE_SERVICE] ❌ Time format validation failed: {error_msg} - {execute_time}")
                    raise ValueError(error_msg)
            
            logging.info(f"[SCHEDULE_SERVICE] ✅ All schedule data validation passed for {frequency_type} schedule")
            return True
            
        except Exception as e:
            logging.error(f"[SCHEDULE_SERVICE] ❌ Schedule data validation failed for {frequency_type}")
            logging.error(f"[SCHEDULE_SERVICE] Error type: {type(e).__name__}")
            logging.error(f"[SCHEDULE_SERVICE] Error message: {str(e)}")
            raise
