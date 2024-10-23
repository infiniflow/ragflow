from ragflow import RAGFlow,Session

from common import API_KEY, HOST_ADDRESS


class TestSession:
    def test_create_session(self):
        rag = RAGFlow(API_KEY, HOST_ADDRESS)
        kb = rag.create_dataset(name="test_create_session")
        assistant = rag.create_chat(name="test_create_session", datasets=[kb])
        session = assistant.create_session()
        assert isinstance(session,Session), "Failed to create a session."

    def test_create_chat_with_success(self):
        rag = RAGFlow(API_KEY, HOST_ADDRESS)
        kb = rag.create_dataset(name="test_create_chat")
        assistant = rag.create_chat(name="test_create_chat", datasets=[kb])
        session = assistant.create_session()
        question = "What is AI"
        for ans in session.ask(question, stream=True):
            pass
        assert not ans.content.startswith("**ERROR**"), "Please check this error."

    def test_delete_sessions_with_success(self):
        rag = RAGFlow(API_KEY, HOST_ADDRESS)
        kb = rag.create_dataset(name="test_delete_session")
        assistant = rag.create_chat(name="test_delete_session",datasets=[kb])
        session=assistant.create_session()
        res=assistant.delete_sessions(ids=[session.id])
        assert res is None, "Failed to delete the dataset."

    def test_update_session_with_success(self):
        rag=RAGFlow(API_KEY,HOST_ADDRESS)
        kb=rag.create_dataset(name="test_update_session")
        assistant = rag.create_chat(name="test_update_session",datasets=[kb])
        session=assistant.create_session(name="old session")
        res=session.update({"name":"new session"})
        assert res is None,"Failed to update the session"


    def test_list_sessions_with_success(self):
        rag=RAGFlow(API_KEY,HOST_ADDRESS)
        kb=rag.create_dataset(name="test_list_session")
        assistant=rag.create_chat(name="test_list_session",datasets=[kb])
        assistant.create_session("test_1")
        assistant.create_session("test_2")
        sessions=assistant.list_sessions()
        if isinstance(sessions,list):
            for session in sessions:
                assert isinstance(session,Session),"Non-Session elements exist in the list"
        else :
            assert False,"Failed to retrieve the session list."