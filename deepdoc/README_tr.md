[English](./README.md) | [简体中文](./README_zh.md) | Türkçe

# *Deep*Doc

- [*Deep*Doc](#deepdoc)
  - [1. Giriş](#1-giriş)
  - [2. Görsel İşleme](#2-görsel-i̇şleme)
  - [3. Ayrıştırıcı](#3-ayrıştırıcı)
    - [Özgeçmiş](#özgeçmiş)

<a name="1"></a>
## 1. Giriş

Farklı alanlardan, farklı formatlarda ve farklı erişim gereksinimleriyle gelen çok sayıda doküman için doğru bir analiz son derece zorlu bir görev haline gelmektedir. *Deep*Doc tam bu amaç için doğmuştur. Şu ana kadar *Deep*Doc'ta iki bileşen bulunmaktadır: görsel işleme ve ayrıştırıcı. OCR, yerleşim tanıma ve TSR sonuçlarımızla ilgileniyorsanız aşağıdaki test programlarını çalıştırabilirsiniz.

```bash
python deepdoc/vision/t_ocr.py -h
usage: t_ocr.py [-h] --inputs INPUTS [--output_dir OUTPUT_DIR]

options:
  -h, --help            show this help message and exit
  --inputs INPUTS       Directory where to store images or PDFs, or a file path to a single image or PDF
  --output_dir OUTPUT_DIR
                        Directory where to store the output images. Default: './ocr_outputs'
```

```bash
python deepdoc/vision/t_recognizer.py -h
usage: t_recognizer.py [-h] --inputs INPUTS [--output_dir OUTPUT_DIR] [--threshold THRESHOLD] [--mode {layout,tsr}]

options:
  -h, --help            show this help message and exit
  --inputs INPUTS       Directory where to store images or PDFs, or a file path to a single image or PDF
  --output_dir OUTPUT_DIR
                        Directory where to store the output images. Default: './layouts_outputs'
  --threshold THRESHOLD
                        A threshold to filter out detections. Default: 0.5
  --mode {layout,tsr}   Task mode: layout recognition or table structure recognition
```

Modellerimiz HuggingFace üzerinden sunulmaktadır. HuggingFace modellerini indirmekte sorun yaşıyorsanız, bu yardımcı olabilir!

```bash
export HF_ENDPOINT=https://hf-mirror.com
```

<a name="2"></a>
## 2. Görsel İşleme

İnsanlar olarak sorunları çözmek için görsel bilgiyi kullanırız.

  - **OCR (Optik Karakter Tanıma)**. Birçok doküman görsel olarak sunulduğundan veya en azından görsele dönüştürülebildiğinden, OCR metin çıkarımı için çok temel, önemli ve hatta evrensel bir çözümdür.
    ```bash
    python deepdoc/vision/t_ocr.py --inputs=gorsel_veya_pdf_yolu --output_dir=sonuc_klasoru
    ```
    Girdi, görseller veya PDF'ler içeren bir dizin ya da tek bir görsel veya PDF dosyası olabilir.
    Sonuçların konumlarını gösteren görsellerin ve OCR metnini içeren txt dosyalarının bulunduğu `sonuc_klasoru` klasörüne bakabilirsiniz.
    <div align="center" style="margin-top:20px;margin-bottom:20px;">
    <img src="https://github.com/infiniflow/ragflow/assets/12318111/f25bee3d-aaf7-4102-baf5-d5208361d110" width="900"/>
    </div>

  - **Yerleşim Tanıma (Layout Recognition)**. Farklı alanlardan gelen dokümanlar farklı yerleşimlere sahip olabilir; gazete, dergi, kitap ve özgeçmiş gibi dokümanlar yerleşim açısından birbirinden farklıdır. Yalnızca makine doğru bir yerleşim analizi yapabildiğinde, metin parçalarının ardışık olup olmadığına, bu parçanın Tablo Yapısı Tanıma (TSR) ile mi işlenmesi gerektiğine veya bu parçanın bir şekil olup bu başlıkla mı açıklandığına karar verebilir.
    Çoğu durumu kapsayan 10 temel yerleşim bileşenimiz vardır:
      - Metin
      - Başlık
      - Şekil
      - Şekil açıklaması
      - Tablo
      - Tablo açıklaması
      - Üst bilgi
      - Alt bilgi
      - Referans
      - Denklem

    Yerleşim algılama sonuçlarını görmek için aşağıdaki komutu deneyin.
    ```bash
    python deepdoc/vision/t_recognizer.py --inputs=gorsel_veya_pdf_yolu --threshold=0.2 --mode=layout --output_dir=sonuc_klasoru
    ```
    Girdi, görseller veya PDF'ler içeren bir dizin ya da tek bir görsel veya PDF dosyası olabilir.
    Aşağıdaki gibi algılama sonuçlarını gösteren görsellerin bulunduğu `sonuc_klasoru` klasörüne bakabilirsiniz:
    <div align="center" style="margin-top:20px;margin-bottom:20px;">
    <img src="https://github.com/infiniflow/ragflow/assets/12318111/07e0f625-9b28-43d0-9fbb-5bf586cd286f" width="1000"/>
    </div>

  - **TSR (Tablo Yapısı Tanıma)**. Veri tablosu, sayılar veya metin dahil verileri sunmak için sıklıkla kullanılan bir yapıdır. Bir tablonun yapısı; hiyerarşik başlıklar, birleştirilmiş hücreler ve yansıtılmış satır başlıkları gibi çok karmaşık olabilir. TSR'nin yanı sıra, içeriği LLM tarafından iyi anlaşılabilecek cümlelere dönüştürüyoruz.
    TSR görevi için beş etiketimiz vardır:
      - Sütun
      - Satır
      - Sütun başlığı
      - Yansıtılmış satır başlığı
      - Birleştirilmiş hücre

    Algılama sonuçlarını görmek için aşağıdaki komutu deneyin.
    ```bash
    python deepdoc/vision/t_recognizer.py --inputs=gorsel_veya_pdf_yolu --threshold=0.2 --mode=tsr --output_dir=sonuc_klasoru
    ```
    Girdi, görseller veya PDF'ler içeren bir dizin ya da tek bir görsel veya PDF dosyası olabilir.
    Algılama sonuçlarını gösteren görsellerin ve HTML sayfalarının bulunduğu `sonuc_klasoru` klasörüne bakabilirsiniz:
    <div align="center" style="margin-top:20px;margin-bottom:20px;">
    <img src="https://github.com/infiniflow/ragflow/assets/12318111/cb24e81b-f2ba-49f3-ac09-883d75606f4c" width="1000"/>
    </div>

  - **Tablo Otomatik Döndürme**. Tabloların yanlış yönde olabileceği (90°, 180° veya 270° döndürülmüş) taranmış PDF'ler için, PDF ayrıştırıcısı tablo yapısı tanıma işleminden önce en iyi döndürme açısını OCR güven puanlarını kullanarak otomatik olarak algılar. Bu, döndürülmüş tablolar için OCR doğruluğunu ve tablo yapısı algılamasını önemli ölçüde artırır.

    Özellik 4 döndürme açısını (0°, 90°, 180°, 270°) değerlendirir ve en yüksek OCR güvenine sahip olanı seçer. En iyi yönlendirmeyi belirledikten sonra, doğru döndürülmüş tablo görseli üzerinde OCR'yi yeniden gerçekleştirir.

    Bu özellik **varsayılan olarak etkindir**. Ortam değişkeni ile kontrol edebilirsiniz:
    ```bash
    # Tablo otomatik döndürmeyi devre dışı bırak
    export TABLE_AUTO_ROTATE=false

    # Tablo otomatik döndürmeyi etkinleştir (varsayılan)
    export TABLE_AUTO_ROTATE=true
    ```

    Veya API parametresi ile:
    ```python
    from deepdoc.parser import PdfParser

    parser = PdfParser()
    # Bu çağrı için otomatik döndürmeyi devre dışı bırak
    boxes, tables = parser(pdf_path, auto_rotate_tables=False)
    ```

<a name="3"></a>
## 3. Ayrıştırıcı

PDF, DOCX, EXCEL ve PPT olmak üzere dört doküman formatının kendine özgü ayrıştırıcısı vardır. En karmaşık olanı, PDF'nin esnekliği nedeniyle PDF ayrıştırıcısıdır. PDF ayrıştırıcısının çıktısı şunları içerir:
  - PDF'deki konumlarıyla birlikte metin parçaları (sayfa numarası ve dikdörtgen konumları).
  - PDF'den kırpılmış görsel ve doğal dil cümlelerine çevrilmiş içerikleriyle tablolar.
  - Açıklama ve şekil içindeki metinlerle birlikte şekiller.

### Özgeçmiş

Özgeçmiş çok karmaşık bir doküman türüdür. Çeşitli yerleşimlere sahip yapılandırılmamış metinden oluşan bir özgeçmiş, yaklaşık yüz alanı kapsayan yapılandırılmış veriye dönüştürülebilir.
Ayrıştırıcıyı henüz açık kaynak olarak yayınlamadık; ayrıştırma prosedüründen sonraki işleme yöntemini açık kaynak olarak sunmaktayız.
