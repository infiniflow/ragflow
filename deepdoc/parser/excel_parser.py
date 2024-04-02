# -*- coding: utf-8 -*-
from openpyxl import load_workbook
import sys
from io import BytesIO


class HuExcelParser:
    def html(self, fnm):
        if isinstance(fnm, str):
            wb = load_workbook(fnm)
        else:
            wb = load_workbook(BytesIO(fnm))
        tb = ""
        for sheetname in wb.sheetnames:
            ws = wb[sheetname]
            rows = list(ws.rows)
            if not rows:continue
            tb += f"<table><caption>{sheetname}</caption><tr>"
            for t in list(rows[0]):
                tb += f"<th>{t.value}</th>"
            tb += "</tr>"
            for r in list(rows[1:]):
                tb += "<tr>"
                for i, c in enumerate(r):
                    if c.value is None:
                        tb += "<td></td>"
                    else:
                        tb += f"<td>{c.value}</td>"
                tb += "</tr>"
            tb += "</table>\n"
        return tb

    def __call__(self, fnm):
        if isinstance(fnm, str):
            wb = load_workbook(fnm)
        else:
            wb = load_workbook(BytesIO(fnm))
        res = []
        for sheetname in wb.sheetnames:
            ws = wb[sheetname]
            rows = list(ws.rows)
            if not rows:continue
            ti = list(rows[0])
            for r in list(rows[1:]):
                l = []
                for i, c in enumerate(r):
                    if not c.value:
                        continue
                    t = str(ti[i].value) if i < len(ti) else ""
                    t += ("：" if t else "") + str(c.value)
                    l.append(t)
                l = "; ".join(l)
                if sheetname.lower().find("sheet") < 0:
                    l += " ——" + sheetname
                res.append(l)
        return res

    @staticmethod
    def row_number(fnm, binary):
        if fnm.split(".")[-1].lower().find("xls") >= 0:
            wb = load_workbook(BytesIO(binary))
            total = 0
            for sheetname in wb.sheetnames:
                ws = wb[sheetname]
                total += len(list(ws.rows))
                return total

        if fnm.split(".")[-1].lower() in ["csv", "txt"]:
            txt = binary.decode("utf-8")
            return len(txt.split("\n"))


if __name__ == "__main__":
    psr = HuExcelParser()
    psr(sys.argv[1])
