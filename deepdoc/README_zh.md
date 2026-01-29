[English](./README.md) | 简体中文

# *Deep*Doc

- [*Deep*Doc](#deepdoc)
  - [1. 介绍](#1-介绍)
  - [2. 视觉处理](#2-视觉处理)
  - [3. 解析器](#3-解析器)
    - [简历](#简历)

<a name="1"></a>
## 1. 介绍

对于来自不同领域、具有不同格式和不同检索要求的大量文档，准确的分析成为一项极具挑战性的任务。*Deep*Doc 就是为了这个目的而诞生的。到目前为止，*Deep*Doc 中有两个组成部分：视觉处理和解析器。如果您对我们的OCR、布局识别和TSR结果感兴趣，您可以运行下面的测试程序。

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

HuggingFace为我们的模型提供服务。如果你在下载HuggingFace模型时遇到问题，这可能会有所帮助！！

```bash
export HF_ENDPOINT=https://hf-mirror.com
```

<a name="2"></a>
## 2. 视觉处理

作为人类，我们使用视觉信息来解决问题。

  - **OCR（Optical Character Recognition，光学字符识别）**。由于许多文档都是以图像形式呈现的，或者至少能够转换为图像，因此OCR是文本提取的一个非常重要、基本，甚至通用的解决方案。

    ```bash
    python deepdoc/vision/t_ocr.py --inputs=path_to_images_or_pdfs --output_dir=path_to_store_result
    ```

    输入可以是图像或PDF的目录，或者单个图像、PDF文件。您可以查看文件夹 `path_to_store_result` ，其中有演示结果位置的图像，以及包含OCR文本的txt文件。
    
    <div align="center" style="margin-top:20px;margin-bottom:20px;">
    <img src="https://github.com/infiniflow/ragflow/assets/12318111/f25bee3d-aaf7-4102-baf5-d5208361d110" width="900"/>
    </div>

  - 布局识别（Layout recognition）。来自不同领域的文件可能有不同的布局，如报纸、杂志、书籍和简历在布局方面是不同的。只有当机器有准确的布局分析时，它才能决定这些文本部分是连续的还是不连续的，或者这个部分需要表结构识别（Table Structure Recognition，TSR）来处理，或者这个部件是一个图形并用这个标题来描述。我们有10个基本布局组件，涵盖了大多数情况：
      - 文本
      - 标题
      - 配图
      - 配图标题
      - 表格
      - 表格标题
      - 页头
      - 页尾
      - 参考引用
      - 公式
      
     请尝试以下命令以查看布局检测结果。

    ```bash
    python deepdoc/vision/t_recognizer.py --inputs=path_to_images_or_pdfs --threshold=0.2 --mode=layout --output_dir=path_to_store_result
    ```

    输入可以是图像或PDF的目录，或者单个图像、PDF文件。您可以查看文件夹 `path_to_store_result` ，其中有显示检测结果的图像，如下所示：
    <div align="center" style="margin-top:20px;margin-bottom:20px;">
    <img src="https://github.com/infiniflow/ragflow/assets/12318111/07e0f625-9b28-43d0-9fbb-5bf586cd286f" width="1000"/>
    </div>
  
  - **TSR（Table Structure Recognition，表结构识别）**。数据表是一种常用的结构，用于表示包括数字或文本在内的数据。表的结构可能非常复杂，比如层次结构标题、跨单元格和投影行标题。除了TSR，我们还将内容重新组合成LLM可以很好理解的句子。TSR任务有五个标签：
      - 列
      - 行
      - 列标题
      - 行标题
      - 合并单元格
      
    请尝试以下命令以查看布局检测结果。

    ```bash
    python deepdoc/vision/t_recognizer.py --inputs=path_to_images_or_pdfs --threshold=0.2 --mode=tsr --output_dir=path_to_store_result
    ```

    输入可以是图像或PDF的目录，或者单个图像、PDF文件。您可以查看文件夹 `path_to_store_result` ，其中包含图像和html页面，这些页面展示了以下检测结果：

    <div align="center" style="margin-top:20px;margin-bottom:20px;">
    <img src="https://github.com/infiniflow/ragflow/assets/12318111/cb24e81b-f2ba-49f3-ac09-883d75606f4c" width="1000"/>
    </div>

  - **表格自动旋转（Table Auto-Rotation）**。对于扫描的 PDF 文档，表格可能存在方向错误（旋转了 90°、180° 或 270°），
    PDF 解析器会在进行表格结构识别之前，自动使用 OCR 置信度来检测最佳旋转角度。这大大提高了旋转表格的 OCR 准确性和表格结构检测效果。
    
    该功能会评估 4 个旋转角度（0°、90°、180°、270°），并选择 OCR 置信度最高的角度。
    确定最佳方向后，会对旋转后的表格图像重新进行 OCR 识别。
    
    此功能**默认启用**。您可以通过环境变量控制：
    ```bash
    # 禁用表格自动旋转
    export TABLE_AUTO_ROTATE=false
    
    # 启用表格自动旋转（默认）
    export TABLE_AUTO_ROTATE=true
    ```
    
    或通过 API 参数控制：
    ```python
    from deepdoc.parser import PdfParser
    
    parser = PdfParser()
    # 禁用此次调用的自动旋转
    boxes, tables = parser(pdf_path, auto_rotate_tables=False)
    ```
        
<a name="3"></a>
## 3. 解析器

PDF、DOCX、EXCEL和PPT四种文档格式都有相应的解析器。最复杂的是PDF解析器，因为PDF具有灵活性。PDF解析器的输出包括：
  - 在PDF中有自己位置的文本块（页码和矩形位置）。
  - 带有PDF裁剪图像的表格，以及已经翻译成自然语言句子的内容。
  - 图中带标题和文字的图。
  
### 简历

简历是一种非常复杂的文档。由各种格式的非结构化文本构成的简历可以被解析为包含近百个字段的结构化数据。我们还没有启用解析器，因为在解析过程之后才会启动处理方法。
