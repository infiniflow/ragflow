"""Box connector"""
import logging
from datetime import datetime, timezone
from typing import Any

from box_sdk_gen import BoxClient
from common.data_source.config import DocumentSource, INDEX_BATCH_SIZE
from common.data_source.exceptions import (
    ConnectorMissingCredentialError,
    ConnectorValidationError,
)
from common.data_source.interfaces import LoadConnector, PollConnector, SecondsSinceUnixEpoch
from common.data_source.models import Document, GenerateDocumentsOutput
from common.data_source.utils import get_file_ext

class BoxConnector(LoadConnector, PollConnector):
    def __init__(self, folder_id: str, batch_size: int = INDEX_BATCH_SIZE, use_marker: bool = True) -> None:
        self.batch_size = batch_size
        self.folder_id = "0" if not folder_id else folder_id
        self.use_marker = use_marker
        

    def load_credentials(self, auth: Any):
        self.box_client = BoxClient(auth=auth)
        return None


    def validate_connector_settings(self):
        if self.box_client is None:
            raise ConnectorMissingCredentialError("Box")

        try:
            self.box_client.users.get_user_me()
        except Exception as e:
            logging.exception("[Box]: Failed to validate Box credentials")
            raise ConnectorValidationError(f"Unexpected error during Box settings validation: {e}")


    def _yield_files_recursive(
            self,
            folder_id,
            start: SecondsSinceUnixEpoch | None,
            end: SecondsSinceUnixEpoch | None
        ) -> GenerateDocumentsOutput:

        if self.box_client is None:
            raise ConnectorMissingCredentialError("Box")

        result = self.box_client.folders.get_folder_items(
            folder_id=folder_id,
            limit=self.batch_size,
            usemarker=self.use_marker
        )

        while True:
            batch: list[Document] = []
            for entry in result.entries:
                if entry.type == 'file' :
                    file = self.box_client.files.get_file_by_id(
                        entry.id
                    )
                    raw_time = (
                        getattr(file, "created_at", None)
                        or getattr(file, "content_created_at", None)
                    )

                    if raw_time:
                        modified_time = self._box_datetime_to_epoch_seconds(raw_time)
                        if start is not None and modified_time <= start:
                            continue
                        if end is not None and modified_time > end:
                            continue

                    content_bytes = self.box_client.downloads.download_file(file.id)

                    batch.append(
                        Document(
                            id=f"box:{file.id}",
                            blob=content_bytes.read(),
                            source=DocumentSource.BOX,
                            semantic_identifier=file.name,
                            extension=get_file_ext(file.name),
                            doc_updated_at=modified_time,
                            size_bytes=file.size,
                            metadata=file.metadata
                        )
                    )
                elif entry.type == 'folder':
                    yield from self._yield_files_recursive(folder_id=entry.id, start=start, end=end)

            if batch:
                yield batch

            if not result.next_marker:
                break

            result = self.box_client.folders.get_folder_items(
                folder_id=folder_id,
                limit=self.batch_size,
                marker=result.next_marker,
                usemarker=True
            )


    def _box_datetime_to_epoch_seconds(self, dt: datetime) -> SecondsSinceUnixEpoch:
        """Convert a Box SDK datetime to Unix epoch seconds (UTC).
        Only supports datetime; any non-datetime should be filtered out by caller.
        """
        if not isinstance(dt, datetime):
            raise TypeError(f"box_datetime_to_epoch_seconds expects datetime, got {type(dt)}")

        if dt.tzinfo is None:
            dt = dt.replace(tzinfo=timezone.utc)
        else:
            dt = dt.astimezone(timezone.utc)

        return SecondsSinceUnixEpoch(int(dt.timestamp()))


    def poll_source(self, start, end):
        return self._yield_files_recursive(folder_id=self.folder_id, start=start, end=end)


    def load_from_state(self):
        return self._yield_files_recursive(folder_id=self.folder_id, start=None, end=None)


# from flask import Flask, request, redirect

# from box_sdk_gen import BoxClient, BoxOAuth, OAuthConfig, GetAuthorizeUrlOptions

# app = Flask(__name__)

# AUTH = BoxOAuth(
#     OAuthConfig(client_id="8suvn9ik7qezsq2dub0ye6ubox61081z", client_secret="QScvhLgBcZrb2ck1QP1ovkutpRhI2QcN")
# )


# @app.route("/")
# def get_auth():
#     auth_url = AUTH.get_authorize_url(
#         options=GetAuthorizeUrlOptions(redirect_uri="http://localhost:4999/oauth2callback")
#     )
#     return redirect(auth_url, code=302)


# @app.route("/oauth2callback")
# def callback():
#     AUTH.get_tokens_authorization_code_grant(request.args.get("code"))
#     box = BoxConnector()
#     box.load_credentials({"auth": AUTH})
    
#     lst = []
#     for file in box.load_from_state():
#        for f in file:
#            lst.append(f.semantic_identifier)

#     return lst

if __name__ == "__main__":
    pass
    # app.run(port=4999)