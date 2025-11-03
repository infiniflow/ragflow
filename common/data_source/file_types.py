PRESENTATION_MIME_TYPE = (
    "application/vnd.openxmlformats-officedocument.presentationml.presentation"
)

SPREADSHEET_MIME_TYPE = (
    "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
)
WORD_PROCESSING_MIME_TYPE = (
    "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
)
PDF_MIME_TYPE = "application/pdf"


class UploadMimeTypes:
    IMAGE_MIME_TYPES = {"image/jpeg", "image/png", "image/webp"}
    CSV_MIME_TYPES = {"text/csv"}
    TEXT_MIME_TYPES = {
        "text/plain",
        "text/markdown",
        "text/x-markdown",
        "text/x-config",
        "text/tab-separated-values",
        "application/json",
        "application/xml",
        "text/xml",
        "application/x-yaml",
    }
    DOCUMENT_MIME_TYPES = {
        PDF_MIME_TYPE,
        WORD_PROCESSING_MIME_TYPE,
        PRESENTATION_MIME_TYPE,
        SPREADSHEET_MIME_TYPE,
        "message/rfc822",
        "application/epub+zip",
    }

    ALLOWED_MIME_TYPES = IMAGE_MIME_TYPES.union(
        TEXT_MIME_TYPES, DOCUMENT_MIME_TYPES, CSV_MIME_TYPES
    )
