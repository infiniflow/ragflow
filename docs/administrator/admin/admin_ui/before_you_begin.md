---
sidebar_position: 1
sidebar_label: Before You Begin
title: Before You Begin
---

# Before You Begin

## Access the Admin UI

System administrators can access the RAGFlow Admin UI in a browser. The current Admin UI entry point is `/admin`, for example `http://192.168.1.5/admin`.

After entering the Admin UI, administrators can perform service health checks, maintain user accounts, configure sandboxes, control registration, manage roles and permissions, configure system settings, and configure identity providers. The Admin UI should be exposed only to trusted administrators.

![Enter The Admin Console](https://raw.githubusercontent.com/infiniflow/ragflow-docs/main/images/enter_the_admin_console.jpg)

Regular users do not need to enter the Admin UI when they use business features such as knowledge bases, chat, Agent, files, and model providers.

## Initial Administrator Account

After the first deployment, you can log in with the default administrator account `admin@ragflow.io`. Its password is chosen when the admin server starts and no superuser exists yet:

- If the `ADMIN_DEFAULT_PASSWORD` environment variable is set (or `DEFAULT_SUPERUSER_PASSWORD`, which is shared with the web service bootstrap), that value is used.
- Otherwise a random password is generated and written **once** to `logs/admin_bootstrap_password.txt` (mode 0600; in a docker deployment check the `docker/ragflow-logs` volume) — retrieve it from that file, log in, and change it immediately.

This account is used to initialize the system and create subsequent administrator accounts. It is not recommended as a long-term shared daily operations account.

After the first login, reset the default password as soon as possible and create separate accounts for different administrators. In daily administration, grant the `Superuser` identity only to users who actually need Admin UI responsibilities.

## Change the Admin Account Password

This feature is not available yet.
