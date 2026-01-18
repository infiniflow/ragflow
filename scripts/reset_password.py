import sys
import os
import argparse
import getpass

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

    if len(users) > 1:
        print(f"Error: Multiple users found with email '{email}'. Aborting to prevent accidental reset.")
        print("Duplicate users found:")
        for u in users:
            # Try to get attributes safely, defaulting to N/A if missing
            uid = getattr(u, "id", "N/A")
            nickname = getattr(u, "nickname", "N/A")
            create_time = getattr(u, "create_time", "N/A")
            print(f" - ID: {uid}, Nickname: {nickname}, Created: {create_time}")
        sys.exit(1)

    user = users[0]
    # Update password
    UserService.update_user_password(user.id, new_password)
    print(f"Password for user {email} (ID: {user.id}) has been successfully reset.")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Reset user password")
    parser.add_argument("email", help="User email")
    parser.add_argument("--password-stdin", action="store_true", help="Read password from stdin")
    args = parser.parse_args()

    password = None
    if args.password_stdin:
        # Read from stdin (useful for piping)
        password = sys.stdin.read().strip()
    else:
        # Interactive prompt
        try:
            password = getpass.getpass("Enter new password: ")
        except KeyboardInterrupt:
            print("\nOperation cancelled.")
            sys.exit(1)

    if not password:
        print("Error: Password cannot be empty.")
        sys.exit(1)

    try:
        reset_password(args.email, password)
    except Exception as e:
        print(f"An error occurred: {e}")
        sys.exit(1)
