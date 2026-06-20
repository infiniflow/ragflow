<div align="center">
  <img src="web/public/logo-light.png" width="200" alt="MetaGrossAI logo">
  <h1>MetaGrossAI</h1>
</div>

## 💡 Apa itu MetaGrossAI?
**MetaGrossAI** adalah mesin Retrieval-Augmented Generation (RAG) terkemuka yang memadukan RAG mutakhir dengan kemampuan Agen untuk menciptakan lapisan konteks yang unggul untuk LLM. Ini menawarkan alur kerja RAG yang disederhanakan yang dapat diadaptasi untuk perusahaan skala apa pun. Didayai oleh mesin konteks yang terkonvergensi dan templat agen bawaan, MetaGrossAI memungkinkan pengembang untuk mengubah data kompleks menjadi sistem AI yang siap produksi dan presisi tinggi dengan efisiensi luar biasa.

## 🌟 Fitur Utama
### 🍭 **"Kualitas masuk, kualitas keluar"**
- Ekstraksi pengetahuan berbasis pemahaman dokumen mendalam dari data tidak terstruktur dengan format rumit.
- Menemukan "jarum di tumpukan jerami data" dari token yang jumlahnya tidak terbatas.

### 🍱 **Pemotongan (chunking) berbasis templat**
- Cerdas dan dapat dijelaskan.
- Banyak pilihan templat untuk dipilih.

### 🌱 **Kutipan beralasan dengan pengurangan halusinasi**
- Visualisasi pemotongan teks untuk memungkinkan intervensi manusia.
- Tampilan cepat referensi utama dan kutipan yang dapat dilacak untuk mendukung jawaban yang beralasan.

### 🍔 **Kompatibilitas dengan sumber data heterogen**
- Mendukung Word, slide, excel, txt, gambar, salinan pindaian, data terstruktur, halaman web, dan banyak lagi.

## 🎬 Self-Hosting
### 📝 Prasyarat
- CPU >= 4 cores
- RAM >= 16 GB
- Disk >= 50 GB
- Docker >= 24.0.0 & Docker Compose >= v2.26.1
- Python >= 3.13

### 🚀 Memulai Server
1. Pastikan `vm.max_map_count` >= 262144:
   ```bash
   $ sudo sysctl -w vm.max_map_count=262144
