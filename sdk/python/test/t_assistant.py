from ragflow import RAGFlow, Assistant

from common import API_KEY, HOST_ADDRESS
from test_sdkbase import TestSdk


class TestAssistant(TestSdk):
    def test_create_assistant_with_success(self):
        """
        Test creating an assistant with success
        """
        rag = RAGFlow(API_KEY, HOST_ADDRESS)
        kb = rag.create_dataset(name="test_create_assistant")
        assistant = rag.create_assistant("test_create", knowledgebases=[kb])
        if isinstance(assistant, Assistant):
            assert assistant.name == "test_create", "Name does not match."
        else:
            assert False, f"Failed to create assistant, error: {assistant}"

    def test_update_assistant_with_success(self):
        """
        Test updating an assistant with success.
        """
        rag = RAGFlow(API_KEY, HOST_ADDRESS)
        kb = rag.create_dataset(name="test_update_assistant")
        assistant = rag.create_assistant("test_update", knowledgebases=[kb])
        if isinstance(assistant, Assistant):
            assert assistant.name == "test_update", "Name does not match."
            assistant.name = 'new_assistant'
            res = assistant.save()
            assert res is True, f"Failed to update assistant, error: {res}"
        else:
            assert False, f"Failed to create assistant, error: {assistant}"

    def test_delete_assistant_with_success(self):
        """
        Test deleting an assistant with success
        """
        rag = RAGFlow(API_KEY, HOST_ADDRESS)
        kb = rag.create_dataset(name="test_delete_assistant")
        assistant = rag.create_assistant("test_delete", knowledgebases=[kb])
        if isinstance(assistant, Assistant):
            assert assistant.name == "test_delete", "Name does not match."
            res = assistant.delete()
            assert res is True, f"Failed to delete assistant, error: {res}"
        else:
            assert False, f"Failed to create assistant, error: {assistant}"

    def test_list_assistants_with_success(self):
        """
        Test listing assistants with success
        """
        rag = RAGFlow(API_KEY, HOST_ADDRESS)
        list_assistants = rag.list_assistants()
        assert len(list_assistants) > 0, "Do not exist any assistant"
        for assistant in list_assistants:
            assert isinstance(assistant, Assistant), "Existence type is not assistant."

    def test_get_detail_assistant_with_success(self):
        """
        Test getting an assistant's detail with success
        """
        rag = RAGFlow(API_KEY, HOST_ADDRESS)
        kb = rag.create_dataset(name="test_get_assistant")
        rag.create_assistant("test_get_assistant", knowledgebases=[kb])
        assistant = rag.get_assistant(name="test_get_assistant")
        assert isinstance(assistant, Assistant), f"Failed to get assistant, error: {assistant}."
        assert assistant.name == "test_get_assistant", "Name does not match"
