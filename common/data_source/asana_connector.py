from collections.abc import Iterator
import time
from datetime import datetime
import logging
from typing import Any, Dict
import asana
import requests
from common.data_source.config import CONTINUE_ON_CONNECTOR_FAILURE, INDEX_BATCH_SIZE, DocumentSource
from common.data_source.interfaces import LoadConnector, PollConnector
from common.data_source.models import Document, GenerateDocumentsOutput, SecondsSinceUnixEpoch
from common.data_source.utils import extract_size_bytes, get_file_ext



# https://github.com/Asana/python-asana/tree/master?tab=readme-ov-file#documentation-for-api-endpoints
class AsanaTask:
    def __init__(
        self,
        id: str,
        title: str,
        text: str,
        link: str,
        last_modified: datetime,
        project_gid: str,
        project_name: str,
    ) -> None:
        self.id = id
        self.title = title
        self.text = text
        self.link = link
        self.last_modified = last_modified
        self.project_gid = project_gid
        self.project_name = project_name

    def __str__(self) -> str:
        return f"ID: {self.id}\nTitle: {self.title}\nLast modified: {self.last_modified}\nText: {self.text}"


class AsanaAPI:
    def __init__(
        self, api_token: str, workspace_gid: str, team_gid: str | None
    ) -> None:
        self._user = None
        self.workspace_gid = workspace_gid
        self.team_gid = team_gid

        self.configuration = asana.Configuration()
        self.api_client = asana.ApiClient(self.configuration)
        self.tasks_api = asana.TasksApi(self.api_client)
        self.attachments_api = asana.AttachmentsApi(self.api_client)
        self.stories_api = asana.StoriesApi(self.api_client)
        self.users_api = asana.UsersApi(self.api_client)
        self.project_api = asana.ProjectsApi(self.api_client)
        self.project_memberships_api = asana.ProjectMembershipsApi(self.api_client)
        self.workspaces_api = asana.WorkspacesApi(self.api_client)

        self.api_error_count = 0
        self.configuration.access_token = api_token
        self.task_count = 0

    def get_tasks(
        self, project_gids: list[str] | None, start_date: str
    ) -> Iterator[AsanaTask]:
        """Get all tasks from the projects with the given gids that were modified since the given date.
        If project_gids is None, get all tasks from all projects in the workspace."""
        logging.info("Starting to fetch Asana projects")
        projects = self.project_api.get_projects(
            opts={
                "workspace": self.workspace_gid,
                "opt_fields": "gid,name,archived,modified_at",
            }
        )
        start_seconds = int(time.mktime(datetime.now().timetuple()))
        projects_list = []
        project_count = 0
        for project_info in projects:
            project_gid = project_info["gid"]
            if project_gids is None or project_gid in project_gids:
                projects_list.append(project_gid)
            else:
                logging.debug(
                    f"Skipping project: {project_gid} - not in accepted project_gids"
                )
            project_count += 1
            if project_count % 100 == 0:
                logging.info(f"Processed {project_count} projects")
        logging.info(f"Found {len(projects_list)} projects to process")
        for project_gid in projects_list:
            for task in self._get_tasks_for_project(
                project_gid, start_date, start_seconds
            ):
                yield task
        logging.info(f"Completed fetching {self.task_count} tasks from Asana")
        if self.api_error_count > 0:
            logging.warning(
                f"Encountered {self.api_error_count} API errors during task fetching"
            )

    def _get_tasks_for_project(
        self, project_gid: str, start_date: str, start_seconds: int
    ) -> Iterator[AsanaTask]:
        project = self.project_api.get_project(project_gid, opts={})
        project_name = project.get("name", project_gid)
        team = project.get("team") or {}
        team_gid = team.get("gid")

        if project.get("archived"):
            logging.info(f"Skipping archived project: {project_name} ({project_gid})")
            return
        if not team_gid:
            logging.info(
                f"Skipping project without a team: {project_name} ({project_gid})"
            )
            return
        if project.get("privacy_setting") == "private":
            if self.team_gid and team_gid != self.team_gid:
                logging.info(
                    f"Skipping private project not in configured team: {project_name} ({project_gid})"
                )
                return
            logging.info(
                f"Processing private project in configured team: {project_name} ({project_gid})"
            )

        simple_start_date = start_date.split(".")[0].split("+")[0]
        logging.info(
            f"Fetching tasks modified since {simple_start_date} for project: {project_name} ({project_gid})"
        )

        opts = {
            "opt_fields": "name,memberships,memberships.project,completed_at,completed_by,created_at,"
            "created_by,custom_fields,dependencies,due_at,due_on,external,html_notes,liked,likes,"
            "modified_at,notes,num_hearts,parent,projects,resource_subtype,resource_type,start_on,"
            "workspace,permalink_url",
            "modified_since": start_date,
        }
        tasks_from_api = self.tasks_api.get_tasks_for_project(project_gid, opts)
        for data in tasks_from_api:
            self.task_count += 1
            if self.task_count % 10 == 0:
                end_seconds = time.mktime(datetime.now().timetuple())
                runtime_seconds = end_seconds - start_seconds
                if runtime_seconds > 0:
                    logging.info(
                        f"Processed {self.task_count} tasks in {runtime_seconds:.0f} seconds "
                        f"({self.task_count / runtime_seconds:.2f} tasks/second)"
                    )

            logging.debug(f"Processing Asana task: {data['name']}")

            text = self._construct_task_text(data)

            try:
                text += self._fetch_and_add_comments(data["gid"])

                last_modified_date = self.format_date(data["modified_at"])
                text += f"Last modified: {last_modified_date}\n"

                task = AsanaTask(
                    id=data["gid"],
                    title=data["name"],
                    text=text,
                    link=data["permalink_url"],
                    last_modified=datetime.fromisoformat(data["modified_at"]),
                    project_gid=project_gid,
                    project_name=project_name,
                )
                yield task
            except Exception:
                logging.error(
                    f"Error processing task {data['gid']} in project {project_gid}",
                    exc_info=True,
                )
                self.api_error_count += 1

    def _construct_task_text(self, data: Dict) -> str:
        text = f"{data['name']}\n\n"

        if data["notes"]:
            text += f"{data['notes']}\n\n"

        if data["created_by"] and data["created_by"]["gid"]:
            creator = self.get_user(data["created_by"]["gid"])["name"]
            created_date = self.format_date(data["created_at"])
            text += f"Created by: {creator} on {created_date}\n"

        if data["due_on"]:
            due_date = self.format_date(data["due_on"])
            text += f"Due date: {due_date}\n"

        if data["completed_at"]:
            completed_date = self.format_date(data["completed_at"])
            text += f"Completed on: {completed_date}\n"

        text += "\n"
        return text

    def _fetch_and_add_comments(self, task_gid: str) -> str:
        text = ""
        stories_opts: Dict[str, str] = {}
        story_start = time.time()
        stories = self.stories_api.get_stories_for_task(task_gid, stories_opts)

        story_count = 0
        comment_count = 0

        for story in stories:
            story_count += 1
            if story["resource_subtype"] == "comment_added":
                comment = self.stories_api.get_story(
                    story["gid"], opts={"opt_fields": "text,created_by,created_at"}
                )
                commenter = self.get_user(comment["created_by"]["gid"])["name"]
                text += f"Comment by {commenter}: {comment['text']}\n\n"
                comment_count += 1

        story_duration = time.time() - story_start
        logging.debug(
            f"Processed {story_count} stories (including {comment_count} comments) in {story_duration:.2f} seconds"
        )

        return text

    def get_attachments(self, task_gid: str) -> list[dict]:
        """
        Fetch full attachment info (including download_url) for a task.
        """
        attachments: list[dict] = []

        try:
            # Step 1: list attachment compact records
            for att in self.attachments_api.get_attachments_for_object(
                parent=task_gid,
                opts={}
            ):
                gid = att.get("gid")
                if not gid:
                    continue

                try:
                    # Step 2: expand to full attachment
                    full = self.attachments_api.get_attachment(
                        attachment_gid=gid,
                        opts={
                            "opt_fields": "name,download_url,size,created_at"
                        }
                    )

                    if full.get("download_url"):
                        attachments.append(full)

                except Exception:
                    logging.exception(
                        f"Failed to fetch attachment detail {gid} for task {task_gid}"
                    )
                    self.api_error_count += 1

        except Exception:
            logging.exception(f"Failed to list attachments for task {task_gid}")
            self.api_error_count += 1

        return attachments

    def get_accessible_emails(
        self,
        workspace_id: str,
        project_ids: list[str] | None,
        team_id: str | None,
    ):

        ws_users = self.users_api.get_users(
            opts={
                "workspace": workspace_id,
                "opt_fields": "gid,name,email"
            }
        )

        workspace_users = {
            u["gid"]: u.get("email")
            for u in ws_users
            if u.get("email")
        }

        if not project_ids:
            return set(workspace_users.values())


        project_emails = set()

        for pid in project_ids:
            project = self.project_api.get_project(
                pid,
                opts={"opt_fields": "team,privacy_setting"}
            )

            if project["privacy_setting"] == "private":
                if team_id and project.get("team", {}).get("gid") != team_id:
                    continue

            memberships = self.project_memberships_api.get_project_membership(
                pid,
                opts={"opt_fields": "user.gid,user.email"}
            )

            for m in memberships:
                email = m["user"].get("email")
                if email:
                    project_emails.add(email)

        return project_emails

    def get_user(self, user_gid: str) -> Dict:
        if self._user is not None:
            return self._user
        self._user = self.users_api.get_user(user_gid, {"opt_fields": "name,email"})

        if not self._user:
            logging.warning(f"Unable to fetch user information for user_gid: {user_gid}")
            return {"name": "Unknown"}
        return self._user

    def format_date(self, date_str: str) -> str:
        date = datetime.fromisoformat(date_str)
        return time.strftime("%Y-%m-%d", date.timetuple())

    def get_time(self) -> str:
        return time.strftime("%Y-%m-%d %H:%M:%S", time.localtime())


class AsanaConnector(LoadConnector, PollConnector):
    def __init__(
        self,
        asana_workspace_id: str,
        asana_project_ids: str | None = None,
        asana_team_id: str | None = None,
        batch_size: int = INDEX_BATCH_SIZE,
        continue_on_failure: bool = CONTINUE_ON_CONNECTOR_FAILURE,
    ) -> None:
        self.workspace_id = asana_workspace_id
        self.project_ids_to_index: list[str] | None = (
            asana_project_ids.split(",") if asana_project_ids else None
        )
        self.asana_team_id = asana_team_id if asana_team_id else None
        self.batch_size = batch_size
        self.continue_on_failure = continue_on_failure
        self.size_threshold = None
        logging.info(
            f"AsanaConnector initialized with workspace_id: {asana_workspace_id}"
        )

    def load_credentials(self, credentials: dict[str, Any]) -> dict[str, Any] | None:
        self.api_token = credentials["asana_api_token_secret"]
        self.asana_client = AsanaAPI(
            api_token=self.api_token,
            workspace_gid=self.workspace_id,
            team_gid=self.asana_team_id,
        )
        self.workspace_users_email = self.asana_client.get_accessible_emails(self.workspace_id, self.project_ids_to_index, self.asana_team_id)
        logging.info("Asana credentials loaded and API client initialized")
        return None

    def poll_source(
        self, start: SecondsSinceUnixEpoch, end: SecondsSinceUnixEpoch | None
    ) -> GenerateDocumentsOutput:
        start_time = datetime.fromtimestamp(start).isoformat()
        logging.info(f"Starting Asana poll from {start_time}")
        docs_batch: list[Document] = []
        tasks = self.asana_client.get_tasks(self.project_ids_to_index, start_time)
        for task in tasks:
            docs = self._task_to_documents(task)
            docs_batch.extend(docs)

            if len(docs_batch) >= self.batch_size:
                logging.info(f"Yielding batch of {len(docs_batch)} documents")
                yield docs_batch
                docs_batch = []

        if docs_batch:
            logging.info(f"Yielding final batch of {len(docs_batch)} documents")
            yield docs_batch

        logging.info("Asana poll completed")

    def load_from_state(self) -> GenerateDocumentsOutput:
        logging.info("Starting full index of all Asana tasks")
        return self.poll_source(start=0, end=None)

    def _task_to_documents(self, task: AsanaTask) -> list[Document]:
        docs: list[Document] = []

        attachments = self.asana_client.get_attachments(task.id)

        for att in attachments:
            try:
                resp = requests.get(att["download_url"], timeout=30)
                resp.raise_for_status()
                file_blob = resp.content
                filename = att.get("name", "attachment")
                size_bytes = extract_size_bytes(att)
                if (
                    self.size_threshold is not None
                    and isinstance(size_bytes, int)
                    and size_bytes > self.size_threshold
                ):
                    logging.warning(
                        f"{filename} exceeds size threshold of {self.size_threshold}. Skipping."
                    )
                    continue
                docs.append(
                    Document(
                        id=f"asana:{task.id}:{att['gid']}",
                        blob=file_blob,
                        extension=get_file_ext(filename) or "",
                        size_bytes=size_bytes,
                        doc_updated_at=task.last_modified,
                        source=DocumentSource.ASANA,
                        semantic_identifier=filename,
                        primary_owners=list(self.workspace_users_email),
                    )
                )
            except Exception:
                logging.exception(
                    f"Failed to download attachment {att.get('gid')} for task {task.id}"
                )

        return docs



if __name__ == "__main__":
    import time
    import os

    logging.info("Starting Asana connector test")
    connector = AsanaConnector(
        os.environ["WORKSPACE_ID"],
        os.environ["PROJECT_IDS"],
        os.environ["TEAM_ID"],
    )
    connector.load_credentials(
        {
            "asana_api_token_secret": os.environ["API_TOKEN"],
        }
    )
    logging.info("Loading all documents from Asana")
    all_docs = connector.load_from_state()
    current = time.time()
    one_day_ago = current - 24 * 60 * 60  # 1 day
    logging.info("Polling for documents updated in the last 24 hours")
    latest_docs = connector.poll_source(one_day_ago, current)
    for docs in all_docs:
        for doc in docs:
            print(doc.id)
    logging.info("Asana connector test completed")