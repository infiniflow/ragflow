class AdminException(Exception):
    def __init__(self, message, code=400):
        super().__init__(message)
        self.code = code
        self.message = message

class UserNotFoundError(AdminException):
    def __init__(self, username):
        super().__init__(f"User '{username}' not found", 404)

class UserAlreadyExistsError(AdminException):
    def __init__(self, username):
        super().__init__(f"User '{username}' already exists", 409)

class CannotDeleteAdminError(AdminException):
    def __init__(self):
        super().__init__("Cannot delete admin account", 403)