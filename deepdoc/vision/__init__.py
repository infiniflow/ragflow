
from .ocr import OCR
from .recognizer import Recognizer
from .layout_recognizer import LayoutRecognizer
from .table_structure_recognizer import TableStructureRecognizer

def init_in_out(args):
    from PIL import Image
    import fitz
    import os
    import traceback
    from api.utils.file_utils import traversal_files
    images = []
    outputs = []

    if not os.path.exists(args.output_dir):
        os.mkdir(args.output_dir)

    def pdf_pages(fnm, zoomin=3):
        nonlocal outputs, images
        pdf = fitz.open(fnm)
        mat = fitz.Matrix(zoomin, zoomin)
        for i, page in enumerate(pdf):
            pix = page.get_pixmap(matrix=mat)
            img = Image.frombytes("RGB", [pix.width, pix.height],
                                  pix.samples)
            images.append(img)
            outputs.append(os.path.split(fnm)[-1] + f"_{i}.jpg")

    def images_and_outputs(fnm):
        nonlocal outputs, images
        if fnm.split(".")[-1].lower() == "pdf":
            pdf_pages(fnm)
            return
        try:
            images.append(Image.open(fnm))
            outputs.append(os.path.split(fnm)[-1])
        except Exception as e:
            traceback.print_exc()

    if os.path.isdir(args.inputs):
        for fnm in traversal_files(args.inputs):
            images_and_outputs(fnm)
    else:
        images_and_outputs(args.inputs)

    for i in range(len(outputs)): outputs[i] = os.path.join(args.output_dir, outputs[i])

    return images, outputs