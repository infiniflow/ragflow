<div align="center">
<a href="https://demo.ragflow.io/">
<img src="web/src/assets/logo-with-text.png" width="520" alt="Logo ragflow">
</a>
</div>

<p align="center">
  <a href="./README.md">English</a> |
  <a href="./README_zh.md">简体中文</a> |
  <a href="./README_ja.md">日本語</a> |
  <a href="./README_ko.md">한국어</a> |
  <a href="./README_id.md">Bahasa Indonesia</a>
</p>

<p align="center">
    <a href="https://x.com/intent/follow?screen_name=infiniflowai" target="_blank">
        <img src="https://img.shields.io/twitter/follow/infiniflow?logo=X&color=%20%23f5f5f5" alt="Ikuti di X (Twitter)">
    </a>
    <a href="https://demo.ragflow.io" target="_blank">
        <img alt="Lencana Daring" src="https://img.shields.io/badge/Online-Demo-4e6b99">
    </a>
    <a href="https://hub.docker.com/r/infiniflow/ragflow" target="_blank">
        <img src="https://img.shields.io/badge/docker_pull-ragflow:v0.13.0-brightgreen" alt="docker pull infiniflow/ragflow:v0.13.0">
    </a>
    <a href="https://github.com/infiniflow/ragflow/releases/latest">
        <img src="https://img.shields.io/github/v/release/infiniflow/ragflow?color=blue&label=Rilis%20Terbaru" alt="Rilis Terbaru">
    </a>
    <a href="https://github.com/infiniflow/ragflow/blob/main/LICENSE">
        <img height="21" src="https://img.shields.io/badge/Lisensi-Apache--2.0-ffffff?labelColor=d4eaf7&color=2e6cc4" alt="Lisensi">
    </a>
</p>

<h4 align="center">
  <a href="https://ragflow.io/docs/dev/">Dokumentasi</a> |
  <a href="https://github.com/infiniflow/ragflow/issues/162">Peta Jalan</a> |
  <a href="https://twitter.com/infiniflowai">Twitter</a> |
  <a href="https://discord.gg/4XxujFgUN7">Discord</a> |
  <a href="https://demo.ragflow.io">Demo</a>
</h4>

<details open>
<summary></b>📕 Daftar Isi</b></summary>

- 💡 [Apa Itu RAGFlow?](#-apa-itu-ragflow)
- 🎮 [Demo](#-demo)
- 📌 [Pembaruan Terbaru](#-pembaruan-terbaru)
- 🌟 [Fitur Utama](#-fitur-utama)
- 🔎 [Arsitektur Sistem](#-arsitektur-sistem)
- 🎬 [Mulai](#-mulai)
- 🔧 [Konfigurasi](#-konfigurasi)
- 🔧 [Membangun Image Docker tanpa Model Embedding](#-membangun-image-docker-tanpa-model-embedding)
- 🔧 [Membangun Image Docker dengan Model Embedding](#-membangun-image-docker-dengan-model-embedding)
- 🔨 [Meluncurkan aplikasi dari Sumber untuk Pengembangan](#-meluncurkan-aplikasi-dari-sumber-untuk-pengembangan)
- 📚 [Dokumentasi](#-dokumentasi)
- 📜 [Peta Jalan](#-peta-jalan)
- 🏄 [Komunitas](#-komunitas)
- 🙌 [Kontribusi](#-kontribusi)

</details>

## 💡 Apa Itu RAGFlow?

[RAGFlow](https://ragflow.io/) adalah mesin RAG (Retrieval-Augmented Generation) open-source berbasis pemahaman dokumen yang mendalam. Platform ini menyediakan alur kerja RAG yang efisien untuk bisnis dengan berbagai skala, menggabungkan LLM (Large Language Models) untuk menyediakan kemampuan tanya-jawab yang benar dan didukung oleh referensi dari data terstruktur kompleks.

## 🎮 Demo

Coba demo kami di [https://demo.ragflow.io](https://demo.ragflow.io).
<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://github.com/infiniflow/ragflow/assets/7248/2f6baa3e-1092-4f11-866d-36f6a9d075e5" width="1200"/>
<img src="https://github.com/user-attachments/assets/504bbbf1-c9f7-4d83-8cc5-e9cb63c26db6" width="1200"/>
</div>

## 🔥 Pembaruan Terbaru

- 2024-11-01: Penambahan ekstraksi kata kunci dan pembuatan pertanyaan terkait untuk meningkatkan akurasi pengambilan.
- 2024-09-13: Penambahan mode pencarian untuk Q&A basis pengetahuan.
- 2024-09-09: Penambahan template agen konsultan medis.
- 2024-08-22: Dukungan untuk teks ke pernyataan SQL melalui RAG.
- 2024-08-02: Dukungan GraphRAG yang terinspirasi oleh [graphrag](https://github.com/microsoft/graphrag) dan mind map.

## 🎉 Tetap Terkini

⭐️ Star repositori kami untuk tetap mendapat informasi tentang fitur baru dan peningkatan menarik! 🌟
<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://github.com/user-attachments/assets/18c9707e-b8aa-4caf-a154-037089c105ba" width="1200"/>
</div>

## 🌟 Fitur Utama

### 🍭 **"Kualitas Masuk, Kualitas Keluar"**

- Ekstraksi pengetahuan berbasis pemahaman dokumen mendalam dari data tidak terstruktur dengan format yang rumit.
- Menemukan "jarum di tumpukan data" dengan token yang hampir tidak terbatas.

### 🍱 **Pemotongan Berbasis Template**

- Cerdas dan dapat dijelaskan.
- Banyak pilihan template yang tersedia.

### 🌱 **Referensi yang Didasarkan pada Data untuk Mengurangi Hallusinasi**

- Visualisasi pemotongan teks memungkinkan intervensi manusia.
- Tampilan cepat referensi kunci dan referensi yang dapat dilacak untuk mendukung jawaban yang didasarkan pada fakta.

### 🍔 **Kompatibilitas dengan Sumber Data Heterogen**

- Mendukung Word, slide, excel, txt, gambar, salinan hasil scan, data terstruktur, halaman web, dan banyak lagi.

### 🛀 **Alur Kerja RAG yang Otomatis dan Mudah**

- Orkestrasi RAG yang ramping untuk bisnis kecil dan besar.
- LLM yang dapat dikonfigurasi serta model embedding.
- Peringkat ulang berpasangan dengan beberapa pengambilan ulang.
- API intuitif untuk integrasi yang mudah dengan bisnis.

## 🔎 Arsitektur Sistem

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://github.com/infiniflow/ragflow/assets/12318111/d6ac5664-c237-4200-a7c2-a4a00691b485" width="1000"/>
</div>

## 🎬 Mulai

### 📝 Prasyarat

- CPU >= 4 inti
- RAM >= 16 GB
- Disk >= 50 GB
- Docker >= 24.0.0 & Docker Compose >= v2.26.1

### 🚀 Menjalankan Server

1. Pastikan `vm.max_map_count` >= 262144:

   > Untuk memeriksa nilai `vm.max_map_count`:
   >
   > ```bash
   > $ sysctl vm.max_map_count
   > ```
   >
   > Jika nilainya kurang dari 262144, setel ulang `vm.max_map_count` ke setidaknya 262144:
   >
   > ```bash
   > # Dalam contoh ini, kita atur menjadi 262144:
   > $ sudo sysctl -w vm.max_map_count=262144
   > ```
   >
   > Perubahan ini akan hilang setelah sistem direboot. Untuk membuat perubahan ini permanen, tambahkan atau perbarui nilai
   `vm.max_map_count` di **/etc/sysctl.conf**:
   >
   > ```bash
   > vm.max_map_count=262144
   > ```

2. Clone repositori:

   ```bash
   $ git clone https://github.com/infiniflow/ragflow.git
   ```

3. Bangun image Docker pre-built dan jalankan server:

   > Perintah di bawah ini akan mengunduh versi dev dari Docker image RAGFlow slim (`dev-slim`). Image RAGFlow slim
   tidak termasuk model embedding atau library Python dan berukuran sekitar 1GB.

   ```bash
   $ cd ragflow/docker
   $ docker compose -f docker-compose.yml up -d
   ```

   > - Untuk mengunduh versi tertentu dari image Docker RAGFlow slim, perbarui variabel `RAGFlow_IMAGE` di *
       *docker/.env** sesuai dengan versi yang diinginkan. Misalnya, `RAGFLOW_IMAGE=infiniflow/ragflow:v0.13.0-slim`.
       Setelah mengubah ini, jalankan ulang perintah di atas untuk memulai unduhan.
   > - Untuk mengunduh versi dev dari image Docker RAGFlow *termasuk* model embedding dan library Python, perbarui
       variabel `RAGFlow_IMAGE` di **docker/.env** menjadi `RAGFLOW_IMAGE=infiniflow/ragflow:dev`. Setelah mengubah ini,
       jalankan ulang perintah di atas untuk memulai unduhan.
   > - Untuk mengunduh versi tertentu dari image Docker RAGFlow *termasuk* model embedding dan library Python, perbarui
       variabel `RAGFlow_IMAGE` di **docker/.env** sesuai dengan versi yang diinginkan. Misalnya,
       `RAGFLOW_IMAGE=infiniflow/ragflow:v0.13.0`. Setelah mengubah ini, jalankan ulang perintah di atas untuk memulai unduhan.

   > **CATATAN:** Image Docker RAGFlow yang mencakup model embedding dan library Python berukuran sekitar 9GB
   dan mungkin memerlukan waktu lebih lama untuk dimuat.

4. Periksa status server setelah server aktif dan berjalan:

   ```bash
   $ docker logs -f ragflow-server
   ```

   _Output berikut menandakan bahwa sistem berhasil diluncurkan:_

   ```bash

         ____   ___    ______ ______ __               
        / __ \ /   |  / ____// ____// /____  _      __
       / /_/ // /| | / / __ / /_   / // __ \| | /| / /
      / _, _// ___ |/ /_/ // __/  / // /_/ /| |/ |/ / 
     /_/ |_|/_/  |_|\____//_/    /_/ \____/ |__/|__/ 

    * Running on all addresses (0.0.0.0)
    * Running on http://127.0.0.1:9380
    * Running on http://x.x.x.x:9380
    INFO:werkzeug:Press CTRL+C to quit
   ```
   > Jika Anda melewatkan langkah ini dan langsung login ke RAGFlow, browser Anda mungkin menampilkan error `network anormal`
   karena RAGFlow mungkin belum sepenuhnya siap.

5. Buka browser web Anda, masukkan alamat IP server Anda, dan login ke RAGFlow.
   > Dengan pengaturan default, Anda hanya perlu memasukkan `http://IP_DEVICE_ANDA` (**tanpa** nomor port) karena 
   port HTTP default `80` bisa dihilangkan saat menggunakan konfigurasi default.
6. Dalam [service_conf.yaml](./docker/service_conf.yaml), pilih LLM factory yang diinginkan di `user_default_llm` dan perbarui
   bidang `API_KEY` dengan kunci API yang sesuai.

   > Lihat [llm_api_key_setup](https://ragflow.io/docs/dev/llm_api_key_setup) untuk informasi lebih lanjut.

   _Sistem telah siap digunakan!_

## 🔧 Konfigurasi

Untuk konfigurasi sistem, Anda perlu mengelola file-file berikut:

- [.env](./docker/.env): Menyimpan pengaturan dasar sistem, seperti `SVR_HTTP_PORT`, `MYSQL_PASSWORD`, dan
  `MINIO_PASSWORD`.
- [service_conf.yaml](./docker/service_conf.yaml): Mengonfigurasi aplikasi backend.
- [docker-compose.yml](./docker/docker-compose.yml): Sistem ini bergantung pada [docker-compose.yml](./docker/docker-compose.yml) untuk memulai.

Anda harus memastikan bahwa perubahan pada file [.env](./docker/.env) sesuai dengan yang ada di file [service_conf.yaml](./docker/service_conf.yaml).

> File [./docker/README](./docker/README.md) menyediakan penjelasan detail tentang pengaturan lingkungan dan konfigurasi aplikasi,
> dan Anda DIWAJIBKAN memastikan bahwa semua pengaturan lingkungan yang tercantum di
> [./docker/README](./docker/README.md) selaras dengan konfigurasi yang sesuai di
> [service_conf.yaml](./docker/service_conf.yaml).

Untuk memperbarui port HTTP default (80), buka [docker-compose.yml](./docker/docker-compose.yml) dan ubah `80:80`
menjadi `<YOUR_SERVING_PORT>:80`.

Pembaruan konfigurasi ini memerlukan reboot semua kontainer agar efektif:

> ```bash
> $ docker compose -f docker/docker-compose.yml up -d
> ```

## 🔧 Membangun Docker Image tanpa Model Embedding

Image ini berukuran sekitar 1 GB dan bergantung pada aplikasi LLM eksternal dan embedding.

```bash
git clone https://github.com/infiniflow/ragflow.git
cd ragflow/
pip3 install huggingface-hub nltk
python3 download_deps.py
docker build -f Dockerfile.slim -t infiniflow/ragflow:dev-slim .
```

## 🔧 Membangun Docker Image Termasuk Model Embedding

Image ini berukuran sekitar 9 GB. Karena sudah termasuk model embedding, ia hanya bergantung pada aplikasi LLM eksternal.

```bash
git clone https://github.com/infiniflow/ragflow.git
cd ragflow/
pip3 install huggingface-hub nltk
python3 download_deps.py
docker build -f Dockerfile -t infiniflow/ragflow:dev .
```

## 🔨 Menjalankan Aplikasi dari untuk Pengembangan

1. Instal Poetry, atau lewati langkah ini jika sudah terinstal:
   ```bash
   curl -sSL https://install.python-poetry.org | python3 -
   ```

2. Clone kode sumber dan instal dependensi Python:
   ```bash
   git clone https://github.com/infiniflow/ragflow.git
   cd ragflow/
   export POETRY_VIRTUALENVS_CREATE=true POETRY_VIRTUALENVS_IN_PROJECT=true
   ~/.local/bin/poetry install --sync --no-root # install modul python RAGFlow
   ```

3. Jalankan aplikasi yang diperlukan (MinIO, Elasticsearch, Redis, dan MySQL) menggunakan Docker Compose:
   ```bash
   docker compose -f docker/docker-compose-base.yml up -d
   ```

   Tambahkan baris berikut ke `/etc/hosts` untuk memetakan semua host yang ditentukan di **docker/service_conf.yaml** ke `127.0.0.1`:
   ```
   127.0.0.1       es01 mysql minio redis
   ```  
   Di **docker/service_conf.yaml**, perbarui port mysql ke `5455` dan es ke `1200`, sesuai dengan yang ditentukan di **docker/.env**.

4. Jika Anda tidak dapat mengakses HuggingFace, atur variabel lingkungan `HF_ENDPOINT` untuk menggunakan situs mirror:

   ```bash
   export HF_ENDPOINT=https://hf-mirror.com
   ```

5. Jalankan aplikasi backend:
   ```bash
   source .venv/bin/activate
   export PYTHONPATH=$(pwd)
   bash docker/launch_backend_service.sh
   ```

6. Instal dependensi frontend:
   ```bash
   cd web
   npm install --force
   ```  
7. Konfigurasikan frontend untuk memperbarui `proxy.target` di **.umirc.ts** menjadi `http://127.0.0.1:9380`:
8. Jalankan aplikasi frontend:
   ```bash
   npm run dev 
   ```  

   _Output berikut menandakan bahwa sistem berhasil diluncurkan:_

   ![](https://github.com/user-attachments/assets/0daf462c-a24d-4496-a66f-92533534e187)

## 📚 Dokumentasi

- [Quickstart](https://ragflow.io/docs/dev/)
- [Panduan Pengguna](https://ragflow.io/docs/dev/category/guides)
- [Referensi](https://ragflow.io/docs/dev/category/references)
- [FAQ](https://ragflow.io/docs/dev/faq)

## 📜 Roadmap

Lihat [Roadmap RAGFlow 2024](https://github.com/infiniflow/ragflow/issues/162)

## 🏄 Komunitas

- [Discord](https://discord.gg/4XxujFgUN7)
- [Twitter](https://twitter.com/infiniflowai)
- [GitHub Discussions](https://github.com/orgs/infiniflow/discussions)

## 🙌 Kontribusi

RAGFlow berkembang melalui kolaborasi open-source. Dalam semangat ini, kami menerima kontribusi dari komunitas.
Jika Anda ingin berpartisipasi, tinjau terlebih dahulu [Panduan Kontribusi](./CONTRIBUTING.md).