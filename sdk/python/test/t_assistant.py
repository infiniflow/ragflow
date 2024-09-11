from ragflow import RAGFlow, Assistant

from common import API_KEY, HOST_ADDRESS
from test_sdkbase import TestSdk


class TestAssistant(TestSdk):
    def test_create_assistant_with_success(self):
        """
        Test creating an assistant with success
        """
        rag = RAGFlow(API_KEY, HOST_ADDRESS)
        kb = rag.get_dataset(name="God")
        assistant = rag.create_assistant("God",knowledgebases=[kb])
        if isinstance(assistant, Assistant):
            assert assistant.name == "God", "Name does not match."
        else:
            assert False, f"Failed to create assistant, error: {assistant}"

    def test_update_assistant_with_success(self):
        """
        Test updating an assistant with success.
        """
        rag = RAGFlow(API_KEY, HOST_ADDRESS)
        kb = rag.get_dataset(name="God")
        assistant = rag.create_assistant("ABC",knowledgebases=[kb])
        if isinstance(assistant, Assistant):
            assert assistant.name == "ABC", "Name does not match."
            assistant.name = 'DEF'
            res = assistant.save()
            assert res is True, f"Failed to update assistant, error: {res}"
        else:
            assert False, f"Failed to create assistant, error: {assistant}"

    def test_delete_assistant_with_success(self):
        """
        Test deleting an assistant with success
        """
        rag = RAGFlow(API_KEY, HOST_ADDRESS)
        kb = rag.get_dataset(name="God")
        assistant = rag.create_assistant("MA",knowledgebases=[kb])
        if isinstance(assistant, Assistant):
            assert assistant.name == "MA", "Name does not match."
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
        assistant = rag.get_assistant(name="God")
        assert isinstance(assistant, Assistant), f"Failed to get assistant, error: {assistant}."
        assert assistant.name == "God", "Name does not match"
