from ragflow import RAGFlow, Chat
from xgboost.testing import datasets

from common import API_KEY, HOST_ADDRESS
from test_sdkbase import TestSdk


class TestChat(TestSdk):
    def test_create_chat_with_success(self):
        """
        Test creating an chat with success
        """
        rag = RAGFlow(API_KEY, HOST_ADDRESS)
        kb = rag.create_dataset(name="test_create_chat")
        chat = rag.create_chat("test_create", datasets=[kb])
        if isinstance(chat, Chat):
            assert chat.name == "test_create", "Name does not match."
        else:
            assert False, f"Failed to create chat, error: {chat}"

    def test_update_chat_with_success(self):
        """
        Test updating an chat with success.
        """
        rag = RAGFlow(API_KEY, HOST_ADDRESS)
        kb = rag.create_dataset(name="test_update_chat")
        chat = rag.create_chat("test_update", datasets=[kb])
        if isinstance(chat, Chat):
            assert chat.name == "test_update", "Name does not match."
            res=chat.update({"name":"new_chat"})
            assert res is None, f"Failed to update chat, error: {res}"
        else:
            assert False, f"Failed to create chat, error: {chat}"

    def test_delete_chats_with_success(self):
        """
        Test deleting an chat with success
        """
        rag = RAGFlow(API_KEY, HOST_ADDRESS)
        kb = rag.create_dataset(name="test_delete_chat")
        chat = rag.create_chat("test_delete", datasets=[kb])
        if isinstance(chat, Chat):
            assert chat.name == "test_delete", "Name does not match."
            res = rag.delete_chats(ids=[chat.id])
            assert res is None, f"Failed to delete chat, error: {res}"
        else:
            assert False, f"Failed to create chat, error: {chat}"

    def test_list_chats_with_success(self):
        """
        Test listing chats with success
        """
        rag = RAGFlow(API_KEY, HOST_ADDRESS)
        list_chats = rag.list_chats()
        assert len(list_chats) > 0, "Do not exist any chat"
        for chat in list_chats:
            assert isinstance(chat, Chat), "Existence type is not chat."
