# Paperless-ngx UI Integration Guide

## Overview

This document describes the UI integration for the Paperless-ngx data source connector in RAGFlow.

## User Interface Components

### 1. Data Source Selection

Paperless-ngx appears in the data source list with:
- **Icon**: Document stack with "ngx" badge (green)
- **Name**: Paperless-ngx
- **Description**: "Connect to Paperless-ngx to sync and index your document management system."

### 2. Configuration Form

When users click to add a Paperless-ngx connector, they see a modal form with the following fields:

#### Required Fields

1. **Name** (Text Input)
   - User-defined name for this connector instance
   - Example: "My Paperless Documents"

2. **Paperless-ngx URL** (Text Input)
   - The base URL of the Paperless-ngx instance
   - Placeholder: `https://paperless.example.com`
   - Tooltip: "The base URL of your Paperless-ngx instance (e.g., https://paperless.example.com or http://localhost:8000)"
   - Validation: Required

3. **API Token** (Password Input)
   - API token from Paperless-ngx
   - Tooltip: "Generate an API token in Paperless-ngx: Settings → API Tokens. This token is used for authentication."
   - Validation: Required
   - Display: Masked password field

#### Optional Fields

4. **Verify SSL** (Checkbox)
   - Whether to verify SSL certificates
   - Default: Checked (true)
   - Tooltip: "Whether to verify SSL certificates. Disable only for self-signed certificates in development."

5. **Batch Size** (Number Input)
   - Number of documents to process per batch
   - Default: 2
   - Placeholder: "Defaults to 2"
   - Tooltip: "Number of documents to process in each batch. Adjust based on system resources."
   - Validation: Optional, positive integer

### 3. Form Layout

```
┌─────────────────────────────────────────────────────┐
│  📄 Paperless-ngx                                   │
│  Add Data Source                                    │
├─────────────────────────────────────────────────────┤
│                                                     │
│  Name *                                            │
│  ┌─────────────────────────────────────────────┐  │
│  │ My Paperless Documents                      │  │
│  └─────────────────────────────────────────────┘  │
│                                                     │
│  Paperless-ngx URL * ⓘ                            │
│  ┌─────────────────────────────────────────────┐  │
│  │ https://paperless.example.com               │  │
│  └─────────────────────────────────────────────┘  │
│                                                     │
│  API Token * ⓘ                                    │
│  ┌─────────────────────────────────────────────┐  │
│  │ ••••••••••••••••                           │  │
│  └─────────────────────────────────────────────┘  │
│                                                     │
│  ☑ Verify SSL ⓘ                                   │
│                                                     │
│  Batch Size ⓘ                                     │
│  ┌─────────────────────────────────────────────┐  │
│  │ 10                                          │  │
│  └─────────────────────────────────────────────┘  │
│                                                     │
│                          [ Cancel ]  [ Confirm ]   │
└─────────────────────────────────────────────────────┘
```

## Technical Implementation

### Files Modified

1. **web/src/pages/user-setting/data-source/constant/index.tsx**
   - Added `PAPERLESS_NGX` to `DataSourceKey` enum
   - Added icon and description in `generateDataSourceInfo()`
   - Added form field configurations in `DataSourceFormFields`
   - Added default values in `DataSourceFormDefaultValues`

2. **web/src/locales/en.ts**
   - Added `paperless_ngxDescription`
   - Added `paperless_ngxBaseUrlTip`
   - Added `paperless_ngxApiTokenTip`
   - Added `paperless_ngxVerifySslTip`
   - Added `paperless_ngxBatchSizeTip`

3. **web/src/assets/svg/data-source/paperless-ngx.svg**
   - Created custom SVG icon representing document management
   - Icon features document stack with green "ngx" badge

### Data Structure

When the form is submitted, it generates the following configuration:

```json
{
  "name": "My Paperless Documents",
  "source": "paperless_ngx",
  "config": {
    "base_url": "https://paperless.example.com",
    "verify_ssl": true,
    "batch_size": 10,
    "credentials": {
      "api_token": "your-api-token-here"
    }
  }
}
```

This matches the backend connector's expected configuration format.

## User Workflow

### Step 1: Navigate to Data Sources
1. User goes to Settings → Data Sources
2. Clicks "Add Data Source"
3. Selects "Paperless-ngx" from the list

### Step 2: Configure Connection
1. Fill in the required fields:
   - Name for the connector
   - Paperless-ngx server URL
   - API token (generated in Paperless-ngx)
2. Optionally adjust:
   - SSL verification (for development/self-signed certs)
   - Batch size (for performance tuning)

### Step 3: Save and Sync
1. Click "Confirm" to create the connector
2. The connector appears in the data sources list
3. Link it to a knowledge base to start syncing documents

## Validation

The form performs the following validations:

1. **Name**: Required, non-empty string
2. **Paperless-ngx URL**: Required, should be a valid URL
3. **API Token**: Required, non-empty string
4. **Batch Size**: Optional, must be a positive integer if provided

## Error Handling

The UI will display errors from the backend if:
- The Paperless-ngx URL is unreachable
- The API token is invalid or expired
- The user lacks permissions
- Network connectivity issues occur

These errors are displayed as toast notifications or inline error messages.

## Testing Checklist

- [ ] Paperless-ngx appears in data source list
- [ ] Icon displays correctly
- [ ] Description is visible
- [ ] Form opens when selecting Paperless-ngx
- [ ] All fields render correctly
- [ ] Tooltips appear on hover
- [ ] Required field validation works
- [ ] Form submission sends correct data structure
- [ ] Success message appears after creation
- [ ] New connector appears in data sources list
- [ ] Connector can be edited
- [ ] Connector can be deleted

## Troubleshooting

### Icon Not Displaying
- Verify the SVG file exists at `web/src/assets/svg/data-source/paperless-ngx.svg`
- Check that the SVG is valid XML
- Clear browser cache

### Form Fields Missing
- Verify `DataSourceKey.PAPERLESS_NGX` is in the enum
- Check that form fields are defined in `DataSourceFormFields`
- Ensure default values are set in `DataSourceFormDefaultValues`

### Translations Not Showing
- Check that translations exist in `web/src/locales/en.ts`
- Verify the translation keys match exactly
- Rebuild the frontend if necessary

## Screenshots

(Screenshots would be added here after running the application)

### Data Source List
Shows Paperless-ngx as an available data source option.

### Configuration Form
Shows the complete form with all fields and tooltips.

### Created Connector
Shows the connector after successful creation in the data sources list.

## Future Enhancements

Potential improvements for the UI:

1. **Advanced Options Accordion**
   - Move optional fields (verify_ssl, batch_size) into a collapsible "Advanced" section
   - Keep the main form cleaner

2. **Connection Test Button**
   - Add a "Test Connection" button to validate credentials before saving
   - Show real-time feedback on connection status

3. **Documentation Link**
   - Add a link to Paperless-ngx documentation
   - Help users find where to generate API tokens

4. **Visual Feedback**
   - Show loading spinner when testing connection
   - Display success/error icons after validation

5. **Sync Status Indicator**
   - Show last sync time
   - Display number of documents synced
   - Show sync progress in real-time
