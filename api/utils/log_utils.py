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
import os
import typing
import traceback
import logging
import inspect
from logging.handlers import TimedRotatingFileHandler
from threading import RLock

from api.utils import file_utils


class LoggerFactory(object):
    TYPE = "FILE"
    LOG_FORMAT = "[%(levelname)s] [%(asctime)s] [jobId] [%(process)s:%(thread)s] - [%(module)s.%(funcName)s] [line:%(lineno)d]: %(message)s"
    LEVEL = logging.DEBUG
    logger_dict = {}
    global_handler_dict = {}

    LOG_DIR = None
    PARENT_LOG_DIR = None
    log_share = True

    append_to_parent_log = None

    lock = RLock()
    # CRITICAL = 50
    # FATAL = CRITICAL
    # ERROR = 40
    # WARNING = 30
    # WARN = WARNING
    # INFO = 20
    # DEBUG = 10
    # NOTSET = 0
    levels = (10, 20, 30, 40)
    schedule_logger_dict = {}

    @staticmethod
    def set_directory(directory=None, parent_log_dir=None,
                      append_to_parent_log=None, force=False):
        if parent_log_dir:
            LoggerFactory.PARENT_LOG_DIR = parent_log_dir
        if append_to_parent_log:
            LoggerFactory.append_to_parent_log = append_to_parent_log
        with LoggerFactory.lock:
            if not directory:
                directory = file_utils.get_project_base_directory("logs")
            if not LoggerFactory.LOG_DIR or force:
                LoggerFactory.LOG_DIR = directory
            if LoggerFactory.log_share:
                oldmask = os.umask(000)
                os.makedirs(LoggerFactory.LOG_DIR, exist_ok=True)
                os.umask(oldmask)
            else:
                os.makedirs(LoggerFactory.LOG_DIR, exist_ok=True)
            for loggerName, ghandler in LoggerFactory.global_handler_dict.items():
                for className, (logger,
                                handler) in LoggerFactory.logger_dict.items():
                    logger.removeHandler(ghandler)
                ghandler.close()
            LoggerFactory.global_handler_dict = {}
            for className, (logger,
                            handler) in LoggerFactory.logger_dict.items():
                logger.removeHandler(handler)
                _handler = None
                if handler:
                    handler.close()
                if className != "default":
                    _handler = LoggerFactory.get_handler(className)
                    logger.addHandler(_handler)
                LoggerFactory.assemble_global_handler(logger)
                LoggerFactory.logger_dict[className] = logger, _handler

    @staticmethod
    def new_logger(name):
        logger = logging.getLogger(name)
        logger.propagate = False
        logger.setLevel(LoggerFactory.LEVEL)
        return logger

    @staticmethod
    def get_logger(class_name=None):
        with LoggerFactory.lock:
            if class_name in LoggerFactory.logger_dict.keys():
                logger, handler = LoggerFactory.logger_dict[class_name]
                if not logger:
                    logger, handler = LoggerFactory.init_logger(class_name)
            else:
                logger, handler = LoggerFactory.init_logger(class_name)
            return logger

    @staticmethod
    def get_global_handler(logger_name, level=None, log_dir=None):
        if not LoggerFactory.LOG_DIR:
            return logging.StreamHandler()
        if log_dir:
            logger_name_key = logger_name + "_" + log_dir
        else:
            logger_name_key = logger_name + "_" + LoggerFactory.LOG_DIR
        # if loggerName not in LoggerFactory.globalHandlerDict:
        if logger_name_key not in LoggerFactory.global_handler_dict:
            with LoggerFactory.lock:
                if logger_name_key not in LoggerFactory.global_handler_dict:
                    handler = LoggerFactory.get_handler(
                        logger_name, level, log_dir)
                    LoggerFactory.global_handler_dict[logger_name_key] = handler
        return LoggerFactory.global_handler_dict[logger_name_key]

    @staticmethod
    def get_handler(class_name, level=None, log_dir=None,
                    log_type=None, job_id=None):
        if not log_type:
            if not LoggerFactory.LOG_DIR or not class_name:
                return logging.StreamHandler()
                # return Diy_StreamHandler()

            if not log_dir:
                log_file = os.path.join(
                    LoggerFactory.LOG_DIR,
                    "{}.log".format(class_name))
            else:
                log_file = os.path.join(log_dir, "{}.log".format(class_name))
        else:
            log_file = os.path.join(log_dir, "rag_flow_{}.log".format(
                log_type) if level == LoggerFactory.LEVEL else 'rag_flow_{}_error.log'.format(log_type))

        os.makedirs(os.path.dirname(log_file), exist_ok=True)
        if LoggerFactory.log_share:
            handler = ROpenHandler(log_file,
                                   when='D',
                                   interval=1,
                                   backupCount=14,
                                   delay=True)
        else:
            handler = TimedRotatingFileHandler(log_file,
                                               when='D',
                                               interval=1,
                                               backupCount=14,
                                               delay=True)
        if level:
            handler.level = level

        return handler

    @staticmethod
    def init_logger(class_name):
        with LoggerFactory.lock:
            logger = LoggerFactory.new_logger(class_name)
            handler = None
            if class_name:
                handler = LoggerFactory.get_handler(class_name)
                logger.addHandler(handler)
                LoggerFactory.logger_dict[class_name] = logger, handler

            else:
                LoggerFactory.logger_dict["default"] = logger, handler

            LoggerFactory.assemble_global_handler(logger)
            return logger, handler

    @staticmethod
    def assemble_global_handler(logger):
        if LoggerFactory.LOG_DIR:
            for level in LoggerFactory.levels:
                if level >= LoggerFactory.LEVEL:
                    level_logger_name = logging._levelToName[level]
                    logger.addHandler(
                        LoggerFactory.get_global_handler(
                            level_logger_name, level))
        if LoggerFactory.append_to_parent_log and LoggerFactory.PARENT_LOG_DIR:
            for level in LoggerFactory.levels:
                if level >= LoggerFactory.LEVEL:
                    level_logger_name = logging._levelToName[level]
                    logger.addHandler(
                        LoggerFactory.get_global_handler(level_logger_name, level, LoggerFactory.PARENT_LOG_DIR))


def setDirectory(directory=None):
    LoggerFactory.set_directory(directory)


def setLevel(level):
    LoggerFactory.LEVEL = level


def getLogger(className=None, useLevelFile=False):
    if className is None:
        frame = inspect.stack()[1]
        module = inspect.getmodule(frame[0])
        className = 'stat'
    return LoggerFactory.get_logger(className)


def exception_to_trace_string(ex):
    return "".join(traceback.TracebackException.from_exception(ex).format())


class ROpenHandler(TimedRotatingFileHandler):
    def _open(self):
        prevumask = os.umask(000)
        rtv = TimedRotatingFileHandler._open(self)
        os.umask(prevumask)
        return rtv


def sql_logger(job_id='', log_type='sql'):
    key = job_id + log_type
    if key in LoggerFactory.schedule_logger_dict.keys():
        return LoggerFactory.schedule_logger_dict[key]
    return get_job_logger(job_id=job_id, log_type=log_type)


def ready_log(msg, job=None, task=None, role=None, party_id=None, detail=None):
    prefix, suffix = base_msg(job, task, role, party_id, detail)
    return f"{prefix}{msg} ready{suffix}"


def start_log(msg, job=None, task=None, role=None, party_id=None, detail=None):
    prefix, suffix = base_msg(job, task, role, party_id, detail)
    return f"{prefix}start to {msg}{suffix}"


def successful_log(msg, job=None, task=None, role=None,
                   party_id=None, detail=None):
    prefix, suffix = base_msg(job, task, role, party_id, detail)
    return f"{prefix}{msg} successfully{suffix}"


def warning_log(msg, job=None, task=None, role=None,
                party_id=None, detail=None):
    prefix, suffix = base_msg(job, task, role, party_id, detail)
    return f"{prefix}{msg} is not effective{suffix}"


def failed_log(msg, job=None, task=None, role=None,
               party_id=None, detail=None):
    prefix, suffix = base_msg(job, task, role, party_id, detail)
    return f"{prefix}failed to {msg}{suffix}"


def base_msg(job=None, task=None, role: str = None,
             party_id: typing.Union[str, int] = None, detail=None):
    if detail:
        detail_msg = f" detail: \n{detail}"
    else:
        detail_msg = ""
    if task is not None:
        return f"task {task.f_task_id} {task.f_task_version} ", f" on {task.f_role} {task.f_party_id}{detail_msg}"
    elif job is not None:
        return "", f" on {job.f_role} {job.f_party_id}{detail_msg}"
    elif role and party_id:
        return "", f" on {role} {party_id}{detail_msg}"
    else:
        return "", f"{detail_msg}"


def exception_to_trace_string(ex):
    return "".join(traceback.TracebackException.from_exception(ex).format())


def get_logger_base_dir():
    job_log_dir = file_utils.get_rag_flow_directory('logs')
    return job_log_dir


def get_job_logger(job_id, log_type):
    rag_flow_log_dir = file_utils.get_rag_flow_directory('logs', 'rag_flow')
    job_log_dir = file_utils.get_rag_flow_directory('logs', job_id)
    if not job_id:
        log_dirs = [rag_flow_log_dir]
    else:
        if log_type == 'audit':
            log_dirs = [job_log_dir, rag_flow_log_dir]
        else:
            log_dirs = [job_log_dir]
    if LoggerFactory.log_share:
        oldmask = os.umask(000)
        os.makedirs(job_log_dir, exist_ok=True)
        os.makedirs(rag_flow_log_dir, exist_ok=True)
        os.umask(oldmask)
    else:
        os.makedirs(job_log_dir, exist_ok=True)
        os.makedirs(rag_flow_log_dir, exist_ok=True)
    logger = LoggerFactory.new_logger(f"{job_id}_{log_type}")
    for job_log_dir in log_dirs:
        handler = LoggerFactory.get_handler(class_name=None, level=LoggerFactory.LEVEL,
                                            log_dir=job_log_dir, log_type=log_type, job_id=job_id)
        error_handler = LoggerFactory.get_handler(
            class_name=None,
            level=logging.ERROR,
            log_dir=job_log_dir,
            log_type=log_type,
            job_id=job_id)
        logger.addHandler(handler)
        logger.addHandler(error_handler)
    with LoggerFactory.lock:
        LoggerFactory.schedule_logger_dict[job_id + log_type] = logger
    return logger
