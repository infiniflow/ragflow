---
sidebar_position: 3
sidebar_label: Manage User Accounts
title: Manage User Accounts
---

# Manage User Accounts

## View and Search Users

User accounts are managed on the **User management** page. Administrators can view the user list, including `Email`, `Nickname`, `Status`, `User type`, and `Last login time`.

![User Management](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/user_management_1.jpg)

![User Management](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/user_management_2.jpg)

![User Management](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/user_management_3.jpg)

Use the search box in the upper-right corner to search users by `Email` or `Nickname`. You can also filter `Active` or `Inactive` users by `Status`.

![View And Search Users](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/view_and_search_users_1.jpg)

![View And Search Users](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/view_and_search_users_2.jpg)

## Create a New User

To create an account for a new member, go to **User management** and click **New user**. In the dialog, fill in `Email`, `Password`, and `Confirm password`.

1. Go to the **User management** page.
2. Click **New user**.
3. Enter the user's `Email`.
4. Enter the initial password.
5. Enter the same password again in `Confirm password`.
6. Click **Confirm** to create the account.

![Create A New User](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/create_a_new_user_1.jpg)

![Create A New User](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/create_a_new_user_2.jpg)

After creation, return to the user list and confirm that the account appears and that its status matches expectations. Then send the login information to the user through a secure channel.

**Caution:** `Email` must use a valid email format. After email verification is enabled, users must complete email verification before they can use the account normally. If email verification is not enabled, the system only validates the email format.

When creating a new user, the password and confirmation password must match. The frontend validation for the current create-user form requires at least 6 characters. Password reset requires a new password of at least 8 characters. In production environments, use a stronger unified password policy.

## Disable or Restore an Account

The `Status` field in the user list controls whether an account can log in to the system.

| Status | Meaning |
| --- | --- |
| `Active` | The account is enabled and can log in and use the system normally. |
| `Inactive` | The account is disabled. The user cannot log in, but the account and related data are retained. |

1. Go to the **User management** page and find the target user in the user list.
2. In the `Status` column of that user's row, click the current status dropdown and select `Active` or `Inactive` as needed.

![Disable Or Restore Accounts](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/disable_or_restore_accounts.jpg)

**Caution:** The currently logged-in administrator cannot disable their own account. Disabling an account does not delete user data; it only prevents the user from logging in. If you need to permanently remove a user, confirm that the account no longer needs to be retained before deleting it.

## Set Backend Administrator Identity

`User type` distinguishes `Normal` from `Superuser`. `Normal` is a regular user type. `Superuser` can enter the Admin UI and perform system-level management operations.

1. Find the target user on the **User management** page.
2. Open the selector in the `User type` column.
3. Select `Normal` or `Superuser`.
4. Wait for the system to submit the change and refresh the user list.

![Set Backend Administrator Identity](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/set_backend_administrator_identity.jpg)

**Caution:** The currently logged-in administrator cannot modify their own `Superuser` type in the list.

## Reset User Passwords

When a user forgets their password or must be forced to change it, administrators can reset another user's password from **Actions** in the user list. After the reset-password dialog opens, the page displays the target user's `Email` and requires `New password` and `Confirm new password`.

1. Find the target user on the **User management** page. Hover over the user row to display **Actions**, then click the reset-password button.
2. Confirm that the `Email` in the dialog is the target user.
3. Enter the new password and confirm it again.
4. Click **Change password**.

![Reset User Passwords](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/reset_user_passwords_1.jpg)

![Reset User Passwords](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/reset_user_passwords_2.jpg)

**Caution:** The currently logged-in administrator cannot reset their own password through list **Actions**.

## Delete Users

When a user no longer needs system access and related resources have been handed over, administrators can delete the user from **Actions** in the user list. Before deletion, the page displays a confirmation dialog and shows the `Email` of the user to be deleted.

1. Find the target user on the **User management** page. Hover over the user row to display **Actions**, then click the delete button.
2. Check the user's `Email` in the confirmation dialog.
3. Click **Delete** to delete the user.

![Delete Users](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/delete_users.jpg)

**Caution:** The currently logged-in administrator cannot delete their own account through list **Actions**.

Deleting a user is a high-risk operation. Before doing it, confirm whether the user's related data, knowledge bases, Agents, files, API keys, and business responsibilities have been handed over. If you only need to temporarily prevent login, change the account status to `Inactive` first.

## View User Details and Resource Impact

Click the detail button in **Actions** on the user list to enter the user detail page. The detail page displays the user's `Email`, account status, `Last login time`, `Create time`, `Last update time`, `Language`, `Is anonymous`, and `Is superuser`.

**Caution:** Before disabling, deleting, or downgrading `Superuser`, administrators should check the detail page to confirm whether the user still has important resources or recent login activity.

![View User Details And Resource Impact](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/view_user_details_and_resource_impact.jpg)
