---
sidebar_position: 3
slug: /add_google_drive
sidebar_custom_props: {
  categoryIcon: SiGoogledrive
}
---

# Add Google Drive as data source

Add Google Drive as one of the data sources in RAGFlow.

---

This document provides step-by-step instructions for integrating Google Drive as a data source in RAGFlow.

## 1. Create a Google Cloud project

You can either create a dedicated project for RAGFlow or use an existing Google Cloud external project. In this case, we create a Google Cloud project from scratch:

1. Open the project creation page `https://console.cloud.google.com/projectcreate`:  
![](https://github.com/infiniflow/ragflow-docs/blob/040e4acd4c1eac6dc73dc44e934a6518de78d097/images/google_drive/image1.jpeg?raw=true)
2. Under **App Information**, provide an App name and your Gmail account as user support email:
![](https://github.com/infiniflow/ragflow-docs/blob/040e4acd4c1eac6dc73dc44e934a6518de78d097/images/google_drive/image2.png?raw=true)
3. Select **External**:
   _Your app will start in testing mode and will only be available to a selected list of users._
![](https://github.com/infiniflow/ragflow-docs/blob/040e4acd4c1eac6dc73dc44e934a6518de78d097/images/google_drive/image3.jpeg?raw=true)
4: Click **Create** to confirm creation.

## 2. Configure OAuth Consent Screen

You need to configure the OAuth Consent Screen because it is the step where you define how your app asks for permission and what specific data it wants to access on behalf of a user. It's a mandatory part of setting up OAuth 2.0 authentication with Google. Think of it as creating a standardized permission slip for your app. Without it, Google will not allow your app to request access to user data.

1. Go to **APIs & Services** → **OAuth consent screen**.
2. Ensure **User Type** is set to **External**:  
![](https://github.com/infiniflow/ragflow-docs/blob/040e4acd4c1eac6dc73dc44e934a6518de78d097/images/google_drive/image4.jpeg?raw=true)
3. Under Under **Test Users**, click **+ Add users** to add test users:  
![](https://github.com/infiniflow/ragflow-docs/blob/040e4acd4c1eac6dc73dc44e934a6518de78d097/images/google_drive/image5.jpeg?raw=true)
![](https://github.com/infiniflow/ragflow-docs/blob/040e4acd4c1eac6dc73dc44e934a6518de78d097/images/google_drive/image6.jpeg?raw=true)

## 3. Create OAuth Client Credentials

1. Navigate to `https://console.cloud.google.com/auth/clients`.
2. Select **Web Application** as **Application type** for the created project:  
![](https://github.com/infiniflow/ragflow-docs/blob/040e4acd4c1eac6dc73dc44e934a6518de78d097/images/google_drive/image7.png?raw=true)
3. Enter a client name.
4. Add `http://localhost:9380/v1/connector/google-drive/oauth/web/callback` as **Authorised redirect URIs**:
5. Add **Authorised JavaScript origins**:
   - If deploying RAGFlow from Docker, use `http://localhost:80`:  
     ![](https://github.com/infiniflow/ragflow-docs/blob/040e4acd4c1eac6dc73dc44e934a6518de78d097/images/google_drive/image8.png?raw=true)
   - If building RAGFlow from source, use `http://localhost:9222`
     ![](https://github.com/infiniflow/ragflow-docs/blob/040e4acd4c1eac6dc73dc44e934a6518de78d097/images/google_drive/image9.png?raw=true)

6.  After saving, click **Download JSON** in the popup window; this credential file will later be uploaded into RAGFlow.

![](https://github.com/infiniflow/ragflow-docs/blob/040e4acd4c1eac6dc73dc44e934a6518de78d097/images/google_drive/image10.png?raw=true)

## 4. Add Scopes

You need to add scopes to explicitly define the specific level of access your application requires from a user's Google Drive, such as read-only access to files. These scopes are presented to the user on the consent screen, ensuring transparency by showing exactly what permissions they are granted. To do so:

1. Click **Data Access** → **Add or remove scopes**, and add the following entries and click **Update**:

```
https://www.googleapis.com/auth/drive.readonly
https://www.googleapis.com/auth/drive.metadata.readonly
https://www.googleapis.com/auth/admin.directory.group.readonly
https://www.googleapis.com/auth/admin.directory.user.readonly
```

![](https://github.com/infiniflow/ragflow-docs/blob/040e4acd4c1eac6dc73dc44e934a6518de78d097/images/google_drive/image11.jpeg?raw=true)
![](https://github.com/infiniflow/ragflow-docs/blob/040e4acd4c1eac6dc73dc44e934a6518de78d097/images/google_drive/image12.jpeg?raw=true)

2. Click **Save** to save your data access changes:

![](https://github.com/infiniflow/ragflow-docs/blob/040e4acd4c1eac6dc73dc44e934a6518de78d097/images/google_drive/image13.jpeg?raw=true)

## 5. Enable required APIs

You need to enable the required APIs (such as the Google Drive API) to formally grant your Google Cloud project permission to communicate with Google's services on behalf of your application. These APIs act as a gateway; even if you have valid OAuth credentials, Google will block requests to a disabled API. Enabling them ensures that when RAGFlow attempts to list or retrieve files, Google's servers recognize and authorize the request.

1. Navigate to the Google API Library `https://console.cloud.google.com/apis/library`:  
![](https://github.com/infiniflow/ragflow-docs/blob/040e4acd4c1eac6dc73dc44e934a6518de78d097/images/google_drive/image14.png?raw=true)

2. Enable the following APIs:  
   - Google Drive API 
   - Admin SDK API 
   - Google Sheets API 
   - Google Docs API
  
![](https://github.com/infiniflow/ragflow-docs/blob/040e4acd4c1eac6dc73dc44e934a6518de78d097/images/google_drive/image15.png?raw=true)
![](https://github.com/infiniflow/ragflow-docs/blob/040e4acd4c1eac6dc73dc44e934a6518de78d097/images/google_drive/image16.png?raw=true)
![](https://github.com/infiniflow/ragflow-docs/blob/040e4acd4c1eac6dc73dc44e934a6518de78d097/images/google_drive/image17.png?raw=true)
![](https://github.com/infiniflow/ragflow-docs/blob/040e4acd4c1eac6dc73dc44e934a6518de78d097/images/google_drive/image18.png?raw=true)
![](https://github.com/infiniflow/ragflow-docs/blob/040e4acd4c1eac6dc73dc44e934a6518de78d097/images/google_drive/image19.png?raw=true)
![](https://github.com/infiniflow/ragflow-docs/blob/040e4acd4c1eac6dc73dc44e934a6518de78d097/images/google_drive/image21.png?raw=true)

## 6. Add Google Drive as a data source in RAGFlow

1. Go to **Data Sources** inside RAGFlow and select **Google Drive**.
2. Under **OAuth Token JSON**, upload the previously downloaded JSON credentials you saved in [Section 2](#2-configure-oauth-consent-screen):  
![](https://github.com/infiniflow/ragflow-docs/blob/040e4acd4c1eac6dc73dc44e934a6518de78d097/images/google_drive/image22.jpeg?raw=true)
3. Enter the url of the shared Google Drive folder link:
![](https://github.com/infiniflow/ragflow-docs/blob/040e4acd4c1eac6dc73dc44e934a6518de78d097/images/google_drive/image23.png?raw=true)
4. Click **Authorize with Google**  
   _A browser window appears showing that Google hasn't verified this app._  
![](https://github.com/infiniflow/ragflow-docs/blob/040e4acd4c1eac6dc73dc44e934a6518de78d097/images/google_drive/image25.jpeg?raw=true)
5. Click **Continue** → **Select All** → **Continue**.
6. When the authorization succeeds, select **OK** to add the data source.
![](https://github.com/infiniflow/ragflow-docs/blob/040e4acd4c1eac6dc73dc44e934a6518de78d097/images/google_drive/image26.jpeg?raw=true)
![](https://github.com/infiniflow/ragflow-docs/blob/040e4acd4c1eac6dc73dc44e934a6518de78d097/images/google_drive/image27.jpeg?raw=true)
![](https://github.com/infiniflow/ragflow-docs/blob/040e4acd4c1eac6dc73dc44e934a6518de78d097/images/google_drive/image28.png?raw=true)
![](https://github.com/infiniflow/ragflow-docs/blob/040e4acd4c1eac6dc73dc44e934a6518de78d097/images/google_drive/image29.png?raw=true)