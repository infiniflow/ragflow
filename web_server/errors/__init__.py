from .general_error import *


class FateFlowError(Exception):
    message = 'Unknown Fate Flow Error'

    def __init__(self, message=None, *args, **kwargs):
        message = str(message) if message is not None else self.message
        message = message.format(*args, **kwargs)
        super().__init__(message)