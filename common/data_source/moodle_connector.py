from __future__ import annotations

import logging
import os
from collections.abc import Generator
from datetime import datetime, timezone
from retry import retry
from typing import Any, Optional

from markdownify import markdownify as md
from moodle import Moodle as MoodleClient, MoodleException

from common.data_source.config import INDEX_BATCH_SIZE
from common.data_source.exceptions import (
    ConnectorMissingCredentialError,
    CredentialExpiredError,
    InsufficientPermissionsError,
    ConnectorValidationError,
)
from common.data_source.interfaces import (
    LoadConnector,
    PollConnector,
    SecondsSinceUnixEpoch,
)
from common.data_source.models import Document
from common.data_source.utils import batch_generator, rl_requests

logger = logging.getLogger(__name__)


class MoodleConnector(LoadConnector, PollConnector):
    """Moodle LMS connector for accessing course content"""

    def __init__(self, moodle_url: str, batch_size: int = INDEX_BATCH_SIZE) -> None:
        self.moodle_url = moodle_url.rstrip("/")
        self.batch_size = batch_size
        self.moodle_client: Optional[MoodleClient] = None

    def _add_token_to_url(self, file_url: str) -> str:
        """Append Moodle token to URL if missing"""
        if not self.moodle_client:
            return file_url
        token = getattr(self.moodle_client, "token", "")
        if "token=" in file_url.lower():
            return file_url
        delimiter = "&" if "?" in file_url else "?"
        return f"{file_url}{delimiter}token={token}"

    def _log_error(
        self, context: str, error: Exception, level: str = "warning"
    ) -> None:
        """Simplified logging wrapper"""
        msg = f"{context}: {error}"
        if level == "error":
            logger.error(msg)
        else:
            logger.warning(msg)

    def _get_latest_timestamp(self, *timestamps: int) -> int:
        """Return latest valid timestamp"""
        return max((t for t in timestamps if t and t > 0), default=0)

    def _yield_in_batches(
        self, generator: Generator[Document, None, None]
    ) -> Generator[list[Document], None, None]:
        for batch in batch_generator(generator, self.batch_size):
            yield batch

    def load_credentials(self, credentials: dict[str, Any]) -> None:
        token = credentials.get("moodle_token")
        if not token:
            raise ConnectorMissingCredentialError("Moodle API token is required")

        try:
            self.moodle_client = MoodleClient(
                self.moodle_url + "/webservice/rest/server.php", token
            )
            self.moodle_client.core.webservice.get_site_info()
        except MoodleException as e:
            if "invalidtoken" in str(e).lower():
                raise CredentialExpiredError("Moodle token is invalid or expired")
            raise ConnectorMissingCredentialError(
                f"Failed to initialize Moodle client: {e}"
            )

    def validate_connector_settings(self) -> None:
        if not self.moodle_client:
            raise ConnectorMissingCredentialError("Moodle client not initialized")

        try:
            site_info = self.moodle_client.core.webservice.get_site_info()
            if not site_info.sitename:
                raise InsufficientPermissionsError("Invalid Moodle API response")
        except MoodleException as e:
            msg = str(e).lower()
            if "invalidtoken" in msg:
                raise CredentialExpiredError("Moodle token is invalid or expired")
            if "accessexception" in msg:
                raise InsufficientPermissionsError(
                    "Insufficient permissions. Ensure web services are enabled and permissions are correct."
                )
            raise ConnectorValidationError(f"Moodle validation error: {e}")
        except Exception as e:
            raise ConnectorValidationError(f"Unexpected validation error: {e}")

    # -------------------------------------------------------------------------
    # Data loading & polling
    # -------------------------------------------------------------------------

    def load_from_state(self) -> Generator[list[Document], None, None]:
        if not self.moodle_client:
            raise ConnectorMissingCredentialError("Moodle client not initialized")

        logger.info("Starting full load from Moodle workspace")
        courses = self._get_enrolled_courses()
        if not courses:
            logger.warning("No courses found to process")
            return

        yield from self._yield_in_batches(self._process_courses(courses))

    def poll_source(
        self, start: SecondsSinceUnixEpoch, end: SecondsSinceUnixEpoch
    ) -> Generator[list[Document], None, None]:
        if not self.moodle_client:
            raise ConnectorMissingCredentialError("Moodle client not initialized")

        logger.info(
            f"Polling Moodle updates between {datetime.fromtimestamp(start)} and {datetime.fromtimestamp(end)}"
        )
        courses = self._get_enrolled_courses()
        if not courses:
            logger.warning("No courses found to poll")
            return

        yield from self._yield_in_batches(
            self._get_updated_content(courses, start, end)
        )

    @retry(tries=3, delay=1, backoff=2)
    def _get_enrolled_courses(self) -> list:
        if not self.moodle_client:
            raise ConnectorMissingCredentialError("Moodle client not initialized")

        try:
            return self.moodle_client.core.course.get_courses()
        except MoodleException as e:
            self._log_error("fetching courses", e, "error")
            raise ConnectorValidationError(f"Failed to fetch courses: {e}")

    @retry(tries=3, delay=1, backoff=2)
    def _get_course_contents(self, course_id: int):
        if not self.moodle_client:
            raise ConnectorMissingCredentialError("Moodle client not initialized")

        try:
            return self.moodle_client.core.course.get_contents(courseid=course_id)
        except MoodleException as e:
            self._log_error(f"fetching course contents for {course_id}", e)
            return []

    def _process_courses(self, courses) -> Generator[Document, None, None]:
        for course in courses:
            try:
                contents = self._get_course_contents(course.id)
                for section in contents:
                    for module in section.modules:
                        doc = self._process_module(course, section, module)
                        if doc:
                            yield doc
            except Exception as e:
                self._log_error(f"processing course {course.fullname}", e)

    def _get_updated_content(
        self, courses, start: float, end: float
    ) -> Generator[Document, None, None]:
        for course in courses:
            try:
                contents = self._get_course_contents(course.id)
                for section in contents:
                    for module in section.modules:
                        times = [
                            getattr(module, "timecreated", 0),
                            getattr(module, "timemodified", 0),
                        ]
                        if hasattr(module, "contents"):
                            times.extend(
                                getattr(c, "timemodified", 0)
                                for c in module.contents
                                if c and getattr(c, "timemodified", 0)
                            )
                        last_mod = self._get_latest_timestamp(*times)
                        if start < last_mod <= end:
                            doc = self._process_module(course, section, module)
                            if doc:
                                yield doc
            except Exception as e:
                self._log_error(f"polling course {course.fullname}", e)

    def _process_module(self, course, section, module) -> Optional[Document]:
        try:
            mtype = module.modname
            if mtype in ["label", "url"]:
                return None
            if mtype == "resource":
                return self._process_resource(course, section, module)
            if mtype == "forum":
                return self._process_forum(course, section, module)
            if mtype == "page":
                return self._process_page(course, section, module)
            if mtype in ["assign", "quiz"]:
                return self._process_activity(course, section, module)
            if mtype == "book":
                return self._process_book(course, section, module)
        except Exception as e:
            self._log_error(f"processing module {getattr(module, 'name', '?')}", e)
        return None

    def _process_resource(self, course, section, module) -> Optional[Document]:
        if not getattr(module, "contents", None):
            return None

        file_info = module.contents[0]
        if not getattr(file_info, "fileurl", None):
            return None

        file_name = os.path.basename(file_info.filename)
        ts = self._get_latest_timestamp(
            getattr(module, "timecreated", 0),
            getattr(module, "timemodified", 0),
            getattr(file_info, "timemodified", 0),
        )

        try:
            resp = rl_requests.get(
                self._add_token_to_url(file_info.fileurl), timeout=60
            )
            resp.raise_for_status()
            blob = resp.content
            ext = os.path.splitext(file_name)[1] or ".bin"
            semantic_id = f"{course.fullname} / {section.name} / {file_name}"

            # Create metadata dictionary with relevant information
            metadata = {
                "moodle_url": self.moodle_url,
                "course_id": getattr(course, "id", None),
                "course_name": getattr(course, "fullname", None),
                "course_shortname": getattr(course, "shortname", None),
                "section_id": getattr(section, "id", None),
                "section_name": getattr(section, "name", None),
                "section_number": getattr(section, "section", None),
                "module_id": getattr(module, "id", None),
                "module_name": getattr(module, "name", None),
                "module_type": getattr(module, "modname", None),
                "module_instance": getattr(module, "instance", None),
                "file_url": getattr(file_info, "fileurl", None),
                "file_name": file_name,
                "file_size": getattr(file_info, "filesize", len(blob)),
                "file_type": getattr(file_info, "mimetype", None),
                "time_created": getattr(module, "timecreated", None),
                "time_modified": getattr(module, "timemodified", None),
                "visible": getattr(module, "visible", None),
                "groupmode": getattr(module, "groupmode", None),
            }

            return Document(
                id=f"moodle_resource_{module.id}",
                source="moodle",
                semantic_identifier=semantic_id,
                extension=ext,
                blob=blob,
                doc_updated_at=datetime.fromtimestamp(ts or 0, tz=timezone.utc),
                size_bytes=len(blob),
                metadata=metadata,
            )
        except Exception as e:
            self._log_error(f"downloading resource {file_name}", e, "error")
            return None

    def _process_forum(self, course, section, module) -> Optional[Document]:
        if not self.moodle_client or not getattr(module, "instance", None):
            return None

        try:
            result = self.moodle_client.mod.forum.get_forum_discussions(
                forumid=module.instance
            )
            disc_list = getattr(result, "discussions", [])
            if not disc_list:
                return None

            markdown = [f"# {module.name}\n"]
            latest_ts = self._get_latest_timestamp(
                getattr(module, "timecreated", 0),
                getattr(module, "timemodified", 0),
            )

            for d in disc_list:
                markdown.append(f"## {d.name}\n\n{md(d.message or '')}\n\n---\n")
                latest_ts = max(latest_ts, getattr(d, "timemodified", 0))

            blob = "\n".join(markdown).encode("utf-8")
            semantic_id = f"{course.fullname} / {section.name} / {module.name}"

            # Create metadata dictionary with relevant information
            metadata = {
                "moodle_url": self.moodle_url,
                "course_id": getattr(course, "id", None),
                "course_name": getattr(course, "fullname", None),
                "course_shortname": getattr(course, "shortname", None),
                "section_id": getattr(section, "id", None),
                "section_name": getattr(section, "name", None),
                "section_number": getattr(section, "section", None),
                "module_id": getattr(module, "id", None),
                "module_name": getattr(module, "name", None),
                "module_type": getattr(module, "modname", None),
                "forum_id": getattr(module, "instance", None),
                "discussion_count": len(disc_list),
                "time_created": getattr(module, "timecreated", None),
                "time_modified": getattr(module, "timemodified", None),
                "visible": getattr(module, "visible", None),
                "groupmode": getattr(module, "groupmode", None),
                "discussions": [
                    {
                        "id": getattr(d, "id", None),
                        "name": getattr(d, "name", None),
                        "user_id": getattr(d, "userid", None),
                        "user_fullname": getattr(d, "userfullname", None),
                        "time_created": getattr(d, "timecreated", None),
                        "time_modified": getattr(d, "timemodified", None),
                    }
                    for d in disc_list
                ],
            }

            return Document(
                id=f"moodle_forum_{module.id}",
                source="moodle",
                semantic_identifier=semantic_id,
                extension=".md",
                blob=blob,
                doc_updated_at=datetime.fromtimestamp(latest_ts or 0, tz=timezone.utc),
                size_bytes=len(blob),
                metadata=metadata,
            )
        except Exception as e:
            self._log_error(f"processing forum {module.name}", e)
            return None

    def _process_page(self, course, section, module) -> Optional[Document]:
        if not getattr(module, "contents", None):
            return None

        file_info = module.contents[0]
        if not getattr(file_info, "fileurl", None):
            return None

        file_name = os.path.basename(file_info.filename)
        ts = self._get_latest_timestamp(
            getattr(module, "timecreated", 0),
            getattr(module, "timemodified", 0),
            getattr(file_info, "timemodified", 0),
        )

        try:
            resp = rl_requests.get(
                self._add_token_to_url(file_info.fileurl), timeout=60
            )
            resp.raise_for_status()
            blob = resp.content
            ext = os.path.splitext(file_name)[1] or ".html"
            semantic_id = f"{course.fullname} / {section.name} / {module.name}"

            # Create metadata dictionary with relevant information
            metadata = {
                "moodle_url": self.moodle_url,
                "course_id": getattr(course, "id", None),
                "course_name": getattr(course, "fullname", None),
                "course_shortname": getattr(course, "shortname", None),
                "section_id": getattr(section, "id", None),
                "section_name": getattr(section, "name", None),
                "section_number": getattr(section, "section", None),
                "module_id": getattr(module, "id", None),
                "module_name": getattr(module, "name", None),
                "module_type": getattr(module, "modname", None),
                "module_instance": getattr(module, "instance", None),
                "page_url": getattr(file_info, "fileurl", None),
                "file_name": file_name,
                "file_size": getattr(file_info, "filesize", len(blob)),
                "file_type": getattr(file_info, "mimetype", None),
                "time_created": getattr(module, "timecreated", None),
                "time_modified": getattr(module, "timemodified", None),
                "visible": getattr(module, "visible", None),
                "groupmode": getattr(module, "groupmode", None),
            }

            return Document(
                id=f"moodle_page_{module.id}",
                source="moodle",
                semantic_identifier=semantic_id,
                extension=ext,
                blob=blob,
                doc_updated_at=datetime.fromtimestamp(ts or 0, tz=timezone.utc),
                size_bytes=len(blob),
                metadata=metadata,
            )
        except Exception as e:
            self._log_error(f"processing page {file_name}", e, "error")
            return None

    def _process_activity(self, course, section, module) -> Optional[Document]:
        desc = getattr(module, "description", "")
        if not desc:
            return None

        mtype, mname = module.modname, module.name
        markdown = f"# {mname}\n\n**Type:** {mtype.capitalize()}\n\n{md(desc)}"
        ts = self._get_latest_timestamp(
            getattr(module, "timecreated", 0),
            getattr(module, "timemodified", 0),
            getattr(module, "added", 0),
        )

        semantic_id = f"{course.fullname} / {section.name} / {mname}"
        blob = markdown.encode("utf-8")

        # Create metadata dictionary with relevant information
        metadata = {
            "moodle_url": self.moodle_url,
            "course_id": getattr(course, "id", None),
            "course_name": getattr(course, "fullname", None),
            "course_shortname": getattr(course, "shortname", None),
            "section_id": getattr(section, "id", None),
            "section_name": getattr(section, "name", None),
            "section_number": getattr(section, "section", None),
            "module_id": getattr(module, "id", None),
            "module_name": getattr(module, "name", None),
            "module_type": getattr(module, "modname", None),
            "activity_type": mtype,
            "activity_instance": getattr(module, "instance", None),
            "description": desc,
            "time_created": getattr(module, "timecreated", None),
            "time_modified": getattr(module, "timemodified", None),
            "added": getattr(module, "added", None),
            "visible": getattr(module, "visible", None),
            "groupmode": getattr(module, "groupmode", None),
        }

        return Document(
            id=f"moodle_{mtype}_{module.id}",
            source="moodle",
            semantic_identifier=semantic_id,
            extension=".md",
            blob=blob,
            doc_updated_at=datetime.fromtimestamp(ts or 0, tz=timezone.utc),
            size_bytes=len(blob),
            metadata=metadata,
        )

    def _process_book(self, course, section, module) -> Optional[Document]:
        if not getattr(module, "contents", None):
            return None

        contents = module.contents
        chapters = [
            c
            for c in contents
            if getattr(c, "fileurl", None)
            and os.path.basename(c.filename) == "index.html"
        ]
        if not chapters:
            return None

        latest_ts = self._get_latest_timestamp(
            getattr(module, "timecreated", 0),
            getattr(module, "timemodified", 0),
            *[getattr(c, "timecreated", 0) for c in contents],
            *[getattr(c, "timemodified", 0) for c in contents],
        )

        markdown_parts = [f"# {module.name}\n"]
        chapter_info = []

        for ch in chapters:
            try:
                resp = rl_requests.get(self._add_token_to_url(ch.fileurl), timeout=60)
                resp.raise_for_status()
                html = resp.content.decode("utf-8", errors="ignore")
                markdown_parts.append(md(html) + "\n\n---\n")

                # Collect chapter information for metadata
                chapter_info.append(
                    {
                        "chapter_id": getattr(ch, "chapterid", None),
                        "title": getattr(ch, "title", None),
                        "filename": getattr(ch, "filename", None),
                        "fileurl": getattr(ch, "fileurl", None),
                        "time_created": getattr(ch, "timecreated", None),
                        "time_modified": getattr(ch, "timemodified", None),
                        "size": getattr(ch, "filesize", None),
                    }
                )
            except Exception as e:
                self._log_error(f"processing book chapter {ch.filename}", e)

        blob = "\n".join(markdown_parts).encode("utf-8")
        semantic_id = f"{course.fullname} / {section.name} / {module.name}"

        # Create metadata dictionary with relevant information
        metadata = {
            "moodle_url": self.moodle_url,
            "course_id": getattr(course, "id", None),
            "course_name": getattr(course, "fullname", None),
            "course_shortname": getattr(course, "shortname", None),
            "section_id": getattr(section, "id", None),
            "section_name": getattr(section, "name", None),
            "section_number": getattr(section, "section", None),
            "module_id": getattr(module, "id", None),
            "module_name": getattr(module, "name", None),
            "module_type": getattr(module, "modname", None),
            "book_id": getattr(module, "instance", None),
            "chapter_count": len(chapters),
            "chapters": chapter_info,
            "time_created": getattr(module, "timecreated", None),
            "time_modified": getattr(module, "timemodified", None),
            "visible": getattr(module, "visible", None),
            "groupmode": getattr(module, "groupmode", None),
        }

        return Document(
            id=f"moodle_book_{module.id}",
            source="moodle",
            semantic_identifier=semantic_id,
            extension=".md",
            blob=blob,
            doc_updated_at=datetime.fromtimestamp(latest_ts or 0, tz=timezone.utc),
            size_bytes=len(blob),
            metadata=metadata,
        )
