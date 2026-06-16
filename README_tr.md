<div align="center">
  <img src="web/public/logo-light.png" width="200" alt="MetaGrossAI logo">
  <h1>MetaGrossAI</h1>
</div>

## 💡 MetaGrossAI Nedir?
**MetaGrossAI**, LLM'ler için üstün bir bağlam katmanı oluşturmak amacıyla en yeni RAG'i Ajan (Agent) yetenekleriyle birleştiren lider bir Geri Getirme Artırılmış Üretim (RAG) motorudur. Her ölçekteki kuruluşa uyarlanabilen modern bir RAG iş akışı sunar. Birleştirilmiş bir bağlam motoru ve önceden oluşturulmuş ajan şablonlarıyla desteklenen MetaGrossAI, geliştiricilerin karmaşık verileri olağanüstü verimlilik ve hassasiyetle üretime hazır yapay zeka sistemlerine dönüştürmesine olanak tanır.

## 🌟 Temel Özellikler
### 🍭 **"Kaliteli girdi, kaliteli çıktı"**
- Karmaşık formatlara sahip yapılandırılmamış verilerden derin belge anlayışına dayalı bilgi çıkarımı.
- Sınırsız token içerisinden "veri samanlığındaki iğneyi" bulur.

### 🍱 **Şablon tabanlı parçalama (chunking)**
- Akıllı ve açıklanabilir.
- Aralarından seçim yapabileceğiniz çok sayıda şablon seçeneği.

### 🌱 **Azaltılmış halüsinasyonlar ile temellendirilmiş alıntılar**
- İnsan müdahalesine izin vermek için metin parçalamanın görselleştirilmesi.
- Temellendirilmiş cevapları desteklemek için temel referansların ve izlenebilir alıntıların hızlı görünümü.

### 🍔 **Heterojen veri kaynakları ile uyumluluk**
- Word, slaytlar, excel, txt, resimler, taranmış kopyalar, yapılandırılmış veriler, web sayfaları ve daha fazlasını destekler.

## 🎬 Kendi Sunucunda Barındırma (Self-Hosting)
### 📝 Ön Koşullar
- CPU >= 4 cores
- RAM >= 16 GB
- Disk >= 50 GB
- Docker >= 24.0.0 & Docker Compose >= v2.26.1
- Python >= 3.13

### 🚀 Sunucuyu Başlatma
1. `vm.max_map_count` >= 262144 olduğundan emin olun:
   ```bash
   $ sudo sysctl -w vm.max_map_count=262144
