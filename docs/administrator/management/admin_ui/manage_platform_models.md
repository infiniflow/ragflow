---
sidebar_position: 5
sidebar_label: Manage Platform Models
title: Manage Platform Models
---

# Manage Platform Models

Superusers configure model providers that every tenant can use in **Admin > Platform models**. Tenants see these models in their model pickers with a **Platform** label and can choose them as defaults or in datasets, chats, and agents. Tenants cannot see the credentials or change the configuration.

:::caution System-wide setting
Changes on this page apply to every tenant. Disabling an instance or a model makes it unavailable everywhere it is used. Defaults and resources that reference it show as unavailable and are not switched to another model, with one existing exception: if the agentic RAG decision relevance filter cannot resolve its selected judge, it falls back to the environment-configured judge.
:::

## Add a provider, instance, and models

1. Click **Add provider** and choose a provider from the catalog. A provider appears once; add several instances to it for different accounts or endpoints.
2. Click **Add instance**, enter the instance name, credentials, and endpoint, and save. Saving an instance does not add any model.
3. Expand the instance and click **Add model**. Enter the model name or click **List models** to choose from the provider, then select the model types. Tenants can select a model only where its types apply. **Document parsing** marks a model that can parse documents in addition to its other types.

Use **Test connection** in the instance dialog before you enable an instance for production use. For a saved instance, the test calls one enabled model of each type and needs at least one enabled model; for a new instance, it checks the provider's catalog models. Either can incur a small provider charge.

## Credentials

Saved credentials are never shown again. When you edit an instance, a credential field left blank keeps the saved value and a new value replaces it. An endpoint that contains a user name, query string, or fragment is also hidden after saving; leave it blank to keep it.

To test a changed credential, fill in every credential field of the instance. Some providers save their endpoint with the credentials; if that endpoint is hidden, enter it too. Saving still keeps the fields you leave blank.

## Platform default models

**Platform default models** sets one model per type. A platform default applies only when a user has not chosen a default model of that type in **Model providers > Set default models**. Existing user choices and dataset embedding models do not change when you set or clear a platform default. Clearing a platform default leaves users without their own choice of that type with no default model.

Only enabled models of enabled instances can be set as a platform default.

## Upgrade and rollback

Platform models use the existing model tables and `system_settings`; no schema change is needed. Platform providers are stored with the reserved tenant ID `__platform__`, and the Admin service initializes the `platform.provider_create_lock` setting at startup. Platform defaults are stored as `platform.default_model.<type>` settings. If a tenant, user, or membership already uses the reserved ID, the Admin service refuses to start and reports the conflict.

Upgrade every API, Admin, and Ingestor instance before configuring platform models. Until the Admin service has initialized the platform settings, platform models are not offered or used.

Before rolling back to a release without platform models, remove platform defaults and disable platform models, then point user defaults, datasets, chats, and agents that use them back to tenant models. This step is required. An older release does not know platform models: a reference by model ID fails with an unavailable model error, but a user default saved from a platform model also stores the model name, and an older release may resolve that name to a tenant model with the same provider, instance, and model name instead. Datasets whose embedding model is a platform model need that model, or a re-index with another embedding model.

Upgrading leaves existing tenant model rows unchanged, and running an older release's migration afterwards leaves platform rows in place, so a later upgrade finds them again.

### Existing datasets without an embedding binding

Some older datasets may contain chunks but have no saved embedding model. The service cannot determine which model produced existing vectors, so it will not automatically use the current platform default for them. If the original embedding model is known, explicitly select that same model in the dataset settings to restore the binding. If it is unknown, rebuild the dataset index with a chosen embedding model before using vector retrieval; do not assume the current default produced the existing vectors.
