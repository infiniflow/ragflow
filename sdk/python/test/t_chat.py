from ragflow import RAGFlow, Chat
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
    docs= kb.upload_documents(documents)
    for doc in docs:
        doc.add_chunk("This is a test to add chunk")
    rag.create_chat("test_create", dataset_ids=[kb.id])


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
    docs = kb.upload_documents(documents)
    for doc in docs:
        doc.add_chunk("This is a test to add chunk")
    chat = rag.create_chat("test_update", dataset_ids=[kb.id])
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
    docs = kb.upload_documents(documents)
    for doc in docs:
        doc.add_chunk("This is a test to add chunk")
    chat = rag.create_chat("test_delete", dataset_ids=[kb.id])
    rag.delete_chats(ids=[chat.id])

def test_list_chats_with_success(get_api_key_fixture):
    API_KEY = get_api_key_fixture
    rag = RAGFlow(API_KEY, HOST_ADDRESS)
    kb = rag.create_dataset(name="test_delete_chat")
    displayed_name = "ragflow.txt"
    with open("./ragflow.txt", "rb") as file:
        blob = file.read()
    document = {"displayed_name": displayed_name, "blob": blob}
    documents = []
    documents.append(document)
    docs = kb.upload_documents(documents)
    for doc in docs:
        doc.add_chunk("This is a test to add chunk")
    rag.create_chat("test_list_1", dataset_ids=[kb.id])
    rag.create_chat("test_list_2", dataset_ids=[kb.id])
    rag.list_chats()


