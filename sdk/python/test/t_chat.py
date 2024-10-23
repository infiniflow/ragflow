from ragflow import RAGFlow, Chat
import time
HOST_ADDRESS = 'http://127.0.0.1:9380'

def test_create_chat_with_name(get_api_key_fixture):
    API_KEY = get_api_key_fixture
    rag = RAGFlow(API_KEY, HOST_ADDRESS)
    kb = rag.create_dataset(name="test_create_chat")
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
    rag.create_chat("test_create", datasets=[kb])


def test_update_chat_with_name(get_api_key_fixture):
    API_KEY = get_api_key_fixture
    rag = RAGFlow(API_KEY, HOST_ADDRESS)
    kb = rag.create_dataset(name="test_update_chat")
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
    chat = rag.create_chat("test_update", datasets=[kb])
    chat.update({"name": "new_chat"})


def test_delete_chats_with_success(get_api_key_fixture):
    API_KEY = get_api_key_fixture
    rag = RAGFlow(API_KEY, HOST_ADDRESS)
    kb = rag.create_dataset(name="test_delete_chat")
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
    chat = rag.create_chat("test_delete", datasets=[kb])
    rag.delete_chats(ids=[chat.id])

    API_KEY = get_api_key_fixture
    rag = RAGFlow(API_KEY, HOST_ADDRESS)
    rag.list_chats()


