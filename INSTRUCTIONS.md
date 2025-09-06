# RAGFlow with Supabase and Tailscale

This document provides instructions on how to configure this forked version of RAGFlow to use Supabase for database and storage, and Tailscale for remote access.

## Prerequisites

- A Supabase account (free tier is sufficient).
- A Tailscale account and Tailscale installed on both your local machine and the server where you will run RAGFlow.

## 1. Supabase Configuration

1.  **Create a new Supabase project:**
    - Go to your Supabase dashboard and create a new project.

2.  **Get Database Credentials:**
    - In your Supabase project, navigate to **Settings > Database**.
    - Under **Connection info**, you will find the following:
        - `Host`
        - `Port`
        - `Database name` (usually `postgres`)
        - `User` (usually `postgres`)
        - `Password`

3.  **Get S3 Storage Credentials:**
    - In your Supabase project, navigate to **Settings > Storage**.
    - You will find the following:
        - `Endpoint URL`
        - `Region`
    - To get the `Access Key` and `Secret Key`, you need to generate them. Go to **Storage > Settings > S3 Connection** and generate a new key pair.
    - Create a new bucket in the Supabase Storage section. The name of this bucket will be your `S3_BUCKET`.

4.  **Configure RAGFlow:**
    - In the `docker` directory, copy the `.env.example` file to a new file named `.env`.
    - Open the `.env` file and fill in the values for the PostgreSQL and S3 sections with the credentials you obtained from Supabase.

## 2. Running RAGFlow

Once you have configured your `.env` file, you can start RAGFlow using Docker Compose:

```bash
cd docker
docker-compose up -d
```

## 3. Tailscale Access

To access your RAGFlow instance from your local machine using Tailscale, you need to find the Tailscale IP address of your server.

1.  **Find your server's Tailscale IP:**
    - On your server, run the following command:
      ```bash
      tailscale ip -4
      ```
    - This will give you the Tailscale IP address of your server (e.g., `100.x.x.x`).

2.  **Access RAGFlow:**
    - On your local machine, open your web browser and go to `http://<your_server_tailscale_ip>:9380`.
    - You should now be able to access the RAGFlow web interface.

## 4. Troubleshooting

- If you have any issues connecting to the database or storage, double-check your credentials in the `.env` file.
- Make sure that the bucket you created in Supabase Storage is public if you want to access the files directly.
- If you have trouble accessing RAGFlow through Tailscale, ensure that Tailscale is running on both your local machine and the server, and that there are no firewall rules blocking the connection.
