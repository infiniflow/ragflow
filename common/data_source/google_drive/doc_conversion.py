import io
import logging
import mimetypes
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, cast
from urllib.parse import urlparse, urlunparse

from googleapiclient.errors import HttpError  # type: ignore  # type: ignore
from googleapiclient.http import MediaIoBaseDownload  # type: ignore
from pydantic import BaseModel

from common.data_source.config import DocumentSource, FileOrigin
from common.data_source.google_drive.constant import DRIVE_FOLDER_TYPE, DRIVE_SHORTCUT_TYPE
from common.data_source.google_drive.model import GDriveMimeType, GoogleDriveFileType
from common.data_source.google_drive.section_extraction import HEADING_DELIMITER
from common.data_source.google_util.resource import GoogleDriveService, get_drive_service
from common.data_source.models import ConnectorFailure, Document, DocumentFailure, ImageSection, SlimDocument, TextSection
from common.data_source.utils import get_file_ext

# Image types that should be excluded from processing
EXCLUDED_IMAGE_TYPES = [
    "image/bmp",
    "image/tiff",
    "image/gif",
    "image/svg+xml",
    "image/avif",
]

GOOGLE_MIME_TYPES_TO_EXPORT = {
    GDriveMimeType.DOC.value: "text/plain",
    GDriveMimeType.SPREADSHEET.value: "text/csv",
    GDriveMimeType.PPT.value: "text/plain",
}

GOOGLE_NATIVE_EXPORT_TARGETS: dict[str, tuple[str, str]] = {
    GDriveMimeType.DOC.value: ("application/vnd.openxmlformats-officedocument.wordprocessingml.document", ".docx"),
    GDriveMimeType.SPREADSHEET.value: ("application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", ".xlsx"),
    GDriveMimeType.PPT.value: ("application/vnd.openxmlformats-officedocument.presentationml.presentation", ".pptx"),
}
GOOGLE_NATIVE_EXPORT_FALLBACK: tuple[str, str] = ("application/pdf", ".pdf")

ACCEPTED_PLAIN_TEXT_FILE_EXTENSIONS = [
    ".txt",
    ".md",
    ".mdx",
    ".conf",
    ".log",
    ".json",
    ".csv",
    ".tsv",
    ".xml",
    ".yml",
    ".yaml",
    ".sql",
]

ACCEPTED_DOCUMENT_FILE_EXTENSIONS = [
    ".pdf",
    ".docx",
    ".pptx",
    ".xlsx",
    ".eml",
    ".epub",
    ".html",
]

ACCEPTED_IMAGE_FILE_EXTENSIONS = [
    ".png",
    ".jpg",
    ".jpeg",
    ".webp",
]

ALL_ACCEPTED_FILE_EXTENSIONS = ACCEPTED_PLAIN_TEXT_FILE_EXTENSIONS + ACCEPTED_DOCUMENT_FILE_EXTENSIONS + ACCEPTED_IMAGE_FILE_EXTENSIONS

MAX_RETRIEVER_EMAILS = 20
CHUNK_SIZE_BUFFER = 64  # extra bytes past the limit to read
# This is not a standard valid Unicode char, it is used by the docs advanced API to
# represent smart chips (elements like dates and doc links).
SMART_CHIP_CHAR = "\ue907"
WEB_VIEW_LINK_KEY = "webViewLink"
# Fallback templates for generating web links when Drive omits webViewLink.
_FALLBACK_WEB_VIEW_LINK_TEMPLATES = {
    GDriveMimeType.DOC.value: "https://docs.google.com/document/d/{}/view",
    GDriveMimeType.SPREADSHEET.value: "https://docs.google.com/spreadsheets/d/{}/view",
    GDriveMimeType.PPT.value: "https://docs.google.com/presentation/d/{}/view",
}


class PermissionSyncContext(BaseModel):
    """
    This is the information that is needed to sync permissions for a document.
    """

    primary_admin_email: str
    google_domain: str


def onyx_document_id_from_drive_file(file: GoogleDriveFileType) -> str:
    link = file.get(WEB_VIEW_LINK_KEY)
    if not link:
        file_id = file.get("id")
        if not file_id:
            raise KeyError(f"Google Drive file missing both '{WEB_VIEW_LINK_KEY}' and 'id' fields.")
        mime_type = file.get("mimeType", "")
        template = _FALLBACK_WEB_VIEW_LINK_TEMPLATES.get(mime_type)
        if template is None:
            link = f"https://drive.google.com/file/d/{file_id}/view"
        else:
            link = template.format(file_id)
        logging.debug(
            "Missing webViewLink for Google Drive file with id %s. Falling back to constructed link %s",
            file_id,
            link,
        )
    parsed_url = urlparse(link)
    parsed_url = parsed_url._replace(query="")  # remove query parameters
    spl_path = parsed_url.path.split("/")
    if spl_path and (spl_path[-1] in ["edit", "view", "preview"]):
        spl_path.pop()
        parsed_url = parsed_url._replace(path="/".join(spl_path))
    # Remove query parameters and reconstruct URL
    return urlunparse(parsed_url)


def _find_nth(haystack: str, needle: str, n: int, start: int = 0) -> int:
    start = haystack.find(needle, start)
    while start >= 0 and n > 1:
        start = haystack.find(needle, start + len(needle))
        n -= 1
    return start


def align_basic_advanced(basic_sections: list[TextSection | ImageSection], adv_sections: list[TextSection]) -> list[TextSection | ImageSection]:
    """Align the basic sections with the advanced sections.
    In particular, the basic sections contain all content of the file,
    including smart chips like dates and doc links. The advanced sections
    are separated by section headers and contain header-based links that
    improve user experience when they click on the source in the UI.

    There are edge cases in text matching (i.e. the heading is a smart chip or
    there is a smart chip in the doc with text containing the actual heading text)
    that make the matching imperfect; this is hence done on a best-effort basis.
    """
    if len(adv_sections) <= 1:
        return basic_sections  # no benefit from aligning

    basic_full_text = "".join([section.text for section in basic_sections if isinstance(section, TextSection)])
    new_sections: list[TextSection | ImageSection] = []
    heading_start = 0
    for adv_ind in range(1, len(adv_sections)):
        heading = adv_sections[adv_ind].text.split(HEADING_DELIMITER)[0]
        # retrieve the longest part of the heading that is not a smart chip
        heading_key = max(heading.split(SMART_CHIP_CHAR), key=len).strip()
        if heading_key == "":
            logging.warning(f"Cannot match heading: {heading}, its link will come from the following section")
            continue
        heading_offset = heading.find(heading_key)

        # count occurrences of heading str in previous section
        heading_count = adv_sections[adv_ind - 1].text.count(heading_key)

        prev_start = heading_start
        heading_start = _find_nth(basic_full_text, heading_key, heading_count, start=prev_start) - heading_offset
        if heading_start < 0:
            logging.warning(f"Heading key {heading_key} from heading {heading} not found in basic text")
            heading_start = prev_start
            continue

        new_sections.append(
            TextSection(
                link=adv_sections[adv_ind - 1].link,
                text=basic_full_text[prev_start:heading_start],
            )
        )

    # handle last section
    new_sections.append(TextSection(link=adv_sections[-1].link, text=basic_full_text[heading_start:]))
    return new_sections


def is_valid_image_type(mime_type: str) -> bool:
    """
    Check if mime_type is a valid image type.

    Args:
        mime_type: The MIME type to check

    Returns:
        True if the MIME type is a valid image type, False otherwise
    """
    return bool(mime_type) and mime_type.startswith("image/") and mime_type not in EXCLUDED_IMAGE_TYPES


def is_gdrive_image_mime_type(mime_type: str) -> bool:
    """
    Return True if the mime_type is a common image type in GDrive.
    (e.g. 'image/png', 'image/jpeg')
    """
    return is_valid_image_type(mime_type)


def _get_extension_from_file(file: GoogleDriveFileType, mime_type: str, fallback: str = ".bin") -> str:
    file_name = file.get("name") or ""
    if file_name:
        suffix = Path(file_name).suffix
        if suffix:
            return suffix

    file_extension = file.get("fileExtension")
    if file_extension:
        return f".{file_extension.lstrip('.')}"

    guessed = mimetypes.guess_extension(mime_type or "")
    if guessed:
        return guessed

    return fallback


def _download_file_blob(
    service: GoogleDriveService,
    file: GoogleDriveFileType,
    size_threshold: int,
    allow_images: bool,
) -> tuple[bytes, str] | None:
    mime_type = file.get("mimeType", "")
    file_id = file.get("id")
    if not file_id:
        logging.warning("Encountered Google Drive file without id.")
        return None

    if is_gdrive_image_mime_type(mime_type) and not allow_images:
        logging.debug(f"Skipping image {file.get('name')} because allow_images is False.")
        return None

    blob: bytes = b""
    extension = ".bin"
    try:
        if mime_type in GOOGLE_NATIVE_EXPORT_TARGETS:
            export_mime, extension = GOOGLE_NATIVE_EXPORT_TARGETS[mime_type]
            request = service.files().export_media(fileId=file_id, mimeType=export_mime)
            blob = _download_request(request, file_id, size_threshold)
        elif mime_type.startswith("application/vnd.google-apps"):
            export_mime, extension = GOOGLE_NATIVE_EXPORT_FALLBACK
            request = service.files().export_media(fileId=file_id, mimeType=export_mime)
            blob = _download_request(request, file_id, size_threshold)
        else:
            extension = _get_extension_from_file(file, mime_type)
            blob = download_request(service, file_id, size_threshold)
    except HttpError:
        raise

    if not blob:
        return None
    if not extension:
        extension = _get_extension_from_file(file, mime_type)
    return blob, extension


def download_request(service: GoogleDriveService, file_id: str, size_threshold: int) -> bytes:
    """
    Download the file from Google Drive.
    """
    # For other file types, download the file
    # Use the correct API call for downloading files
    request = service.files().get_media(fileId=file_id)
    return _download_request(request, file_id, size_threshold)


def _download_request(request: Any, file_id: str, size_threshold: int) -> bytes:
    response_bytes = io.BytesIO()
    downloader = MediaIoBaseDownload(response_bytes, request, chunksize=size_threshold + CHUNK_SIZE_BUFFER)
    done = False
    while not done:
        download_progress, done = downloader.next_chunk()
        if download_progress.resumable_progress > size_threshold:
            logging.warning(f"File {file_id} exceeds size threshold of {size_threshold}. Skipping2.")
            return bytes()

    response = response_bytes.getvalue()
    if not response:
        logging.warning(f"Failed to download {file_id}")
        return bytes()
    return response


def _download_and_extract_sections_basic(
    file: dict[str, str],
    service: GoogleDriveService,
    allow_images: bool,
    size_threshold: int,
) -> list[TextSection | ImageSection]:
    """Extract text and images from a Google Drive file."""
    file_id = file["id"]
    file_name = file["name"]
    mime_type = file["mimeType"]
    link = file.get(WEB_VIEW_LINK_KEY, "")

    # For non-Google files, download the file
    # Use the correct API call for downloading files
    # lazy evaluation to only download the file if necessary
    def response_call() -> bytes:
        return download_request(service, file_id, size_threshold)

    if is_gdrive_image_mime_type(mime_type):
        # Skip images if not explicitly enabled
        if not allow_images:
            return []

        # Store images for later processing
        sections: list[TextSection | ImageSection] = []

        def store_image_and_create_section(**kwargs):
            pass

        try:
            section, embedded_id = store_image_and_create_section(
                image_data=response_call(),
                file_id=file_id,
                display_name=file_name,
                media_type=mime_type,
                file_origin=FileOrigin.CONNECTOR,
                link=link,
            )
            sections.append(section)
        except Exception as e:
            logging.error(f"Failed to process image {file_name}: {e}")
        return sections

    # For Google Docs, Sheets, and Slides, export as plain text
    if mime_type in GOOGLE_MIME_TYPES_TO_EXPORT:
        export_mime_type = GOOGLE_MIME_TYPES_TO_EXPORT[mime_type]
        # Use the correct API call for exporting files
        request = service.files().export_media(fileId=file_id, mimeType=export_mime_type)
        response = _download_request(request, file_id, size_threshold)
        if not response:
            logging.warning(f"Failed to export {file_name} as {export_mime_type}")
            return []

        text = response.decode("utf-8")
        return [TextSection(link=link, text=text)]

    # Process based on mime type
    if mime_type == "text/plain":
        try:
            text = response_call().decode("utf-8")
            return [TextSection(link=link, text=text)]
        except UnicodeDecodeError as e:
            logging.warning(f"Failed to extract text from {file_name}: {e}")
            return []

    elif mime_type == "application/vnd.openxmlformats-officedocument.wordprocessingml.document":

        def docx_to_text_and_images(*args, **kwargs):
            return "docx_to_text_and_images"

        text, _ = docx_to_text_and_images(io.BytesIO(response_call()))
        return [TextSection(link=link, text=text)]

    elif mime_type == "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":

        def xlsx_to_text(*args, **kwargs):
            return "xlsx_to_text"

        text = xlsx_to_text(io.BytesIO(response_call()), file_name=file_name)
        return [TextSection(link=link, text=text)] if text else []

    elif mime_type == "application/vnd.openxmlformats-officedocument.presentationml.presentation":

        def pptx_to_text(*args, **kwargs):
            return "pptx_to_text"

        text = pptx_to_text(io.BytesIO(response_call()), file_name=file_name)
        return [TextSection(link=link, text=text)] if text else []

    elif mime_type == "application/pdf":

        def read_pdf_file(*args, **kwargs):
            return "read_pdf_file"

        text, _pdf_meta, images = read_pdf_file(io.BytesIO(response_call()))
        pdf_sections: list[TextSection | ImageSection] = [TextSection(link=link, text=text)]

        # Process embedded images in the PDF
        try:
            for idx, (img_data, img_name) in enumerate(images):
                section, embedded_id = store_image_and_create_section(
                    image_data=img_data,
                    file_id=f"{file_id}_img_{idx}",
                    display_name=img_name or f"{file_name} - image {idx}",
                    file_origin=FileOrigin.CONNECTOR,
                )
                pdf_sections.append(section)
        except Exception as e:
            logging.error(f"Failed to process PDF images in {file_name}: {e}")
        return pdf_sections

    # Final attempt at extracting text
    file_ext = get_file_ext(file.get("name", ""))
    if file_ext not in ALL_ACCEPTED_FILE_EXTENSIONS:
        logging.warning(f"Skipping file {file.get('name')} due to extension.")
        return []

    try:

        def extract_file_text(*args, **kwargs):
            return "extract_file_text"

        text = extract_file_text(io.BytesIO(response_call()), file_name)
        return [TextSection(link=link, text=text)]
    except Exception as e:
        logging.warning(f"Failed to extract text from {file_name}: {e}")
        return []


def _convert_drive_item_to_document(
    creds: Any,
    allow_images: bool,
    size_threshold: int,
    retriever_email: str,
    file: GoogleDriveFileType,
    # if not specified, we will not sync permissions
    # will also be a no-op if EE is not enabled
    permission_sync_context: PermissionSyncContext | None,
) -> Document | ConnectorFailure | None:
    """
    Main entry point for converting a Google Drive file => Document object.
    """

    def _get_drive_service() -> GoogleDriveService:
        return get_drive_service(creds, user_email=retriever_email)

    doc_id = "unknown"
    link = file.get(WEB_VIEW_LINK_KEY)

    try:
        if file.get("mimeType") in [DRIVE_SHORTCUT_TYPE, DRIVE_FOLDER_TYPE]:
            logging.info("Skipping shortcut/folder.")
            return None

        size_str = file.get("size")
        if size_str:
            try:
                size_int = int(size_str)
            except ValueError:
                logging.warning(f"Parsing string to int failed: size_str={size_str}")
            else:
                if size_int > size_threshold:
                    logging.warning(f"{file.get('name')} exceeds size threshold of {size_threshold}. Skipping.")
                    return None

        blob_and_ext = _download_file_blob(
            service=_get_drive_service(),
            file=file,
            size_threshold=size_threshold,
            allow_images=allow_images,
        )

        if blob_and_ext is None:
            logging.info(f"Skipping file {file.get('name')} due to incompatible type or download failure.")
            return None

        blob, extension = blob_and_ext
        if not blob:
            logging.warning(f"Failed to download {file.get('name')}. Skipping.")
            return None

        doc_id = onyx_document_id_from_drive_file(file)
        modified_time = file.get("modifiedTime")
        try:
            doc_updated_at = datetime.fromisoformat(modified_time.replace("Z", "+00:00")) if modified_time else datetime.now(timezone.utc)
        except ValueError:
            logging.warning(f"Failed to parse modifiedTime for {file.get('name')}, defaulting to current time.")
            doc_updated_at = datetime.now(timezone.utc)

        return Document(
            id=doc_id,
            source=DocumentSource.GOOGLE_DRIVE,
            semantic_identifier=file.get("name", ""),
            blob=blob,
            extension=extension,
            size_bytes=len(blob),
            doc_updated_at=doc_updated_at,
        )
    except Exception as e:
        doc_id = "unknown"
        try:
            doc_id = onyx_document_id_from_drive_file(file)
        except Exception as e2:
            logging.warning(f"Error getting document id from file: {e2}")

        file_name = file.get("name", doc_id)
        error_str = f"Error converting file '{file_name}' to Document as {retriever_email}: {e}"
        if isinstance(e, HttpError) and e.status_code == 403:
            logging.warning(f"Uncommon permissions error while downloading file. User {retriever_email} was able to see file {file_name} but cannot download it.")
            logging.warning(error_str)

        return ConnectorFailure(
            failed_document=DocumentFailure(
                document_id=doc_id,
                document_link=link,
            ),
            failed_entity=None,
            failure_message=error_str,
            exception=e,
        )


def convert_drive_item_to_document(
    creds: Any,
    allow_images: bool,
    size_threshold: int,
    # if not specified, we will not sync permissions
    # will also be a no-op if EE is not enabled
    permission_sync_context: PermissionSyncContext | None,
    retriever_emails: list[str],
    file: GoogleDriveFileType,
) -> Document | ConnectorFailure | None:
    """
    Attempt to convert a drive item to a document with each retriever email
    in order. returns upon a successful retrieval or a non-403 error.

    We used to always get the user email from the file owners when available,
    but this was causing issues with shared folders where the owner was not included in the service account
    now we use the email of the account that successfully listed the file. There are cases where a
    user that can list a file cannot download it, so we retry with file owners and admin email.
    """
    first_error = None
    doc_or_failure = None
    retriever_emails = retriever_emails[:MAX_RETRIEVER_EMAILS]
    # use seen instead of list(set()) to avoid re-ordering the retriever emails
    seen = set()
    for retriever_email in retriever_emails:
        if retriever_email in seen:
            continue
        seen.add(retriever_email)
        doc_or_failure = _convert_drive_item_to_document(
            creds,
            allow_images,
            size_threshold,
            retriever_email,
            file,
            permission_sync_context,
        )

        # There are a variety of permissions-based errors that occasionally occur
        # when retrieving files. Often when these occur, there is another user
        # that can successfully retrieve the file, so we try the next user.
        if doc_or_failure is None or isinstance(doc_or_failure, Document) or not (isinstance(doc_or_failure.exception, HttpError) and doc_or_failure.exception.status_code in [401, 403, 404]):
            return doc_or_failure

        if first_error is None:
            first_error = doc_or_failure
        else:
            first_error.failure_message += f"\n\n{doc_or_failure.failure_message}"

    if first_error and isinstance(first_error.exception, HttpError) and first_error.exception.status_code == 403:
        # This SHOULD happen very rarely, and we don't want to break the indexing process when
        # a high volume of 403s occurs early. We leave a verbose log to help investigate.
        logging.error(
            f"Skipping file id: {file.get('id')} name: {file.get('name')} due to 403 error.Attempted to retrieve with {retriever_emails},got the following errors: {first_error.failure_message}"
        )
        return None
    return first_error


def build_slim_document(
    creds: Any,
    file: GoogleDriveFileType,
    # if not specified, we will not sync permissions
    # will also be a no-op if EE is not enabled
    permission_sync_context: PermissionSyncContext | None,
) -> SlimDocument | None:
    if file.get("mimeType") in [DRIVE_FOLDER_TYPE, DRIVE_SHORTCUT_TYPE]:
        return None

    owner_email = cast(str | None, file.get("owners", [{}])[0].get("emailAddress"))

    def _get_external_access_for_raw_gdrive_file(*args, **kwargs):
        return None

    external_access = (
        _get_external_access_for_raw_gdrive_file(
            file=file,
            company_domain=permission_sync_context.google_domain,
            retriever_drive_service=(
                get_drive_service(
                    creds,
                    user_email=owner_email,
                )
                if owner_email
                else None
            ),
            admin_drive_service=get_drive_service(
                creds,
                user_email=permission_sync_context.primary_admin_email,
            ),
        )
        if permission_sync_context
        else None
    )
    return SlimDocument(
        id=onyx_document_id_from_drive_file(file),
        external_access=external_access,
    )
