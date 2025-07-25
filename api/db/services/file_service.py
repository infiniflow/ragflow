#
#  Copyright 2024 The InfiniFlow Authors. All Rights Reserved.
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
import logging
import os
import re
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

from flask_login import current_user
from peewee import fn

from api.constants import FILE_NAME_LEN_LIMIT
from api.db import KNOWLEDGEBASE_FOLDER_NAME, FileSource, FileType, ParserType
from api.db.db_models import DB, Document, File, File2Document, Knowledgebase
from api.db.services import duplicate_name
from api.db.services.common_service import CommonService
from api.db.services.document_service import DocumentService
from api.db.services.file2document_service import File2DocumentService
from api.utils import get_uuid
from api.utils.file_utils import filename_type, read_potential_broken_pdf, thumbnail_img
from rag.utils.storage_factory import STORAGE_IMPL


class FileService(CommonService):
    # Service class for managing file operations and storage
    model = File

    @classmethod
    @DB.connection_context()
    def get_by_pf_id(cls, tenant_id, pf_id, page_number, items_per_page, orderby, desc, keywords):
        # Get files by parent folder ID with pagination and filtering
        # Args:
        #     tenant_id: ID of the tenant
        #     pf_id: Parent folder ID
        #     page_number: Page number for pagination
        #     items_per_page: Number of items per page
        #     orderby: Field to order by
        #     desc: Boolean indicating descending order
        #     keywords: Search keywords
        # Returns:
        #     Tuple of (file_list, total_count)
        if keywords:
            files = cls.model.select().where((cls.model.tenant_id == tenant_id), (cls.model.parent_id == pf_id), (fn.LOWER(cls.model.name).contains(keywords.lower())), ~(cls.model.id == pf_id))
        else:
            files = cls.model.select().where((cls.model.tenant_id == tenant_id), (cls.model.parent_id == pf_id), ~(cls.model.id == pf_id))
        count = files.count()
        if desc:
            files = files.order_by(cls.model.getter_by(orderby).desc())
        else:
            files = files.order_by(cls.model.getter_by(orderby).asc())

        files = files.paginate(page_number, items_per_page)

        res_files = list(files.dicts())
        for file in res_files:
            if file["type"] == FileType.FOLDER.value:
                file["size"] = cls.get_folder_size(file["id"])
                file["kbs_info"] = []
                children = list(
                    cls.model.select()
                    .where(
                        (cls.model.tenant_id == tenant_id),
                        (cls.model.parent_id == file["id"]),
                        ~(cls.model.id == file["id"]),
                    )
                    .dicts()
                )
                file["has_child_folder"] = any(value["type"] == FileType.FOLDER.value for value in children)
                continue
            kbs_info = cls.get_kb_id_by_file_id(file["id"])
            file["kbs_info"] = kbs_info

        return res_files, count

    @classmethod
    @DB.connection_context()
    def get_kb_id_by_file_id(cls, file_id):
        # Get knowledge base IDs associated with a file
        # Args:
        #     file_id: File ID
        # Returns:
        #     List of dictionaries containing knowledge base IDs and names
        kbs = (
            cls.model.select(*[Knowledgebase.id, Knowledgebase.name])
            .join(File2Document, on=(File2Document.file_id == file_id))
            .join(Document, on=(File2Document.document_id == Document.id))
            .join(Knowledgebase, on=(Knowledgebase.id == Document.kb_id))
            .where(cls.model.id == file_id)
        )
        if not kbs:
            return []
        kbs_info_list = []
        for kb in list(kbs.dicts()):
            kbs_info_list.append({"kb_id": kb["id"], "kb_name": kb["name"]})
        return kbs_info_list

    @classmethod
    @DB.connection_context()
    def get_by_pf_id_name(cls, id, name):
        # Get file by parent folder ID and name
        # Args:
        #     id: Parent folder ID
        #     name: File name
        # Returns:
        #     File object or None if not found
        file = cls.model.select().where((cls.model.parent_id == id) & (cls.model.name == name))
        if file.count():
            e, file = cls.get_by_id(file[0].id)
            if not e:
                raise RuntimeError("Database error (File retrieval)!")
            return file
        return None

    @classmethod
    @DB.connection_context()
    def get_id_list_by_id(cls, id, name, count, res):
        # Recursively get list of file IDs by traversing folder structure
        # Args:
        #     id: Starting folder ID
        #     name: List of folder names to traverse
        #     count: Current depth in traversal
        #     res: List to store results
        # Returns:
        #     List of file IDs
        if count < len(name):
            file = cls.get_by_pf_id_name(id, name[count])
            if file:
                res.append(file.id)
                return cls.get_id_list_by_id(file.id, name, count + 1, res)
            else:
                return res
        else:
            return res

    @classmethod
    @DB.connection_context()
    def get_all_innermost_file_ids(cls, folder_id, result_ids):
        # Get IDs of all files in the deepest level of folders
        # Args:
        #     folder_id: Starting folder ID
        #     result_ids: List to store results
        # Returns:
        #     List of file IDs
        subfolders = cls.model.select().where(cls.model.parent_id == folder_id)
        if subfolders.exists():
            for subfolder in subfolders:
                cls.get_all_innermost_file_ids(subfolder.id, result_ids)
        else:
            result_ids.append(folder_id)
        return result_ids

    @classmethod
    @DB.connection_context()
    def create_folder(cls, file, parent_id, name, count):
        # Recursively create folder structure
        # Args:
        #     file: Current file object
        #     parent_id: Parent folder ID
        #     name: List of folder names to create
        #     count: Current depth in creation
        # Returns:
        #     Created file object
        if count > len(name) - 2:
            return file
        else:
            file = cls.insert(
                {"id": get_uuid(), "parent_id": parent_id, "tenant_id": current_user.id, "created_by": current_user.id, "name": name[count], "location": "", "size": 0, "type": FileType.FOLDER.value}
            )
            return cls.create_folder(file, file.id, name, count + 1)

    @classmethod
    @DB.connection_context()
    def is_parent_folder_exist(cls, parent_id):
        # Check if parent folder exists
        # Args:
        #     parent_id: Parent folder ID
        # Returns:
        #     Boolean indicating if folder exists
        parent_files = cls.model.select().where(cls.model.id == parent_id)
        if parent_files.count():
            return True
        cls.delete_folder_by_pf_id(parent_id)
        return False

    @classmethod
    @DB.connection_context()
    def get_root_folder(cls, tenant_id):
        # Get or create root folder for tenant
        # Args:
        #     tenant_id: Tenant ID
        # Returns:
        #     Root folder dictionary
        for file in cls.model.select().where((cls.model.tenant_id == tenant_id), (cls.model.parent_id == cls.model.id)):
            return file.to_dict()

        file_id = get_uuid()
        file = {
            "id": file_id,
            "parent_id": file_id,
            "tenant_id": tenant_id,
            "created_by": tenant_id,
            "name": "/",
            "type": FileType.FOLDER.value,
            "size": 0,
            "location": "",
        }
        cls.save(**file)
        return file

    @classmethod
    @DB.connection_context()
    def get_kb_folder(cls, tenant_id):
        # Get knowledge base folder for tenant
        # Args:
        #     tenant_id: Tenant ID
        # Returns:
        #     Knowledge base folder dictionary
        for root in cls.model.select().where((cls.model.tenant_id == tenant_id), (cls.model.parent_id == cls.model.id)):
            for folder in cls.model.select().where((cls.model.tenant_id == tenant_id), (cls.model.parent_id == root.id), (cls.model.name == KNOWLEDGEBASE_FOLDER_NAME)):
                return folder.to_dict()
        assert False, "Can't find the KB folder. Database init error."

    @classmethod
    @DB.connection_context()
    def new_a_file_from_kb(cls, tenant_id, name, parent_id, ty=FileType.FOLDER.value, size=0, location=""):
        # Create a new file from knowledge base
        # Args:
        #     tenant_id: Tenant ID
        #     name: File name
        #     parent_id: Parent folder ID
        #     ty: File type
        #     size: File size
        #     location: File location
        # Returns:
        #     Created file dictionary
        for file in cls.query(tenant_id=tenant_id, parent_id=parent_id, name=name):
            return file.to_dict()
        file = {
            "id": get_uuid(),
            "parent_id": parent_id,
            "tenant_id": tenant_id,
            "created_by": tenant_id,
            "name": name,
            "type": ty,
            "size": size,
            "location": location,
            "source_type": FileSource.KNOWLEDGEBASE,
        }
        cls.save(**file)
        return file

    @classmethod
    @DB.connection_context()
    def init_knowledgebase_docs(cls, root_id, tenant_id):
        # Initialize knowledge base documents
        # Args:
        #     root_id: Root folder ID
        #     tenant_id: Tenant ID
        for _ in cls.model.select().where((cls.model.name == KNOWLEDGEBASE_FOLDER_NAME) & (cls.model.parent_id == root_id)):
            return
        folder = cls.new_a_file_from_kb(tenant_id, KNOWLEDGEBASE_FOLDER_NAME, root_id)

        for kb in Knowledgebase.select(*[Knowledgebase.id, Knowledgebase.name]).where(Knowledgebase.tenant_id == tenant_id):
            kb_folder = cls.new_a_file_from_kb(tenant_id, kb.name, folder["id"])
            for doc in DocumentService.query(kb_id=kb.id):
                FileService.add_file_from_kb(doc.to_dict(), kb_folder["id"], tenant_id)

    @classmethod
    @DB.connection_context()
    def get_parent_folder(cls, file_id):
        # Get parent folder of a file
        # Args:
        #     file_id: File ID
        # Returns:
        #     Parent folder object
        file = cls.model.select().where(cls.model.id == file_id)
        if file.count():
            e, file = cls.get_by_id(file[0].parent_id)
            if not e:
                raise RuntimeError("Database error (File retrieval)!")
        else:
            raise RuntimeError("Database error (File doesn't exist)!")
        return file

    @classmethod
    @DB.connection_context()
    def get_all_parent_folders(cls, start_id):
        # Get all parent folders in path
        # Args:
        #     start_id: Starting file ID
        # Returns:
        #     List of parent folder objects
        parent_folders = []
        current_id = start_id
        while current_id:
            e, file = cls.get_by_id(current_id)
            if file.parent_id != file.id and e:
                parent_folders.append(file)
                current_id = file.parent_id
            else:
                parent_folders.append(file)
                break
        return parent_folders

    @classmethod
    @DB.connection_context()
    def insert(cls, file):
        # Insert a new file record
        # Args:
        #     file: File data dictionary
        # Returns:
        #     Created file object
        if not cls.save(**file):
            raise RuntimeError("Database error (File)!")
        return File(**file)

    @classmethod
    @DB.connection_context()
    def delete(cls, file):
        #
        return cls.delete_by_id(file.id)

    @classmethod
    @DB.connection_context()
    def delete_by_pf_id(cls, folder_id):
        return cls.model.delete().where(cls.model.parent_id == folder_id).execute()

    @classmethod
    @DB.connection_context()
    def delete_folder_by_pf_id(cls, user_id, folder_id):
        try:
            files = cls.model.select().where((cls.model.tenant_id == user_id) & (cls.model.parent_id == folder_id))
            for file in files:
                cls.delete_folder_by_pf_id(user_id, file.id)
            return (cls.model.delete().where((cls.model.tenant_id == user_id) & (cls.model.id == folder_id)).execute(),)
        except Exception:
            logging.exception("delete_folder_by_pf_id")
            raise RuntimeError("Database error (File retrieval)!")

    @classmethod
    @DB.connection_context()
    def get_file_count(cls, tenant_id):
        files = cls.model.select(cls.model.id).where(cls.model.tenant_id == tenant_id)
        return len(files)

    @classmethod
    @DB.connection_context()
    def get_folder_size(cls, folder_id):
        size = 0

        def dfs(parent_id):
            nonlocal size
            for f in cls.model.select(*[cls.model.id, cls.model.size, cls.model.type]).where(cls.model.parent_id == parent_id, cls.model.id != parent_id):
                size += f.size
                if f.type == FileType.FOLDER.value:
                    dfs(f.id)

        dfs(folder_id)
        return size

    @classmethod
    @DB.connection_context()
    def add_file_from_kb(cls, doc, kb_folder_id, tenant_id):
        for _ in File2DocumentService.get_by_document_id(doc["id"]):
            return
        file = {
            "id": get_uuid(),
            "parent_id": kb_folder_id,
            "tenant_id": tenant_id,
            "created_by": tenant_id,
            "name": doc["name"],
            "type": doc["type"],
            "size": doc["size"],
            "location": doc["location"],
            "source_type": FileSource.KNOWLEDGEBASE,
        }
        cls.save(**file)
        File2DocumentService.save(**{"id": get_uuid(), "file_id": file["id"], "document_id": doc["id"]})

    @classmethod
    @DB.connection_context()
    def move_file(cls, file_ids, folder_id):
        try:
            cls.filter_update((cls.model.id << file_ids,), {"parent_id": folder_id})
        except Exception:
            logging.exception("move_file")
            raise RuntimeError("Database error (File move)!")

    @classmethod
    @DB.connection_context()
    def upload_document(self, kb, file_objs, user_id, user_type="alaska"):
        # Upload documents and extract metadata
        # Args:
        #     kb: Knowledge base object
        #     file_objs: List of file objects to upload
        #     user_id: User ID of the uploader
        # Returns:
        #     Tuple of (error_list, uploaded_files)

        # Import LLM services for metadata extraction
        from api.db.services.llm_service import LLMBundle
        from api.db import LLMType

        root_folder = self.get_root_folder(user_id)
        pf_id = root_folder["id"]
        self.init_knowledgebase_docs(pf_id, user_id)
        kb_root_folder = self.get_kb_folder(user_id)
        kb_folder = self.new_a_file_from_kb(kb.tenant_id, kb.name, kb_root_folder["id"])

        err, files = [], []
        for file in file_objs:
            print(f"Processing file: {file}")  # Debug log
            try:
                MAX_FILE_NUM_PER_USER = int(os.environ.get("MAX_FILE_NUM_PER_USER", 0))
                if MAX_FILE_NUM_PER_USER > 0 and DocumentService.get_doc_count(kb.tenant_id) >= MAX_FILE_NUM_PER_USER:
                    raise RuntimeError("Exceed the maximum file number of a free user!")
                if len(file.filename.encode("utf-8")) > FILE_NAME_LEN_LIMIT:
                    raise RuntimeError(f"File name must be {FILE_NAME_LEN_LIMIT} bytes or less.")

                filename = duplicate_name(DocumentService.query, name=file.filename, kb_id=kb.id)
                filetype = filename_type(filename)
                if filetype == FileType.OTHER.value:
                    raise RuntimeError("This type of file has not been supported yet!")

                location = filename
                while STORAGE_IMPL.obj_exist(kb.id, location):
                    location += "_"

                blob = file.read()
                if filetype == FileType.PDF.value:
                    blob = read_potential_broken_pdf(blob)
                STORAGE_IMPL.put(kb.id, location, blob)

                doc_id = get_uuid()

                img = thumbnail_img(filename, blob)
                thumbnail_location = ""
                if img is not None:
                    thumbnail_location = f"thumbnail_{doc_id}.png"
                    STORAGE_IMPL.put(kb.id, thumbnail_location, img)

                # Extract document content for metadata analysis
                document_text = ""
                try:
                    # Parse document content for metadata extraction
                    parsed_content = self.parse_docs([file], user_id)
                    document_text = parsed_content[:4000]  # Limit text for LLM processing
                except Exception as e:
                    print(f"Failed to parse document content for metadata: {str(e)}")

                # Extract user_type from file representation or use default
                user_type = "alaska"  # default
                try:
                    # Try to extract user_type from file string representation
                    file_str = str(file)
                    if "('homefarm')" in file_str:
                        user_type = "homefarm"
                    elif "('alaska')" in file_str:
                        user_type = "alaska"
                    elif "('kafi')" in file_str:
                        user_type = "kafi"
                    # You can add more user types as needed
                except Exception as e:
                    print(f"Failed to extract user_type from file: {str(e)}")

                # Extract metadata using LLM
                metadata = {}
                # Only extract metadata if document text is not empty and user_type is not "random"
                if document_text.strip() and user_type != "random":
                    logging.info(f"Extracting metadata with user_type: {user_type}")
                    llm_bundle = LLMBundle(kb.tenant_id, LLMType.CHAT, kb.llm_id if hasattr(kb, 'llm_id') else None)
                    metadata = self.extract_metadata_with_llm(document_text, llm_bundle, user_type=user_type)

                doc = {
                    "id": doc_id,
                    "kb_id": kb.id,
                    "parser_id": self.get_parser(filetype, filename, kb.parser_id),
                    "parser_config": kb.parser_config,
                    "created_by": user_id,
                    "type": filetype,
                    "name": filename,
                    "suffix": Path(filename).suffix.lstrip("."),
                    "location": location,
                    "size": len(blob),
                    "thumbnail": thumbnail_location,
                    "meta_fields": metadata  # Store extracted metadata
                }
                DocumentService.insert(doc)

                FileService.add_file_from_kb(doc, kb_folder["id"], kb.tenant_id)
                files.append((doc, blob))
            except Exception as e:
                err.append(file.filename + ": " + str(e))

        return err, files

    @staticmethod
    def parse_docs(file_objs, user_id):
        from rag.app import audio, email, naive, picture, presentation
        import tempfile
        import os

        def dummy(prog=None, msg=""):
            pass

        FACTORY = {ParserType.PRESENTATION.value: presentation, ParserType.PICTURE.value: picture, ParserType.AUDIO.value: audio, ParserType.EMAIL.value: email}
        parser_config = {"chunk_token_num": 16096, "delimiter": "\n!?;。；！？", "layout_recognize": "Plain Text"}
        exe = ThreadPoolExecutor(max_workers=12)
        threads = []
        
        for file in file_objs:
            kwargs = {"lang": "English", "callback": dummy, "parser_config": parser_config, "from_page": 0, "to_page": 100000, "tenant_id": user_id}
            filetype = filename_type(file.filename)
            
            # Reset file pointer to beginning
            file.seek(0)
            blob = file.read()
            
            # Create temporary file for parsers that need file path
            with tempfile.NamedTemporaryFile(delete=False, suffix=os.path.splitext(file.filename)[1]) as tmp_file:
                tmp_file.write(blob)
                tmp_file_path = tmp_file.name
            
            try:
                threads.append(exe.submit(FACTORY.get(FileService.get_parser(filetype, file.filename, ""), naive).chunk, tmp_file_path, blob, **kwargs))
            except Exception as e:
                # Clean up temp file if thread creation fails
                if os.path.exists(tmp_file_path):
                    os.unlink(tmp_file_path)
                raise e

        res = []
        temp_files = []
        for i, th in enumerate(threads):
            try:
                result = th.result()
                res.append("\n".join([ck["content_with_weight"] for ck in result]))
            except Exception as e:
                logging.warning(f"Failed to parse file: {str(e)}")
                res.append("")  # Add empty string for failed parsing

        return "\n\n".join(res)

    @staticmethod
    def get_parser(doc_type, filename, default):
        if doc_type == FileType.VISUAL:
            return ParserType.PICTURE.value
        if doc_type == FileType.AURAL:
            return ParserType.AUDIO.value
        if re.search(r"\.(ppt|pptx|pages)$", filename):
            return ParserType.PRESENTATION.value
        if re.search(r"\.(eml)$", filename):
            return ParserType.EMAIL.value
        return default

    @classmethod
    def extract_metadata_with_llm(cls, document_text, llm_bundle, user_type="homefarm"):
        """Extract metadata from document content using LLM"""
        
        # Escape the document text to prevent formatting issues
        import json
        escaped_document_text = json.dumps(document_text[:4000])[1:-1]  # Remove outer quotes from json.dumps result
        
        if user_type == "alaska":
            metadata_prompt = f"""You are an intelligent assistant responsible for analyzing and standardizing internal knowledge documents for a refrigeration and household appliances enterprise RAG (Retrieval-Augmented Generation) system.

Your task is to extract structured metadata from the following document content related to refrigeration and household appliances business.

[DOCUMENT_START]  
{escaped_document_text}  
[DOCUMENT_END]

Return a valid JSON object with the exact structure below. All values must be written in **English**, except where otherwise specified.

Expected JSON format:

{{
  "id": "auto_generated_or_unique_identifier",
  "domain": "Refrigeration & Household Appliances",
  "industry": "Specific industry, e.g., Air conditioning systems, Refrigeration equipment, Home appliances retail",
  "knowledge_type": "Type of knowledge, e.g., Product Specifications / Installation Guide / Maintenance Manual / Customer Service Script / Technical FAQ / Sales Training",
  "role_target": ["Sales Representative", "Technical Support", "Installation Technician", "Customer Service", "Maintenance Staff"],
  "tone": "Writing tone, e.g., Technical and professional, Customer-friendly, Clear and instructional",
  "context_user": {{
    "intent": "What the user is trying to achieve, e.g., Product consultation, Technical troubleshooting, Installation guidance, Maintenance support",
    "behavior": "Typical behavior, e.g., Seeking product information, Requesting technical support, Learning installation procedures",
    "channel": "Where this knowledge is used, e.g., In-store consultation, Phone support, Field service, Online chat"
  }},
  "keywords": ["keyword_1", "keyword_2", "..."],
  "version": "e.g., v1.0, v2.1",
  "suggested_tags": ["Air Conditioning", "Refrigeration", "Home Appliances", "Installation", "Maintenance", "Technical Support", "Sales"],
  "summary": "Detailed summary including: (1) Purpose and target users in refrigeration/appliances context, (2) Main technical topics and product knowledge, (3) Document structure and practical applications, (4) Value for sales/service staff, (5) Specific usage in appliance business operations",
  "language": "en"
}}

Rules:
1. Do not skip any field — if unsure, use "" or [].
2. Do not invent new keys or values.
3. Keep proper JSON syntax, including commas and brackets.
4. Output must be a single JSON block, without explanation or extra notes.
5. All values must be written in English except the `"language"` field which must be `"en"`.
6. Focus on refrigeration and household appliances business context.
"""
        elif user_type == "kafi":
            metadata_prompt = f"""You are an intelligent assistant responsible for analyzing and standardizing internal knowledge documents for a Vietnamese securities and financial services company RAG (Retrieval-Augmented Generation) system.

Your task is to extract structured metadata from the following document content related to securities trading, investment services, and financial consulting business.

[DOCUMENT_START]  
{escaped_document_text}  
[DOCUMENT_END]

Return a valid JSON object with the exact structure below. All values must be written in **English**, except where otherwise specified.

Expected JSON format:

{{
  "id": "auto_generated_or_unique_identifier",
  "domain": "Securities & Financial Services",
  "industry": "Specific industry, e.g., Stock trading platform, Investment advisory, Margin lending, Financial consulting",
  "knowledge_type": "Type of knowledge, e.g., Trading Guide / Market Analysis / Customer Service Script / Investment Strategy / Margin Loan Documentation / Compliance Manual / Product Training",
  "role_target": ["Financial Advisor", "Trading Support", "Customer Service", "Investment Analyst", "Compliance Officer", "Sales Representative"],
  "tone": "Writing tone, e.g., Professional financial communication, Client-focused, Analytical and data-driven, Regulatory compliant",
  "context_user": {{
    "intent": "What the user is trying to achieve, e.g., Investment guidance, Trading support, Market analysis, Loan consultation, Compliance verification",
    "behavior": "Typical behavior, e.g., Seeking market insights, Requesting trading assistance, Learning investment strategies, Applying for margin loans",
    "channel": "Where this knowledge is used, e.g., Trading platform, Phone consultation, Investment advisory meeting, Online support chat"
  }},
  "keywords": ["keyword_1", "keyword_2", "..."],
  "version": "e.g., v1.0, v2.1",
  "suggested_tags": ["Securities Trading", "Investment", "Margin Loans", "Market Analysis", "Financial Advisory", "Vietnamese Market", "Kafi Trade"],
  "summary": "Detailed summary including: (1) Purpose and target users in securities/financial context, (2) Main financial topics and investment knowledge, (3) Document structure and trading applications, (4) Value for financial advisors/traders, (5) Specific usage in securities business operations",
  "language": "en"
}}

Rules:
1. Do not skip any field — if unsure, use "" or [].
2. Do not invent new keys or values.
3. Keep proper JSON syntax, including commas and brackets.
4. Output must be a single JSON block, without explanation or extra notes.
5. All values must be written in English except the `"language"` field which must be `"en"`.
6. Focus on Vietnamese securities and financial services business context.
"""
        else:
            # Default prompt for homefarm and other user types
            metadata_prompt = f"""You are an intelligent assistant responsible for analyzing and standardizing internal knowledge documents for an enterprise RAG (Retrieval-Augmented Generation) system.

Your task is to extract structured metadata from the following document content.

[DOCUMENT_START]  
{escaped_document_text}  
[DOCUMENT_END]

Return a valid JSON object with the exact structure below. All values must be written in **English**, except where otherwise specified.

Expected JSON format:

{{
  "id": "auto_generated_or_unique_identifier",
  "domain": "General field, e.g., E-commerce, Healthcare, Education, Finance",
  "industry": "Specific industry, e.g., Imported food & retail distribution",
  "knowledge_type": "Type of knowledge, e.g., Product Knowledge / Customer Handling Script / FAQ / Training Material",
  "role_target": ["Role 1", "Role 2", "..."],
  "tone": "Writing tone, e.g., Natural, inspiring, professional, avoid buzzwords",
  "context_user": {{
    "intent": "What the user is trying to achieve when using this document",
    "behavior": "Typical behavior of the user group",
    "channel": "Where this knowledge is used, e.g., Email, Zalo chat, Phone"
  }},
  "keywords": ["keyword_1", "keyword_2", "..."],
  "version": "e.g., v1.0, v2.1",
  "suggested_tags": ["tag_1", "tag_2", "..."],
  "summary": "Detailed summary including: (1) Purpose and user group, (2) Main topics and core knowledge, (3) Document structure and format, (4) Value and benefits for users, (5) Specific usage context",
  "language": "en"
}}

Rules:
1. Do not skip any field — if unsure, use "" or [].
2. Do not invent new keys or values.
3. Keep proper JSON syntax, including commas and brackets.
4. Output must be a single JSON block, without explanation or extra notes.
5. All values must be written in English except the `"language"` field which must be `"en"`.
"""

        try:
            response_text = llm_bundle.chat("", [{"role": "user", "content": metadata_prompt}], {})
            import json
            import re
            
            # Try to extract JSON from markdown code blocks first
            json_match = re.search(r'```(?:json)?\s*(\{.*?\})\s*```', response_text, re.DOTALL)
            if json_match:
                json_str = json_match.group(1)
            else:
                # Fallback: look for JSON without markdown formatting
                json_start = response_text.find('{')
                json_end = response_text.rfind('}') + 1
                if json_start >= 0 and json_end > json_start:
                    json_str = response_text[json_start:json_end]
                else:
                    raise ValueError("No JSON found in response")
            
            # Clean up the JSON string
            json_str = json_str.strip()
            metadata = json.loads(json_str)
            print(f"Successfully parsed metadata: {metadata}")
            return metadata
                
        except Exception as e:
            logging.warning(f"Failed to extract metadata with LLM: {str(e)}")
        
        # Return default metadata based on user type if extraction fails
        if user_type == "alaska":
            return {
                "domain": "Refrigeration & Household Appliances",
                "industry": "Air conditioning & home appliances",
                "knowledge_type": "Technical documentation",
                "role_target": ["Technical Support", "Sales Representative"],
                "tone": "Technical and professional",
                "context_user": {
                    "intent": "Technical reference and customer support",
                    "behavior": "Seeking product or technical information",
                    "channel": "In-store consultation, Phone support"
                },
                "keywords": [],
                "version": "v1.0",
                "suggested_tags": ["Air Conditioning", "Home Appliances", "Technical"],
                "summary": "Technical reference document for refrigeration and household appliances business",
                "language": "en"
            }
        elif user_type == "kafi":
            return {
                "domain": "Securities & Financial Services",
                "industry": "Stock trading & investment advisory",
                "knowledge_type": "Financial documentation",
                "role_target": ["Financial Advisor", "Trading Support", "Customer Service"],
                "tone": "Professional financial communication",
                "context_user": {
                    "intent": "Investment guidance and trading support",
                    "behavior": "Seeking financial advice or trading assistance",
                    "channel": "Trading platform, Phone consultation"
                },
                "keywords": [],
                "version": "v1.0",
                "suggested_tags": ["Securities Trading", "Investment", "Financial Advisory"],
                "summary": "Financial reference document for Vietnamese securities and investment services",
                "language": "en"
            }
        else:
            return {
                "domain": "General",
                "industry": "Unspecified",
                "knowledge_type": "Internal document",
                "role_target": ["General user"],
                "tone": "Professional, formal",
                "context_user": {
                    "intent": "Information reference",
                    "behavior": "Seeking necessary information",
                    "channel": "Internal system"
                },
                "keywords": [],
                "version": "v1.0",
                "suggested_tags": ["Document", "Reference"],
                "summary": "General reference document for internal system",
                "language": "en"
            }
