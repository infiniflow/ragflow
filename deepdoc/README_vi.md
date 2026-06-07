Tiếng Anh | [简体中文](./README_zh.md) | [Tiếng Việt](./README_vi.md)

# *Deep*Doc

- [1. Giới thiệu](#1)
- [2. Vision (Thị giác)](#2)
- [3. Parser (Bộ phân tích)](#3)

<a name="1"></a>
## 1. Giới thiệu

Với một lượng lớn tài liệu từ nhiều lĩnh vực khác nhau, ở nhiều định dạng đa dạng cùng những yêu cầu truy xuất phong phú, việc phân tích chính xác trở thành một thách thức rất lớn. *Deep*Doc ra đời nhằm mục đích đó.

Hiện tại, *Deep*Doc gồm 2 phần: **vision (thị giác)** và **parser (bộ phân tích)**. Bạn có thể chạy các chương trình kiểm thử dưới đây nếu quan tâm đến kết quả OCR, nhận dạng bố cục (layout recognition) và TSR của chúng tôi.

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

Các mô hình của chúng tôi được phục vụ trên HuggingFace. Nếu bạn gặp khó khăn khi tải mô hình từ HuggingFace, lệnh sau có thể hữu ích:

```bash
export HF_ENDPOINT=https://hf-mirror.com
```

<a name="2"></a>
## 2. Vision (Thị giác)

Chúng tôi sử dụng thông tin thị giác để giải quyết vấn đề theo cách con người xử lý.

- **OCR (Nhận dạng ký tự quang học).** Vì rất nhiều tài liệu được trình bày dưới dạng hình ảnh hoặc ít nhất có thể chuyển đổi thành hình ảnh, OCR là giải pháp cơ bản, thiết yếu và thậm chí là phổ quát cho việc trích xuất văn bản.

    ```bash
    python deepdoc/vision/t_ocr.py --inputs=path_to_images_or_pdfs --output_dir=path_to_store_result
    ```

    Đầu vào có thể là thư mục chứa hình ảnh/PDF, hoặc đường dẫn đến một hình ảnh/PDF cụ thể. Bạn có thể xem thư mục `path_to_store_result`, nơi chứa các hình ảnh minh họa vị trí của kết quả và các tệp txt chứa văn bản OCR.

    <div align="center" style="margin-top:20px;margin-bottom:20px;">
    <img src="https://github.com/infiniflow/ragflow/assets/12318111/f25bee3d-aaf7-4102-baf5-d5208361d110" width="900"/>
    </div>

- **Nhận dạng bố cục (Layout recognition).** Các tài liệu từ những lĩnh vực khác nhau có thể có bố cục rất đa dạng — chẳng hạn như báo, tạp chí, sách và sơ yếu lý lịch đều khác biệt rõ rệt về bố cục. Chỉ khi máy có phân tích bố cục chính xác, nó mới có thể quyết định được liệu các phần văn bản này có liên tiếp nhau hay không, hay phần này cần được xử lý bằng Nhận dạng cấu trúc bảng (TSR), hay phần này là hình ảnh và được mô tả bằng chú thích đi kèm.

    Chúng tôi có **10 thành phần bố cục cơ bản** bao phủ hầu hết các trường hợp:
      - Text (Văn bản)
      - Title (Tiêu đề)
      - Figure (Hình ảnh)
      - Figure caption (Chú thích hình ảnh)
      - Table (Bảng)
      - Table caption (Chú thích bảng)
      - Header (Đầu trang)
      - Footer (Chân trang)
      - Reference (Tham chiếu)
      - Equation (Phương trình)

    Hãy thử lệnh sau để xem kết quả phát hiện bố cục:

    ```bash
    python deepdoc/vision/t_recognizer.py --inputs=path_to_images_or_pdfs --threshold=0.2 --mode=layout --output_dir=path_to_store_result
    ```

    Đầu vào có thể là thư mục chứa hình ảnh/PDF, hoặc đường dẫn đến một hình ảnh/PDF cụ thể. Bạn có thể xem thư mục `path_to_store_result`, nơi chứa các hình ảnh minh họa kết quả phát hiện như sau:

    <div align="center" style="margin-top:20px;margin-bottom:20px;">
    <img src="https://github.com/infiniflow/ragflow/assets/12318111/07e0f625-9b28-43d0-9fbb-5bf586cd286f" width="1000"/>
    </div>

- **Nhận dạng cấu trúc bảng (TSR — Table Structure Recognition).** Bảng dữ liệu là một cấu trúc thường được sử dụng để trình bày dữ liệu bao gồm số hoặc văn bản. Cấu trúc của một bảng có thể rất phức tạp, chẳng hạn như tiêu đề phân cấp, ô trải dài nhiều cột và tiêu đề hàng chiếu. Cùng với TSR, chúng tôi cũng tái cấu trúc nội dung thành các câu mà LLM có thể hiểu rõ.

    Chúng tôi có **5 nhãn** cho tác vụ TSR:
      - Column (Cột)
      - Row (Hàng)
      - Column header (Tiêu đề cột)
      - Projected row header (Tiêu đề hàng chiếu)
      - Spanning cell (Ô trải dài)

    Hãy thử lệnh sau để xem kết quả phát hiện:

    ```bash
    python deepdoc/vision/t_recognizer.py --inputs=path_to_images_or_pdfs --threshold=0.2 --mode=tsr --output_dir=path_to_store_result
    ```

    Đầu vào có thể là thư mục chứa hình ảnh/PDF, hoặc đường dẫn đến một hình ảnh/PDF cụ thể. Bạn có thể xem thư mục `path_to_store_result`, nơi chứa cả hình ảnh và trang HTML minh họa kết quả phát hiện như sau:

    <div align="center" style="margin-top:20px;margin-bottom:20px;">
    <img src="https://github.com/infiniflow/ragflow/assets/12318111/cb24e81b-f2ba-49f3-ac09-883d75606f4c" width="1000"/>
    </div>

- **Tự động xoay bảng (Table Auto-Rotation).** Đối với các tệp PDF được quét mà bảng có thể bị xoay sai hướng (xoay 90°, 180° hoặc 270°), bộ phân tích PDF sẽ tự động phát hiện góc xoay tốt nhất bằng cách sử dụng điểm tin cậy OCR trước khi thực hiện nhận dạng cấu trúc bảng. Điều này giúp cải thiện đáng kể độ chính xác của OCR và khả năng phát hiện cấu trúc bảng cho các bảng bị xoay.

    Tính năng này đánh giá 4 góc xoay (0°, 90°, 180°, 270°) và chọn góc có điểm tin cậy OCR cao nhất. Sau khi xác định được hướng tốt nhất, hệ thống sẽ thực hiện lại OCR trên hình ảnh bảng đã được xoay đúng hướng.

    Tính năng này **được bật mặc định**. Bạn có thể kiểm soát thông qua biến môi trường:

    ```bash
    # Tắt tự động xoay bảng
    export TABLE_AUTO_ROTATE=false

    # Bật tự động xoay bảng (mặc định)
    export TABLE_AUTO_ROTATE=true
    ```

    Hoặc thông qua tham số API:

    ```python
    from deepdoc.parser import PdfParser

    parser = PdfParser()
    # Tắt tự động xoay cho lần gọi này
    boxes, tables = parser(pdf_path, auto_rotate_tables=False)
    ```

<a name="3"></a>
## 3. Parser (Bộ phân tích)

Bốn loại định dạng tài liệu PDF, DOCX, EXCEL và PPT có bộ phân tích tương ứng. Phức tạp nhất là bộ phân tích PDF do tính linh hoạt của định dạng PDF. Đầu ra của bộ phân tích PDF bao gồm:

- Các đoạn văn bản kèm vị trí của chúng trong PDF (số trang và tọa độ hình chữ nhật).
- Các bảng với hình ảnh được cắt từ PDF và nội dung đã được chuyển thành câu ngôn ngữ tự nhiên.
- Các hình ảnh với chú thích và văn bản bên trong hình ảnh.

### Sơ yếu lý lịch (Résumé)

Sơ yếu lý lịch là một loại tài liệu rất phức tạp. Một bản sơ yếu lý lịch được cấu thành từ văn bản phi cấu trúc với nhiều bố cục khác nhau có thể được phân giải thành dữ liệu có cấu trúc gồm gần một trăm trường. Chúng tôi chưa mở mã nguồn bộ phân tích này; phương pháp xử lý sẽ được mở sau khi quy trình phân tích hoàn tất.
