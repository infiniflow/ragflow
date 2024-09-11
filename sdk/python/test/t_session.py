from ragflow import RAGFlow,Session

from common import API_KEY, HOST_ADDRESS


class TestSession:
    def test_create_session(self):
        rag = RAGFlow(API_KEY, HOST_ADDRESS)
        kb = rag.create_dataset(name="test_create_session")
        assistant = rag.create_assistant(name="test_create_session", knowledgebases=[kb])
        session = assistant.create_session()
        assert isinstance(session,Session), "Failed to create a session."

    def test_create_chat_with_success(self):
        rag = RAGFlow(API_KEY, HOST_ADDRESS)
        kb = rag.create_dataset(name="test_create_chat")
        assistant = rag.create_assistant(name="test_create_chat", knowledgebases=[kb])
        session = assistant.create_session()
        question = "What is AI"
        for ans in session.chat(question, stream=True):
            pass
        assert ans.content!="\n**ERROR**", "Please check this error."

    def test_delete_session_with_success(self):
        rag = RAGFlow(API_KEY, HOST_ADDRESS)
        kb = rag.create_dataset(name="test_delete_session")
        assistant = rag.create_assistant(name="test_delete_session",knowledgebases=[kb])
        session=assistant.create_session()
        res=session.delete()
        assert res, "Failed to delete the dataset."

    def test_update_session_with_success(self):
        rag=RAGFlow(API_KEY,HOST_ADDRESS)
        kb=rag.create_dataset(name="test_update_session")
        assistant = rag.create_assistant(name="test_update_session",knowledgebases=[kb])
        session=assistant.create_session(name="old session")
        session.name="new session"
        res=session.save()
        assert res,"Failed to update the session"

    def test_get_session_with_success(self):
        rag=RAGFlow(API_KEY,HOST_ADDRESS)
        kb=rag.create_dataset(name="test_get_session")
        assistant = rag.create_assistant(name="test_get_session",knowledgebases=[kb])
        session = assistant.create_session()
        session_2= assistant.get_session(id=session.id)
        assert session.to_json()==session_2.to_json(),"Failed to get the session"

    def test_list_session_with_success(self):
        rag=RAGFlow(API_KEY,HOST_ADDRESS)
        kb=rag.create_dataset(name="test_list_session")
        assistant=rag.create_assistant(name="test_list_session",knowledgebases=[kb])
        assistant.create_session("test_1")
        assistant.create_session("test_2")
        sessions=assistant.list_session()
        if isinstance(sessions,list):
            for session in sessions:
                assert isinstance(session,Session),"Non-Session elements exist in the list"
        else :
            assert False,"Failed to retrieve the session list."