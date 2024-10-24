from ragflow import RAGFlow,Session
import time
HOST_ADDRESS = 'http://127.0.0.1:9380'


def test_create_session_with_success(get_api_key_fixture):
    API_KEY = get_api_key_fixture
    rag = RAGFlow(API_KEY, HOST_ADDRESS)
    kb = rag.create_dataset(name="test_create_session")
    displayed_name = "ragflow.txt"
    with open("./ragflow.txt", "rb") as file:
        blob = file.read()
    document = {"displayed_name": displayed_name, "blob": blob}
    documents = []
    documents.append(document)
    doc_ids = []
    docs = kb.upload_documents(documents)
    for doc in docs:
        doc_ids.append(doc.id)
    kb.async_parse_documents(doc_ids)
    time.sleep(60)
    assistant = rag.create_chat(name="test_create_session", datasets=[kb])
    assistant.create_session()


def test_create_conversation_with_success(get_api_key_fixture):
    API_KEY = get_api_key_fixture
    rag = RAGFlow(API_KEY, HOST_ADDRESS)
    kb = rag.create_dataset(name="test_create_conversation")
    displayed_name = "ragflow.txt"
    with open("./ragflow.txt","rb") as file:
        blob = file.read()
    document = {"displayed_name":displayed_name,"blob":blob}
    documents = []
    documents.append(document)
    doc_ids = []
    docs= kb.upload_documents(documents)
    for doc in docs:
        doc_ids.append(doc.id)
    kb.async_parse_documents(doc_ids)
    time.sleep(60)
    assistant = rag.create_chat(name="test_create_conversation", datasets=[kb])
    session = assistant.create_session()
    question = "What is AI"
    for ans in session.ask(question, stream=True):
        pass
    assert not ans.content.startswith("**ERROR**"), "Please check this error."


def test_delete_sessions_with_success(get_api_key_fixture):
    API_KEY = get_api_key_fixture
    rag = RAGFlow(API_KEY, HOST_ADDRESS)
    kb = rag.create_dataset(name="test_delete_session")
    displayed_name = "ragflow.txt"
    with open("./ragflow.txt","rb") as file:
        blob = file.read()
    document = {"displayed_name":displayed_name,"blob":blob}
    documents = []
    documents.append(document)
    doc_ids = []
    docs= kb.upload_documents(documents)
    for doc in docs:
        doc_ids.append(doc.id)
    kb.async_parse_documents(doc_ids)
    time.sleep(60)
    assistant = rag.create_chat(name="test_delete_session", datasets=[kb])
    session = assistant.create_session()
    assistant.delete_sessions(ids=[session.id])

def test_update_session_with_name(get_api_key_fixture):
    API_KEY = get_api_key_fixture
    rag = RAGFlow(API_KEY, HOST_ADDRESS)
    kb = rag.create_dataset(name="test_update_session")
    displayed_name = "ragflow.txt"
    with open("./ragflow.txt","rb") as file:
        blob = file.read()
    document = {"displayed_name":displayed_name,"blob":blob}
    documents = []
    documents.append(document)
    doc_ids = []
    docs= kb.upload_documents(documents)
    for doc in docs:
        doc_ids.append(doc.id)
    kb.async_parse_documents(doc_ids)
    time.sleep(60)
    assistant = rag.create_chat(name="test_update_session", datasets=[kb])
    session = assistant.create_session(name="old session")
    session.update({"name": "new session"})


def test_list_sessions_with_success(get_api_key_fixture):
    API_KEY = get_api_key_fixture
    rag = RAGFlow(API_KEY, HOST_ADDRESS)
    kb = rag.create_dataset(name="test_list_session")
    displayed_name = "ragflow.txt"
    with open("./ragflow.txt","rb") as file:
        blob = file.read()
    document = {"displayed_name":displayed_name,"blob":blob}
    documents = []
    documents.append(document)
    doc_ids = []
    docs= kb.upload_documents(documents)
    for doc in docs:
        doc_ids.append(doc.id)
    kb.async_parse_documents(doc_ids)
    time.sleep(60)
    assistant = rag.create_chat(name="test_list_session", datasets=[kb])
    assistant.create_session("test_1")
    assistant.create_session("test_2")
    assistant.list_sessions()