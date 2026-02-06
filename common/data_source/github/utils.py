import logging

from github import Github
from github.Repository import Repository

from common.data_source.models import ExternalAccess

from .models import SerializedRepository


def get_external_access_permission(
    repo: Repository, github_client: Github
) -> ExternalAccess:
    """
    Get the external access permission for a repository.
    This functionality requires Enterprise Edition.
    """
    # RAGFlow doesn't implement the Onyx EE external-permissions system.
    # Default to private/unknown permissions.
    return ExternalAccess.empty()


def deserialize_repository(
    cached_repo: SerializedRepository, github_client: Github
) -> Repository:
    """
    Deserialize a SerializedRepository back into a Repository object.
    """
    # Try to access the requester - different PyGithub versions may use different attribute names
    try:
        # Try to get the requester using getattr to avoid linter errors
        requester = getattr(github_client, "_requester", None)
        if requester is None:
            requester = getattr(github_client, "_Github__requester", None)
        if requester is None:
            # If we can't find the requester attribute, we need to fall back to recreating the repo
            raise AttributeError("Could not find requester attribute")

        return cached_repo.to_Repository(requester)
    except Exception as e:
        # If all else fails, re-fetch the repo directly
        logging.warning("Failed to deserialize repository: %s. Attempting to re-fetch.", e)
        repo_id = cached_repo.id
        return github_client.get_repo(repo_id)