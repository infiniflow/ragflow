import json
import logging

class RemoteMinerUParser:
    def __init__(self):
        self.endpoint  = "http://512jx20fv834.vicp.fun/file_parse"
        self.logger = logging.getLogger(__name__)

    def __call__(self, filename=None, binary=None, from_page=0, to_page=100000, callback=None):
        self.logger.info("Parsing started."+filename)
        try:
            if filename is None:
                raise ValueError("Filename must be provided for parsing.")

            # 构建 JSON 文件路径
            json_filename = os.path.splitext(filename)[0] + ".json"
            with open(json_filename, 'r', encoding='utf-8') as f:
                data = json.load(f)

            sections = []
            tables = []
            # 遍历 pdf_info
            for page in data.get("pdf_info", []):
                # 获取 para_blocks 列表
                blocks = page.get("para_blocks", [])
                i = 0
                while i < len(blocks):
                    block = blocks[i]
                    block_type = block.get("type")
                    block_bbox = block.get("bbox")
                    page_num = block.get("page_num")
                    if not block_bbox:
                        i += 1
                        continue

                    # 将 bbox 坐标转换为相对于页面的比例
                    x0, y0, x1, y1 = block_bbox
                    pn = int(page_num.split("_")[1])
                    pos_str = "@@{pn}\t{x0:.1f}\t{x1:.1f}\t{y0:.1f}\t{y1:.1f}##".format(
                        pn=pn, x0=x0, x1=x1, y0=y0, y1=y1
                    )
                    if block_type == "table":
                        html_content = ""
                        for table_block in block.get("blocks", []):
                            for line in table_block.get("lines", []):
                                for span in line.get("spans", []):
                                    html_content += span.get("new_html", "")
                        if html_content:
                            # 使用占位符图像对象，实际应从 block 提取图像数据
                            tables.append(((None, html_content), [(pn, x0, x1, y0, y1)]))
                        i += 1
                    else:
                        # 处理无标题的独立段落
                        content_text = ""
                        for line in block.get("lines", []):
                            for span in line.get("spans", []):
                                text_content = span.get("content")
                                if text_content:
                                    content_text += text_content + "\n"
                        if content_text:
                            sections.append((content_text, pos_str))
                        i += 1

            self.logger.info("Sections: %s", sections)
            self.logger.info("Tables: %s", tables)

            return sections, tables
        except Exception as e:
            self.logger.exception("An error occurred during parsing: %s", str(e))

    def crop(self, text_with_tag, need_position=True):
        self.logger.info("Crop method called with text_with_tag: %s", text_with_tag)
        import re
        # 提取 @@pn\tx0\tx1\ty0\ty1## 标签
        matches = re.findall(r"@@([\d\.]+)\t([\d\.]+)\t([\d\.]+)\t([\d\.]+)\t([\d\.]+)##", text_with_tag)

        if not matches and need_position:
            self.logger.warning("No matches found and need_position is True.")
            return None, []

        # 返回空图像和位置信息
        from PIL import Image
        dummy_image = Image.new("RGB", (1, 1), color="white")

        positions = []
        for pn, x0, x1, y0, y1 in matches:
            positions.append((
                int(pn),
                float(x0),
                float(x1),
                float(y0),
                float(y1)
            ))

        if need_position:
            return dummy_image, positions
        return dummy_image