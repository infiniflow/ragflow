English | [简体中文](./README_zh.md)

# *Deep*Doc

- [1. Introduction](#1)
- [2. Vision](#2)
- [3. Parser](#3)

<a name="1"></a>
## 1. Introduction

With a bunch of documents from various domains with various formats and along with diverse retrieval requirements, 
an accurate analysis becomes a very challenge task. *Deep*Doc is born for that purpose.
There are 2 parts in *Deep*Doc so far: vision and parser. 
You can run the flowing test programs if you're interested in our results of OCR, layout recognition and TSR.
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

Our models are served on HuggingFace. If you have trouble downloading HuggingFace models, this might help!!
```bash
export HF_ENDPOINT=https://hf-mirror.com
```

<a name="2"></a>
## 2. Vision

We use vision information to resolve problems as human being.
  - OCR. Since a lot of documents presented as images or at least be able to transform to image, 
    OCR is a very essential and fundamental or even universal solution for text extraction.
    ```bash
        python deepdoc/vision/t_ocr.py --inputs=path_to_images_or_pdfs --output_dir=path_to_store_result
     ```
    The inputs could be directory to images or PDF, or a image or PDF. 
    You can look into the folder 'path_to_store_result' where has images which demonstrate the positions of results,
    txt files which contain the OCR text.
    <div align="center" style="margin-top:20px;margin-bottom:20px;">
    <img src="https://github.com/infiniflow/ragflow/assets/12318111/f25bee3d-aaf7-4102-baf5-d5208361d110" width="900"/>
    </div>

  - Layout recognition. Documents from different domain may have various layouts, 
    like, newspaper, magazine, book and résumé are distinct in terms of layout. 
    Only when machine have an accurate layout analysis, it can decide if these text parts are successive or not, 
    or this part needs Table Structure Recognition(TSR) to process, or this part is a figure and described with this caption.
    We have 10 basic layout components which covers most cases:
      - Text
      - Title
      - Figure
      - Figure caption
      - Table
      - Table caption
      - Header
      - Footer
      - Reference
      - Equation
      
     Have a try on the following command to see the layout detection results.
     ```bash
        python deepdoc/vision/t_recognizer.py --inputs=path_to_images_or_pdfs --threshold=0.2 --mode=layout --output_dir=path_to_store_result
     ```
    The inputs could be directory to images or PDF, or a image or PDF. 
    You can look into the folder 'path_to_store_result' where has images which demonstrate the detection results as following:
    <div align="center" style="margin-top:20px;margin-bottom:20px;">
    <img src="https://github.com/infiniflow/ragflow/assets/12318111/07e0f625-9b28-43d0-9fbb-5bf586cd286f" width="1000"/>
    </div>
  
  - Table Structure Recognition(TSR). Data table is a frequently used structure to present data including numbers or text.
    And the structure of a table might be very complex, like hierarchy headers, spanning cells and projected row headers.
    Along with TSR, we also reassemble the content into sentences which could be well comprehended by LLM. 
    We have five labels for TSR task:
      - Column
      - Row
      - Column header
      - Projected row header
      - Spanning cell
      
    Have a try on the following command to see the layout detection results.
     ```bash
        python deepdoc/vision/t_recognizer.py --inputs=path_to_images_or_pdfs --threshold=0.2 --mode=tsr --output_dir=path_to_store_result
     ```
    The inputs could be directory to images or PDF, or a image or PDF. 
    You can look into the folder 'path_to_store_result' where has both images and html pages which demonstrate the detection results as following:
    <div align="center" style="margin-top:20px;margin-bottom:20px;">
    <img src="https://github.com/infiniflow/ragflow/assets/12318111/cb24e81b-f2ba-49f3-ac09-883d75606f4c" width="1000"/>
    </div>
        
<a name="3"></a>
## 3. Parser

Four kinds of document formats as PDF, DOCX, EXCEL and PPT have their corresponding parser. 
The most complex one is PDF parser since PDF's flexibility. The output of PDF parser includes:
  - Text chunks with their own positions in PDF(page number and rectangular positions).
  - Tables with cropped image from the PDF, and contents which has already translated into natural language sentences.
  - Figures with caption and text in the figures.
  
### Résumé

The résumé is a very complicated kind of document. A résumé which is composed of unstructured text 
with various layouts could be resolved into structured data composed of nearly a hundred of fields.
We haven't opened the parser yet, as we open the processing method after parsing procedure.

    