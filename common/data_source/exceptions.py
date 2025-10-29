"""Exception class definitions"""


class ConnectorMissingCredentialError(Exception):
    """Missing credentials exception"""
    def __init__(self, connector_name: str):
        super().__init__(f"Missing credentials for {connector_name}")


class ConnectorValidationError(Exception):
    """Connector validation exception"""
    pass


class CredentialExpiredError(Exception):
    """Credential expired exception"""
    pass


class InsufficientPermissionsError(Exception):
    """Insufficient permissions exception"""
    pass


class UnexpectedValidationError(Exception):
    """Unexpected validation exception"""
    pass

class RateLimitTriedTooManyTimesError(Exception):
    pass