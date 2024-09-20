from typing import List, Union

from .base_api import BaseApi


class Dataset(BaseApi):

    def __init__(self, user_key, api_url, authorization_header):
        """
        api_url: http://<host_address>/api/v1
        """
        self.user_key = user_key
        self.api_url = api_url
        self.authorization_header = authorization_header

    def create(self, name: str) -> dict:
        """
        Creates a new Dataset(Knowledgebase).

        :param name: The name of the dataset.

        """
        res = super().post(
            "/datasets",
            {
                "name": name,
            }
        )
        res = res.json()
        if "retmsg" in res and res["retmsg"] == "success":
            return res
        raise Exception(res)

    def list(
            self, page: int = 1, page_size: int = 1024, orderby: str = "create_time", desc: bool = True
    ) -> List:
        """
        Query all Datasets(Knowledgebase).

        :param page: The page number.
        :param page_size: The page size.
        :param orderby: The Field used for sorting.
        :param desc: Whether to sort descending.

        """
        res = super().get("/datasets",
                          {"page": page, "page_size": page_size, "orderby": orderby, "desc": desc})
        res = res.json()
        if "retmsg" in res and res["retmsg"] == "success":
            return res
        raise Exception(res)

    def find_by_name(self, name: str) -> List:
        """
        Query Dataset(Knowledgebase) by Name.

        :param name: The name of the dataset.

        """
        res = super().get("/datasets/search",
                          {"name": name})
        res = res.json()
        if "retmsg" in res and res["retmsg"] == "success":
            return res
        raise Exception(res)

    def update(
            self,
            kb_id: str,
            name: str = None,
            description: str = None,
            permission: str = "me",
            embd_id: str = None,
            language: str = "English",
            parser_id: str = "naive",
            parser_config: dict = None,
            avatar: str = None,
    ) -> dict:
        """
        Updates a Dataset(Knowledgebase).

        :param kb_id: The dataset ID.
        :param name: The name of the dataset.
        :param description: The description of the dataset.
        :param permission: The permission of the dataset.
        :param embd_id: The embedding model ID of the dataset.
        :param language: The language of the dataset.
        :param parser_id: The parsing method of the dataset.
        :param parser_config: The parsing method configuration of the dataset.
        :param avatar: The avatar of the dataset.

        """
        res = super().put(
            "/datasets",
            {
                "kb_id": kb_id,
                "name": name,
                "description": description,
                "permission": permission,
                "embd_id": embd_id,
                "language": language,
                "parser_id": parser_id,
                "parser_config": parser_config,
                "avatar": avatar,
            }
        )
        res = res.json()
        if "retmsg" in res and res["retmsg"] == "success":
            return res
        raise Exception(res)

    def list_documents(
            self, kb_id: str, keywords: str = '', page: int = 1, page_size: int = 1024,
            orderby: str = "create_time", desc: bool = True):
        """
        Query documents file in Dataset(Knowledgebase).

        :param kb_id: The dataset ID.
        :param keywords: Fuzzy search keywords.
        :param page: The page number.
        :param page_size: The page size.
        :param orderby: The Field used for sorting.
        :param desc: Whether to sort descending.

        """
        res = super().get(
            "/documents",
            {
                "kb_id": kb_id, "keywords": keywords, "page": page, "page_size": page_size,
                "orderby": orderby, "desc": desc
            }
        )
        res = res.json()
        if "retmsg" in res and res["retmsg"] == "success":
            return res
        raise Exception(res)

    def retrieval(
            self,
            kb_id: Union[str, List[str]],
            question: str,
            page: int = 1,
            page_size: int = 30,
            similarity_threshold: float = 0.0,
            vector_similarity_weight: float = 0.3,
            top_k: int = 1024,
            rerank_id: str = None,
            keyword: bool = False,
            highlight: bool = False,
            doc_ids: List[str] = None,
    ):
        """
        Run document retrieval in one or more Datasets(Knowledgebase).

        :param kb_id: One or a set of dataset IDs
        :param question: The query question.
        :param page: The page number.
        :param page_size: The page size.
        :param similarity_threshold: The similarity threshold.
        :param vector_similarity_weight: The vector similarity weight.
        :param top_k: Number of top most similar documents to consider (for pre-filtering or ranking).
        :param rerank_id: The rerank model ID.
        :param keyword: Whether you want to enable keyword extraction.
        :param highlight: Whether you want to enable highlighting.
        :param doc_ids: Retrieve only in this set of the documents.

        """
        res = super().post(
            "/datasets/retrieval",
            {
                "kb_id": kb_id,
                "question": question,
                "page": page,
                "page_size": page_size,
                "similarity_threshold": similarity_threshold,
                "vector_similarity_weight": vector_similarity_weight,
                "top_k": top_k,
                "rerank_id": rerank_id,
                "keyword": keyword,
                "highlight": highlight,
                "doc_ids": doc_ids,
            }
        )
        res = res.json()
        if "retmsg" in res and res["retmsg"] == "success":
            return res
        raise Exception(res)
