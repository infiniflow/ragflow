---
sidebar_position: 1
slug: /admin_ui
sidebar_custom_props: {
  categoryIcon: LucidePalette
}
---
# Admin UI

The RAGFlow Admin UI is a web-based interface that provides comprehensive system status monitoring and user management capabilities.


## Accessing the Admin UI

To access the RAGFlow admin UI, append `/admin` to the web UI's address, e.g. `http://[RAGFLOW_WEB_UI_ADDR]/admin`, replace `[RAGFLOW_WEB_UI_ADDR]` with real RAGFlow web UI address.

### Default Credentials
| Username           | Password |
|--------------------|----------|
| `admin@ragflow.io` | `admin`  |

![Enter The Admin Console](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/enter_the_admin_console.png)


## Admin UI Overview

### Service status

The service status page displays of all services within the RAGFlow system.

- **Service List**: View all services in a table.
- **Filtering**: Use the filter button to filter services by **Service Type**.
- **Search**: Use the search bar to quickly find services by **Name** or **Service Type**.
- **Actions** (hover over a row to see action buttons):
  - **Extra Info**: Display additional configuration information of a service in a dialog.
  - **Service Details**: Display detailed status information of a service in a dialog. According to service's type, a service's status information could be displayed as a plain text, a key-value data list, a data table or a bar chart.

![Check Whether Services Are Normal](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/check_whether_services_are_normal.png)

![Query Monitoring Metrics](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/query_monitoring_metrics_1.png)

![Query Monitoring Metrics](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/query_monitoring_metrics_2.png)

![System Status](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/system_status.png)

![Use Monitoring To View System Monitoring](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/use_monitoring_to_view_system_monitoring.png)

![View Alert Rules And Alert Status](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/view_alert_rules_and_alert_status.png)

![View Prometheus Monitoring Status](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/view_prometheus_monitoring_status_1.png)

![View Prometheus Monitoring Status](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/view_prometheus_monitoring_status_2.png)

![View Service Details](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/view_service_details_1.png)

![View Service Details](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/view_service_details_2.png)

![View Service Details](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/view_service_details_3.png)

![View Service Details](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/view_service_details_4.png)


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

![Create A New User](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/create_a_new_user_1.png)

![Create A New User](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/create_a_new_user_2.png)

![Delete Users](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/delete_users.png)

![Disable Or Restore Accounts](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/disable_or_restore_accounts.png)

![Enable Email Verification](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/enable_email_verification.png)

![Enable Registration Whitelist](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/enable_registration_whitelist_1.png)

![Enable Registration Whitelist](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/enable_registration_whitelist_2.png)

![Manage Registration Whitelist](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/manage_registration_whitelist.png)

![Reset User Passwords](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/reset_user_passwords_1.png)

![Reset User Passwords](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/reset_user_passwords_2.png)

![Set Backend Administrator Identity](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/set_backend_administrator_identity.png)

![User Management](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/user_management_1.png)

![User Management](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/user_management_2.png)

![User Management](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/user_management_3.png)

![View And Search Users](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/view_and_search_users_1.png)

![View And Search Users](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/view_and_search_users_2.png)

![View User Details And Resource Impact](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/view_user_details_and_resource_impact.png)

![Whitelist](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/whitelist.png)



### User detail

The user detail page displays a user's detailed information and all resources created or owned by the user, categorized by type (e.g. Dataset, Agent).

![Assign Roles To Users](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/assign_roles_to_users.png)

![Configure System Email](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/configure_system_email.png)

![Configure System Notifications](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/configure_system_notifications.png)

![Maintain Roles](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/maintain_roles.png)

![Manage Licenses](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/manage_licenses.png)

![Role Management](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/role_management.png)

![Select Sandbox Provider](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/select_sandbox_provider.png)

![Set Model Provider Page Visibility Scope](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/set_model_provider_page_visibility_scope.png)
