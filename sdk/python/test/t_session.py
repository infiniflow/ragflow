from ragflow import RAGFlow

from common import API_KEY, HOST_ADDRESS


class TestChatSession:
    def test_create_session(self):
        rag = RAGFlow(API_KEY, HOST_ADDRESS)
        assistant = rag.get_assistant(name="test_assistant")
        session = assistant.create_session()
        assert assistant is not None, "Failed to get the assistant."
        assert session is not None, "Failed to create a session."

    def test_create_chat_with_success(self):
        rag = RAGFlow(API_KEY, HOST_ADDRESS)
        assistant = rag.get_assistant(name="test_assistant")
        session = assistant.create_session()
        assert session is not None, "Failed to create a session."
        prologue = assistant.get_prologue()
        assert isinstance(prologue, str), "Prologue is not a string."
        assert len(prologue) > 0, "Prologue is empty."
        question = "What is AI"
        ans = session.chat(question, stream=True)
        response = ans[-1].content
        assert len(response) > 0, "Assistant did not return any response."
