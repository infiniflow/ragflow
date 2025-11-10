---
sidebar_position: 6
slug: /using_admin_ui
---

# Admin UI

The RAGFlow Admin UI is a web-based interface that provides comprehensive system status monitoring and user management capabilities.


## Accessing the Admin UI

### Launching from source code

1. Start the RAGFlow front-end (if not already running):

   ```bash
   cd web
   npm run dev
   ```

   Typically, the front-end server is running on port `9222`. The following output confirms a successful launch of the RAGFlow UI:

   ```bash
            ╔════════════════════════════════════════════════════╗
            ║ App listening at:                                  ║
            ║  >   Local: http://localhost:9222                  ║
    ready - ║  > Network: http://192.168.1.92:9222               ║
            ║                                                    ║
            ║ Now you can open browser with the above addresses↑ ║
            ╚════════════════════════════════════════════════════╝
   ```


2. Login to RAGFlow Admin UI

   Open your browser and navigate to:

   ```
   http://localhost:9222/admin
   ```

   Or if accessing from a remote machine:

   ```
   http://[YOUR_MACHINE_IP]:9222/admin
   ```

   > Replace `[YOUR_MACHINE_IP]` with your actual machine IP address (e.g., `http://192.168.1.49:9222/admin`).

   Then, you will be presented with a login page where you need to enter your admin user email address and password.

3. After a successful login, you will be redirected to the **Service Status** page, which is the default landing page for the Admin UI.


## Admin UI Overview

### Service status

The service status page displays of all services within the RAGFlow system.

- **Service List**: View all services in a table format.
- **Filtering**: Use the filter button to filter services by **Service Type**.
- **Search**: Use the search bar to quickly find services by **Name** or **Service Type**.
- **Actions** (hover over a row to see action buttons):
  - **Extra Info**: Display additional configuration information of a service in a dialog.
  - **Service Details**: Display detailed status information of a service in a dialog. According to services's type, a service's status information could be displayed as a plain text, a key-value data list, a data table or a bar chart.


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