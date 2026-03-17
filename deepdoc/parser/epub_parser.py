#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#

import zipfile
from io import BytesIO
from xml.etree import ElementTree

from .html_parser import RAGFlowHtmlParser

# OPF XML namespaces
_OPF_NS = "http://www.idpf.org/2007/opf"
_CONTAINER_NS = "urn:oasis:names:tc:opendocument:xmlns:container"

# Media types that contain readable XHTML content
_XHTML_MEDIA_TYPES = {"application/xhtml+xml", "text/html", "text/xml"}


class RAGFlowEpubParser:
    """Parse EPUB files by extracting XHTML content in spine (reading) order
    and delegating to RAGFlowHtmlParser for chunking."""

    def __call__(self, fnm, binary=None, chunk_token_num=512):
        if binary:
            zf = zipfile.ZipFile(BytesIO(binary))
        else:
            zf = zipfile.ZipFile(fnm)

        try:
            content_items = self._get_spine_items(zf)
            all_sections = []
            html_parser = RAGFlowHtmlParser()

            for item_path in content_items:
                try:
                    html_bytes = zf.read(item_path)
                except KeyError:
                    continue
                sections = html_parser(item_path, binary=html_bytes, chunk_token_num=chunk_token_num)
                all_sections.extend(sections)

            return all_sections
        finally:
            zf.close()

    @staticmethod
    def _get_spine_items(zf):
        """Return content file paths in spine (reading) order."""
        # 1. Find the OPF file path from META-INF/container.xml
        try:
            container_xml = zf.read("META-INF/container.xml")
        except KeyError:
            return RAGFlowEpubParser._fallback_xhtml_order(zf)

        container_root = ElementTree.fromstring(container_xml)
        rootfile_el = container_root.find(f".//{{{_CONTAINER_NS}}}rootfile")
        if rootfile_el is None:
            return RAGFlowEpubParser._fallback_xhtml_order(zf)

        opf_path = rootfile_el.get("full-path", "")
        if not opf_path:
            return RAGFlowEpubParser._fallback_xhtml_order(zf)

        # Base directory of the OPF file (content paths are relative to it)
        opf_dir = opf_path.rsplit("/", 1)[0] + "/" if "/" in opf_path else ""

        # 2. Parse the OPF file
        try:
            opf_xml = zf.read(opf_path)
        except KeyError:
            return RAGFlowEpubParser._fallback_xhtml_order(zf)

        opf_root = ElementTree.fromstring(opf_xml)

        # 3. Build id->href+mediatype map from <manifest>
        manifest = {}
        for item in opf_root.findall(f".//{{{_OPF_NS}}}item"):
            item_id = item.get("id", "")
            href = item.get("href", "")
            media_type = item.get("media-type", "")
            if item_id and href:
                manifest[item_id] = (href, media_type)

        # 4. Walk <spine> to get reading order
        spine_items = []
        for itemref in opf_root.findall(f".//{{{_OPF_NS}}}itemref"):
            idref = itemref.get("idref", "")
            if idref not in manifest:
                continue
            href, media_type = manifest[idref]
            if media_type not in _XHTML_MEDIA_TYPES:
                continue
            spine_items.append(opf_dir + href)

        return spine_items if spine_items else RAGFlowEpubParser._fallback_xhtml_order(zf)

    @staticmethod
    def _fallback_xhtml_order(zf):
        """Fallback: return all .xhtml/.html files sorted alphabetically."""
        return sorted(n for n in zf.namelist() if n.lower().endswith((".xhtml", ".html", ".htm")) and not n.startswith("META-INF/"))
