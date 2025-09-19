from PyPDF2 import PdfReader

pdf_path = "/home/bxy/桌面/ABCDE投资合伙企业（有限合伙）托管合同45(5).pdf"

reader = PdfReader(pdf_path)

try:
    outlines = reader.outline  # 部分版本用 reader.getOutlines()
except AttributeError:
    outlines = reader.getOutlines()

print(outlines)