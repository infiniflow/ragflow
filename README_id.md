<div align="center">
<a href="https://demo.ragflow.io/">
<img src="web/src/assets/logo-with-text.svg" width="520" alt="Logo ragflow">
</a>
</div>

<p align="center">
  <a href="./README.md"><img alt="README in English" src="https://img.shields.io/badge/English-DFE0E5"></a>
  <a href="./README_zh.md"><img alt="简体中文版自述文件" src="https://img.shields.io/badge/简体中文-DFE0E5"></a>
  <a href="./README_tzh.md"><img alt="繁體中文版自述文件" src="https://img.shields.io/badge/繁體中文-DFE0E5"></a>
  <a href="./README_ja.md"><img alt="日本語のREADME" src="https://img.shields.io/badge/日本語-DFE0E5"></a>
  <a href="./README_ko.md"><img alt="한국어" src="https://img.shields.io/badge/한국어-DFE0E5"></a>
  <a href="./README_id.md"><img alt="Bahasa Indonesia" src="https://img.shields.io/badge/Bahasa Indonesia-DBEDFA"></a>
  <a href="./README_pt_br.md"><img alt="Português(Brasil)" src="https://img.shields.io/badge/Português(Brasil)-DFE0E5"></a>
</p>

<p align="center">
    <a href="https://x.com/intent/follow?screen_name=infiniflowai" target="_blank">
        <img src="https://img.shields.io/twitter/follow/infiniflow?logo=X&color=%20%23f5f5f5" alt="Ikuti di X (Twitter)">
    </a>
    <a href="https://demo.ragflow.io" target="_blank">
        <img alt="Lencana Daring" src="https://img.shields.io/badge/Online-Demo-4e6b99">
    </a>
    <a href="https://hub.docker.com/r/infiniflow/ragflow" target="_blank">
        <img src="https://img.shields.io/docker/pulls/infiniflow/ragflow?label=Docker%20Pulls&color=0db7ed&logo=docker&logoColor=white&style=flat-square" alt="docker pull infiniflow/ragflow:v0.22.1">
    </a>
    <a href="https://github.com/infiniflow/ragflow/releases/latest">
        <img src="https://img.shields.io/github/v/release/infiniflow/ragflow?color=blue&label=Rilis%20Terbaru" alt="Rilis Terbaru">
    </a>
    <a href="https://github.com/infiniflow/ragflow/blob/main/LICENSE">
        <img height="21" src="https://img.shields.io/badge/Lisensi-Apache--2.0-ffffff?labelColor=d4eaf7&color=2e6cc4" alt="Lisensi">
    </a>
    <a href="https://deepwiki.com/infiniflow/ragflow">
        <img alt="Ask DeepWiki" src="https://deepwiki.com/badge.svg">
    </a>
</p>

<h4 align="center">
  <a href="https://ragflow.io/docs/dev/">Dokumentasi</a> |
  <a href="https://github.com/infiniflow/ragflow/issues/4214">Peta Jalan</a> |
  <a href="https://twitter.com/infiniflowai">Twitter</a> |
  <a href="https://discord.gg/NjYzJD3GM3">Discord</a> |
  <a href="https://demo.ragflow.io">Demo</a>
</h4>

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://raw.githubusercontent.com/infiniflow/ragflow-docs/refs/heads/image/image/ragflow-octoverse.png" width="1200"/>
</div>

<div align="center">
<a href="https://trendshift.io/repositories/9064" target="_blank"><img src="https://trendshift.io/api/badge/repositories/9064" alt="infiniflow%2Fragflow | Trendshift" style="width: 250px; height: 55px;" width="250" height="55"/></a>
</div>

<details open>
<summary><b>📕 Daftar Isi </b> </summary>

- 💡 [Apa Itu RAGFlow?](#-apa-itu-ragflow)
- 🎮 [Demo](#-demo)
- 📌 [Pembaruan Terbaru](#-pembaruan-terbaru)
- 🌟 [Fitur Utama](#-fitur-utama)
- 🔎 [Arsitektur Sistem](#-arsitektur-sistem)
- 🎬 [Mulai](#-mulai)
- 🔧 [Konfigurasi](#-konfigurasi)
- 🔧 [Membangun Image Docker](#-membangun-docker-image)
- 🔨 [Meluncurkan aplikasi dari Sumber untuk Pengembangan](#-meluncurkan-aplikasi-dari-sumber-untuk-pengembangan)
- 📚 [Dokumentasi](#-dokumentasi)
- 📜 [Peta Jalan](#-peta-jalan)
- 🏄 [Komunitas](#-komunitas)
- 🙌 [Kontribusi](#-kontribusi)

</details>

## 💡 Apa Itu RAGFlow?

[RAGFlow](https://ragflow.io/) adalah mesin RAG (Retrieval-Augmented Generation) open-source terkemuka yang mengintegrasikan teknologi RAG mutakhir dengan kemampuan Agent untuk menciptakan lapisan kontekstual superior bagi LLM. Menyediakan alur kerja RAG yang efisien dan dapat diadaptasi untuk perusahaan segala skala. Didukung oleh mesin konteks terkonvergensi dan template Agent yang telah dipra-bangun, RAGFlow memungkinkan pengembang mengubah data kompleks menjadi sistem AI kesetiaan-tinggi dan siap-produksi dengan efisiensi dan presisi yang luar biasa.

## 🎮 Demo

Coba demo kami di [https://demo.ragflow.io](https://demo.ragflow.io).

<div align="center" style="margin-top:20px;margin-bottom:20px;">
<img src="https://raw.githubusercontent.com/infiniflow/ragflow-docs/refs/heads/image/image/chunking.gif" width="1200"/>
<img src="https://raw.githubusercontent.com/infiniflow/ragflow-docs/refs/heads/image/image/agentic-dark.gif" width="1200"/>
</div>

## 🔥 Pembaruan Terbaru

- 2025-11-19 Mendukung Gemini 3 Pro.
- 2025-11-12 Mendukung sinkronisasi data dari Confluence, AWS S3, Discord, Google Drive.
- 2025-10-23 Mendukung MinerU & Docling sebagai metode penguraian dokumen.
- 2025-10-15 Dukungan untuk jalur data yang terorkestrasi.
- 2025-08-08 Mendukung model seri GPT-5 terbaru dari OpenAI.
- 2025-08-01 Mendukung alur kerja agen dan MCP.
- 2025-05-23 Menambahkan komponen pelaksana kode Python/JS ke Agen.
- 2025-05-05 Mendukung kueri lintas bahasa.
- 2025-03-19 Mendukung penggunaan model multi-modal untuk memahami gambar di dalam file PDF atau DOCX.
- 2024-12-18 Meningkatkan model Analisis Tata Letak Dokumen di DeepDoc.
- 2024-08-22 Dukungan untuk teks ke pernyataan SQL melalui RAG.

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
<img src="https://github.com/user-attachments/assets/31b0dd6f-ca4f-445a-9457-70cb44a381b2" width="1000"/>
</div>

## 🎬 Mulai

### 📝 Prasyarat

- CPU >= 4 inti
- RAM >= 16 GB
- Disk >= 50 GB
- Docker >= 24.0.0 & Docker Compose >= v2.26.1
- [gVisor](https://gvisor.dev/docs/user_guide/install/): Hanya diperlukan jika Anda ingin menggunakan fitur eksekutor kode (sandbox) dari RAGFlow.

> [!TIP]
> Jika Anda belum menginstal Docker di komputer lokal Anda (Windows, Mac, atau Linux), lihat [Install Docker Engine](https://docs.docker.com/engine/install/).

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
   > `vm.max_map_count` di **/etc/sysctl.conf**:
   >
   > ```bash
   > vm.max_map_count=262144
   > ```
   >
2. Clone repositori:

   ```bash
   $ git clone https://github.com/infiniflow/ragflow.git
   ```
3. Bangun image Docker pre-built dan jalankan server:

> [!CAUTION]
> Semua gambar Docker dibangun untuk platform x86. Saat ini, kami tidak menawarkan gambar Docker untuk ARM64.
> Jika Anda menggunakan platform ARM64, [silakan gunakan panduan ini untuk membangun gambar Docker yang kompatibel dengan sistem Anda](https://ragflow.io/docs/dev/build_docker_image).

> Perintah di bawah ini mengunduh edisi v0.22.1 dari gambar Docker RAGFlow. Silakan merujuk ke tabel berikut untuk deskripsi berbagai edisi RAGFlow. Untuk mengunduh edisi RAGFlow yang berbeda dari v0.22.1, perbarui variabel RAGFLOW_IMAGE di docker/.env sebelum menggunakan docker compose untuk memulai server.

```bash
   $ cd ragflow/docker
   
   # git checkout v0.22.1
   # Opsional: gunakan tag stabil (lihat releases: https://github.com/infiniflow/ragflow/releases)
   # This steps ensures the **entrypoint.sh** file in the code matches the Docker image version.

   # Use CPU for DeepDoc tasks:
   $ docker compose -f docker-compose.yml up -d

   # To use GPU to accelerate DeepDoc tasks:
   # sed -i '1i DEVICE=gpu' .env
   # docker compose -f docker-compose.yml up -d
```

> Catatan: Sebelum `v0.22.0`, kami menyediakan image dengan model embedding dan image slim tanpa model embedding. Detailnya sebagai berikut:

| RAGFlow image tag | Image size (GB) | Has embedding models? | Stable?                  |
| ----------------- | --------------- | --------------------- | ------------------------ |
| v0.21.1           | &approx;9       | ✔️                    | Stable release           |
| v0.21.1-slim      | &approx;2       | ❌                    | Stable release           |

> Mulai dari `v0.22.0`, kami hanya menyediakan edisi slim dan tidak lagi menambahkan akhiran **-slim** pada tag image.

1. Periksa status server setelah server aktif dan berjalan:

   ```bash
   $ docker logs -f docker-ragflow-cpu-1
   ```

   _Output berikut menandakan bahwa sistem berhasil diluncurkan:_

   ```bash

         ____   ___    ______ ______ __
        / __ \ /   |  / ____// ____// /____  _      __
       / /_/ // /| | / / __ / /_   / // __ \| | /| / /
      / _, _// ___ |/ /_/ // __/  / // /_/ /| |/ |/ /
     /_/ |_|/_/  |_|\____//_/    /_/ \____/ |__/|__/

    * Running on all addresses (0.0.0.0)
   ```

   > Jika Anda melewatkan langkah ini dan langsung login ke RAGFlow, browser Anda mungkin menampilkan error `network anormal`
   > karena RAGFlow mungkin belum sepenuhnya siap.
   >
2. Buka browser web Anda, masukkan alamat IP server Anda, dan login ke RAGFlow.

   > Dengan pengaturan default, Anda hanya perlu memasukkan `http://IP_DEVICE_ANDA` (**tanpa** nomor port) karena
   > port HTTP default `80` bisa dihilangkan saat menggunakan konfigurasi default.
   >
3. Dalam [service_conf.yaml.template](./docker/service_conf.yaml.template), pilih LLM factory yang diinginkan di `user_default_llm` dan perbarui
   bidang `API_KEY` dengan kunci API yang sesuai.

   > Lihat [llm_api_key_setup](https://ragflow.io/docs/dev/llm_api_key_setup) untuk informasi lebih lanjut.
   >

   _Sistem telah siap digunakan!_

## 🔧 Konfigurasi

Untuk konfigurasi sistem, Anda perlu mengelola file-file berikut:

- [.env](./docker/.env): Menyimpan pengaturan dasar sistem, seperti `SVR_HTTP_PORT`, `MYSQL_PASSWORD`, dan
  `MINIO_PASSWORD`.
- [service_conf.yaml.template](./docker/service_conf.yaml.template): Mengonfigurasi aplikasi backend.
- [docker-compose.yml](./docker/docker-compose.yml): Sistem ini bergantung pada [docker-compose.yml](./docker/docker-compose.yml) untuk memulai.

Untuk memperbarui port HTTP default (80), buka [docker-compose.yml](./docker/docker-compose.yml) dan ubah `80:80`
menjadi `<YOUR_SERVING_PORT>:80`.

Pembaruan konfigurasi ini memerlukan reboot semua kontainer agar efektif:

> ```bash
> $ docker compose -f docker-compose.yml up -d
> ```

## 🔧 Membangun Docker Image

Image ini berukuran sekitar 2 GB dan bergantung pada aplikasi LLM eksternal dan embedding.

```bash
git clone https://github.com/infiniflow/ragflow.git
cd ragflow/
docker build --platform linux/amd64 -f Dockerfile -t infiniflow/ragflow:nightly .
```

## 🔨 Menjalankan Aplikasi dari untuk Pengembangan

1. Instal `uv` dan `pre-commit`, atau lewati langkah ini jika sudah terinstal:

   ```bash
   pipx install uv pre-commit
   ```
2. Clone kode sumber dan instal dependensi Python:

   ```bash
   git clone https://github.com/infiniflow/ragflow.git
   cd ragflow/
   uv sync --python 3.10 # install RAGFlow dependent python modules
   uv run download_deps.py
   pre-commit install
   ```
3. Jalankan aplikasi yang diperlukan (MinIO, Elasticsearch, Redis, dan MySQL) menggunakan Docker Compose:

   ```bash
   docker compose -f docker/docker-compose-base.yml up -d
   ```

   Tambahkan baris berikut ke `/etc/hosts` untuk memetakan semua host yang ditentukan di **conf/service_conf.yaml** ke `127.0.0.1`:

   ```
   127.0.0.1       es01 infinity mysql minio redis sandbox-executor-manager
   ```
4. Jika Anda tidak dapat mengakses HuggingFace, atur variabel lingkungan `HF_ENDPOINT` untuk menggunakan situs mirror:

   ```bash
   export HF_ENDPOINT=https://hf-mirror.com
   ```
5. Jika sistem operasi Anda tidak memiliki jemalloc, instal sebagai berikut:

   ```bash
   # ubuntu
   sudo apt-get install libjemalloc-dev
   # centos
   sudo yum install jemalloc
   # mac
   sudo brew install jemalloc
   ```
6. Jalankan aplikasi backend:

   ```bash
   source .venv/bin/activate
   export PYTHONPATH=$(pwd)
   bash docker/launch_backend_service.sh
   ```
7. Instal dependensi frontend:

   ```bash
   cd web
   npm install
   ```
8. Jalankan aplikasi frontend:

   ```bash
   npm run dev
   ```

   _Output berikut menandakan bahwa sistem berhasil diluncurkan:_

   ![](https://github.com/user-attachments/assets/0daf462c-a24d-4496-a66f-92533534e187)
9. Hentikan layanan front-end dan back-end RAGFlow setelah pengembangan selesai:

   ```bash
   pkill -f "ragflow_server.py|task_executor.py"
   ```

## 📚 Dokumentasi

- [Quickstart](https://ragflow.io/docs/dev/)
- [Configuration](https://ragflow.io/docs/dev/configurations)
- [Release notes](https://ragflow.io/docs/dev/release_notes)
- [User guides](https://ragflow.io/docs/dev/category/guides)
- [Developer guides](https://ragflow.io/docs/dev/category/developers)
- [References](https://ragflow.io/docs/dev/category/references)
- [FAQs](https://ragflow.io/docs/dev/faq)

## 📜 Roadmap

Lihat [Roadmap RAGFlow 2025](https://github.com/infiniflow/ragflow/issues/4214)

## 🏄 Komunitas

- [Discord](https://discord.gg/NjYzJD3GM3)
- [Twitter](https://twitter.com/infiniflowai)
- [GitHub Discussions](https://github.com/orgs/infiniflow/discussions)

## 🙌 Kontribusi

RAGFlow berkembang melalui kolaborasi open-source. Dalam semangat ini, kami menerima kontribusi dari komunitas.
Jika Anda ingin berpartisipasi, tinjau terlebih dahulu [Panduan Kontribusi](https://ragflow.io/docs/dev/contributing).
