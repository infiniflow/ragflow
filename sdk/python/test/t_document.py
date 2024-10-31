from ragflow_sdk import RAGFlow, DataSet, Document, Chunk
from common import HOST_ADDRESS


def test_upload_document_with_success(get_api_key_fixture):
    API_KEY = get_api_key_fixture
    rag = RAGFlow(API_KEY, HOST_ADDRESS)
    ds = rag.create_dataset(name="test_upload_document")
    blob = b"Sample document content for test."
    with open("ragflow.txt","rb") as file:
        blob_2=file.read()
    document_infos = []
    document_infos.append({"displayed_name": "test_1.txt","blob": blob})
    document_infos.append({"displayed_name": "test_2.txt","blob": blob_2})
    ds.upload_documents(document_infos)


def test_update_document_with_success(get_api_key_fixture):
    API_KEY = get_api_key_fixture
    rag = RAGFlow(API_KEY, HOST_ADDRESS)
    ds = rag.create_dataset(name="test_update_document")
    blob = b"Sample document content for test."
    document_infos=[{"displayed_name":"test.txt","blob":blob}]
    docs=ds.upload_documents(document_infos)
    doc = docs[0]
    doc.update({"chunk_method": "manual", "name": "manual.txt"})


def test_download_document_with_success(get_api_key_fixture):
    API_KEY = get_api_key_fixture
    rag = RAGFlow(API_KEY, HOST_ADDRESS)
    ds = rag.create_dataset(name="test_download_document")
    blob = b"Sample document content for test."
    document_infos=[{"displayed_name": "test_1.txt","blob": blob}]
    docs=ds.upload_documents(document_infos)
    doc = docs[0]
    with open("test_download.txt","wb+") as file:
        file.write(doc.download())


def test_list_documents_in_dataset_with_success(get_api_key_fixture):
    API_KEY = get_api_key_fixture
    rag = RAGFlow(API_KEY, HOST_ADDRESS)
    ds = rag.create_dataset(name="test_list_documents")
    blob = b"Sample document content for test."
    document_infos = [{"displayed_name": "test.txt","blob":blob}]
    ds.upload_documents(document_infos)
    ds.list_documents(keywords="test", offset=0, limit=12)



def test_delete_documents_in_dataset_with_success(get_api_key_fixture):
    API_KEY = get_api_key_fixture
    rag = RAGFlow(API_KEY, HOST_ADDRESS)
    ds = rag.create_dataset(name="test_delete_documents")
    name = "test_delete_documents.txt"
    blob = b"Sample document content for test."
    document_infos=[{"displayed_name": name, "blob": blob}]
    docs = ds.upload_documents(document_infos)
    ds.delete_documents([docs[0].id])


