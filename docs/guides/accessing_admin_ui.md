---
sidebar_position: 7
slug: /accessing_admin_ui
---

# Admin UI

The RAGFlow Admin UI is a web-based interface that provides comprehensive system status monitoring and user management capabilities.


## Accessing the Admin UI

To access the RAGFlow admin UI, append `/admin` to the web UI's address, e.g. `http://[RAGFLOW_WEB_UI_ADDR]/admin`, replace `[RAGFLOW_WEB_UI_ADDR]` with real RAGFlow web UI address.

### Default Credentials
| Username | Password |
|----------|----------|
| `admin@ragflow.io`   | `admin` |

## Admin UI Overview

### Service status

The service status page displays of all services within the RAGFlow system.

- **Service List**: View all services in a table.
- **Filtering**: Use the filter button to filter services by **Service Type**.
- **Search**: Use the search bar to quickly find services by **Name** or **Service Type**.
- **Actions** (hover over a row to see action buttons):
  - **Extra Info**: Display additional configuration information of a service in a dialog.
  - **Service Details**: Display detailed status information of a service in a dialog. According to service's type, a service's status information could be displayed as a plain text, a key-value data list, a data table or a bar chart.


### User management

The user management page provides comprehensive tools for managing all users in the RAGFlow system.

- **User List**: View all users in a table.
- **Search Users**: Use the search bar to find users by email or nickname.
- **Filter Users**: Click the filter icon to filter by **Status**.
- Click the **"New User"** button to create a new user account in a dialog.
- Activate or deactivate a user by using the switch toggle in **Enable** column, changes take effect immediately.
- **Actions** (hover over a row to see action buttons):
  - **View Details**: Navigate to the user detail page to see comprehensive user information.
  - **Change Password**: Force reset the user's password.
  - **Delete User**: Remove the user from the system with confirmation.


### User detail

The user detail page displays a user's detailed information and all resources created or owned by the user, categorized by type (e.g. Dataset, Agent).