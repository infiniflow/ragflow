import sys
import os
import argparse

# Add the project root to sys.path
sys.path.append(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from common import settings
from api.db.services.user_service import UserService


def reset_password(email, new_password):
    # Initialize settings to load database configuration
    settings.init_settings()

    # Query user by email
    users = UserService.query_user_by_email(email)
    if not users:
        print(f"No user found with email: {email}")
        return

    user = users[0]
    # Update password
    UserService.update_user_password(user.id, new_password)
    print(f"Password for user {email} has been successfully reset.")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Reset user password")
    parser.add_argument("email", help="User email")
    parser.add_argument("password", help="New password")
    args = parser.parse_args()

    try:
        reset_password(args.email, args.password)
    except Exception as e:
        print(f"An error occurred: {e}")
        sys.exit(1)
