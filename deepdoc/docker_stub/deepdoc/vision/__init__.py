# Minimal deepdoc.vision __init__ for Docker — avoids pdfplumber and common imports.
from .ocr import OCR
from .recognizer import Recognizer
from .layout_recognizer import LayoutRecognizer4YOLOv10 as LayoutRecognizer
from .table_structure_recognizer import TableStructureRecognizer

__all__ = ["OCR", "Recognizer", "LayoutRecognizer", "TableStructureRecognizer"]
