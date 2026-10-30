---
sidebar_position: 1
sidebar_label: Before You Begin
title: Before You Begin
---

# Before You Begin

## Access the Admin UI

System administrators can access the RAGFlow Admin UI in a browser. The current Admin UI entry point is `/admin`, for example `http://192.168.1.5/admin`.

After entering the Admin UI, administrators can check service status, manage user accounts, and configure the code execution sandbox. The Admin UI should be exposed only to trusted administrators.

![Enter The Admin Console](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/enter_the_admin_console.jpg)

Regular users do not need to enter the Admin UI when they use business features such as knowledge bases, chat, Agent, files, and model providers.

## Initial Administrator Account

When the Admin service starts with `--init-superuser` and no superuser exists,
it creates the administrator account `admin@ragflow.io`. The initial password
comes from `DEFAULT_SUPERUSER_PASSWORD` or `ADMIN_DEFAULT_PASSWORD`; if neither
is set, RAGFlow generates a random password and writes it to
`logs/admin_bootstrap_password.txt`.

The checked-in Docker configuration sets `ADMIN_DEFAULT_PASSWORD=admin`. If
other hosts can reach the Admin Service, replace that value with a unique secret
in `docker/.env` before the first startup, and restrict the published Admin port
to trusted administrators. This setting only initializes the first
superuser—it does not reset an existing account's password.

This account is used to initialize the system and create subsequent administrator accounts. It is not recommended as a long-term shared daily operations account.

If you used the sample `admin` password, change it immediately after login and
create separate accounts for different administrators. In daily administration,
grant the `Superuser` identity only to users who actually need Admin UI
responsibilities.
