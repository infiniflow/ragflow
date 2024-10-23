from ragflow import RAGFlow, DataSet, Document, Chunk

HOST_ADDRESS = 'http://127.0.0.1:9380'


def test_upload_document_with_success(get_api_key_fixture):
    """
    Test ingesting a document into a dataset with success.
    """
    # Initialize RAGFlow instance
    API_KEY = get_api_key_fixture
    rag = RAGFlow(API_KEY, HOST_ADDRESS)

    # Step 1: Create a new dataset
    ds = rag.create_dataset(name="God")

    # Ensure dataset creation was successful
    assert isinstance(ds, DataSet), f"Failed to create dataset, error: {ds}"
    assert ds.name == "God", "Dataset name does not match."

    # Step 2: Create a new document
    # The blob is the actual file content or a placeholder in this case
    blob = b"Sample document content for ingestion test."
    blob_2 = b"test_2."
    list_1 = []
    list_1.append({"name": "Test_1.txt",
                   "blob": blob})
    list_1.append({"name": "Test_2.txt",
                   "blob": blob_2})
    res = ds.upload_documents(list_1)
    # Ensure document ingestion was successful
    assert res is None, f"Failed to create document, error: {res}"


def test_update_document_with_success(get_api_key_fixture):
    """
    Test updating a document with success.
    Update name or chunk_method are supported
    """
    API_KEY = get_api_key_fixture
    rag = RAGFlow(API_KEY, HOST_ADDRESS)
    ds = rag.list_datasets(name="God")
    ds = ds[0]
    doc = ds.list_documents()
    doc = doc[0]
    if isinstance(doc, Document):
        res = doc.update({"chunk_method": "manual", "name": "manual.txt"})
        assert res is None, f"Failed to update document, error: {res}"
    else:
        assert False, f"Failed to get document, error: {doc}"


def test_download_document_with_success(get_api_key_fixture):
    """
    Test downloading a document with success.
    """
    API_KEY = get_api_key_fixture
    # Initialize RAGFlow instance
    rag = RAGFlow(API_KEY, HOST_ADDRESS)

    # Retrieve a document
    ds = rag.list_datasets(name="God")
    ds = ds[0]
    doc = ds.list_documents(name="manual.txt")
    doc = doc[0]
    # Check if the retrieved document is of type Document
    if isinstance(doc, Document):
        # Download the document content and save it to a file
        with open("./ragflow.txt", "wb+") as file:
            file.write(doc.download())
            # Print the document object for debugging
        print(doc)

        # Assert that the download was successful
        assert True, f"Failed to download document, error: {doc}"
    else:
        # If the document retrieval fails, assert failure
        assert False, f"Failed to get document, error: {doc}"


def test_list_documents_in_dataset_with_success(get_api_key_fixture):
    """
    Test list all documents into a dataset with success.
    """
    API_KEY = get_api_key_fixture
    # Initialize RAGFlow instance
    rag = RAGFlow(API_KEY, HOST_ADDRESS)

    # Step 1: Create a new dataset
    ds = rag.create_dataset(name="God2")

    # Ensure dataset creation was successful
    assert isinstance(ds, DataSet), f"Failed to create dataset, error: {ds}"
    assert ds.name == "God2", "Dataset name does not match."

    # Step 2: Create a new document
    # The blob is the actual file content or a placeholder in this case
    name1 = "Test Document111.txt"
    blob1 = b"Sample document content for ingestion test111."
    name2 = "Test Document222.txt"
    blob2 = b"Sample document content for ingestion test222."
    list_1 = [{"name": name1, "blob": blob1}, {"name": name2, "blob": blob2}]
    ds.upload_documents(list_1)
    for d in ds.list_documents(keywords="test", offset=0, limit=12):
        assert isinstance(d, Document), "Failed to upload documents"


def test_delete_documents_in_dataset_with_success(get_api_key_fixture):
    """
    Test list all documents into a dataset with success.
    """
    API_KEY = get_api_key_fixture
    # Initialize RAGFlow instance
    rag = RAGFlow(API_KEY, HOST_ADDRESS)

    # Step 1: Create a new dataset
    ds = rag.create_dataset(name="God3")

    # Ensure dataset creation was successful
    assert isinstance(ds, DataSet), f"Failed to create dataset, error: {ds}"
    assert ds.name == "God3", "Dataset name does not match."

    # Step 2: Create a new document
    # The blob is the actual file content or a placeholder in this case
    name1 = "Test Document333.txt"
    blob1 = b"Sample document content for ingestion test333."
    name2 = "Test Document444.txt"
    blob2 = b"Sample document content for ingestion test444."
    ds.upload_documents([{"name": name1, "blob": blob1}, {"name": name2, "blob": blob2}])
    for d in ds.list_documents(keywords="document", offset=0, limit=12):
        assert isinstance(d, Document)
        ds.delete_documents([d.id])
    remaining_docs = ds.list_documents(keywords="rag", offset=0, limit=12)
    assert len(remaining_docs) == 0, "Documents were not properly deleted."


def test_parse_and_cancel_document(get_api_key_fixture):
    API_KEY = get_api_key_fixture
    # Initialize RAGFlow with API key and host address
    rag = RAGFlow(API_KEY, HOST_ADDRESS)

    # Create a dataset with a specific name
    ds = rag.create_dataset(name="God4")

    # Define the document name and path
    name3 = 'westworld.pdf'
    path = './test_data/westworld.pdf'

    # Create a document in the dataset using the file path
    ds.upload_documents({"name": name3, "blob": open(path, "rb").read()})

    # Retrieve the document by name
    doc = rag.list_documents(name="westworld.pdf")
    doc = doc[0]
    ds.async_parse_documents(document_ids=[])

    # Print message to confirm asynchronous parsing has been initiated
    print("Async parsing initiated")

    # Use join to wait for parsing to complete and get progress updates
    for progress, msg in doc.join(interval=5, timeout=10):
        print(progress, msg)
        # Assert that the progress is within the valid range (0 to 100)
        assert 0 <= progress <= 100, f"Invalid progress: {progress}"
        # Assert that the message is not empty
        assert msg, "Message should not be empty"
        # Test cancelling the parsing operation
    doc.cancel()
    # Print message to confirm parsing has been cancelled successfully
    print("Parsing cancelled successfully")


def test_bulk_parse_and_cancel_documents(get_api_key_fixture):
    API_KEY = get_api_key_fixture
    # Initialize RAGFlow with API key and host address
    rag = RAGFlow(API_KEY, HOST_ADDRESS)

    # Create a dataset
    ds = rag.create_dataset(name="God5")
    assert ds is not None, "Dataset creation failed"
    assert ds.name == "God5", "Dataset name does not match"

    # Prepare a list of file names and paths
    documents = [
        {'name': 'test1.txt', 'path': 'test_data/test1.txt'},
        {'name': 'test2.txt', 'path': 'test_data/test2.txt'},
        {'name': 'test3.txt', 'path': 'test_data/test3.txt'}
    ]

    # Create documents in bulk
    for doc_info in documents:
        with open(doc_info['path'], "rb") as file:
            created_doc = rag.create_document(ds, name=doc_info['name'], blob=file.read())
            assert created_doc is not None, f"Failed to create document {doc_info['name']}"

            # Retrieve document objects in bulk
    docs = [rag.get_document(name=doc_info['name']) for doc_info in documents]
    ids = [doc.id for doc in docs]
    assert len(docs) == len(documents), "Mismatch between created documents and fetched documents"

    # Initiate asynchronous parsing for all documents
    rag.async_parse_documents(ids)
    print("Async bulk parsing initiated")

    # Wait for all documents to finish parsing and check progress
    for doc in docs:
        for progress, msg in doc.join(interval=5, timeout=10):
            print(f"{doc.name}: Progress: {progress}, Message: {msg}")

            # Assert that progress is within the valid range
            assert 0 <= progress <= 100, f"Invalid progress: {progress} for document {doc.name}"

            # Assert that the message is not empty
            assert msg, f"Message should not be empty for document {doc.name}"

            # If progress reaches 100%, assert that parsing is completed successfully
            if progress == 100:
                assert "completed" in msg.lower(), f"Document {doc.name} did not complete successfully"

                # Cancel parsing for all documents in bulk
    cancel_result = rag.async_cancel_parse_documents(ids)
    assert cancel_result is None or isinstance(cancel_result, type(None)), "Failed to cancel document parsing"
    print("Async bulk parsing cancelled")


def test_parse_document_and_chunk_list(get_api_key_fixture):
    API_KEY = get_api_key_fixture
    rag = RAGFlow(API_KEY, HOST_ADDRESS)
    ds = rag.create_dataset(name="God7")
    name = 'story.txt'
    path = 'test_data/story.txt'
    # name = "Test Document rag.txt"
    # blob = " Sample document content for rag test66. rag wonderful apple os documents apps. Sample document content for rag test66. rag wonderful apple os documents apps.Sample document content for rag test66. rag wonderful apple os documents apps.Sample document content for rag test66. rag wonderful apple os documents apps. Sample document content for rag test66. rag wonderful apple os documents apps. Sample document content for rag test66. rag wonderful apple os documents apps. Sample document content for rag test66. rag wonderful apple os documents apps. Sample document content for rag test66. rag wonderful apple os documents apps. Sample document content for rag test66. rag wonderful apple os documents apps.  Sample document content for rag test66. rag wonderful apple os documents apps. Sample document content for rag test66. rag wonderful apple os documents apps. Sample document content for rag test66. rag wonderful apple os documents apps."
    rag.create_document(ds, name=name, blob=open(path, "rb").read())
    doc = rag.get_document(name=name)
    doc.async_parse()

    # Wait for parsing to complete and get progress updates using join
    for progress, msg in doc.join(interval=5, timeout=30):
        print(progress, msg)
        # Assert that progress is within 0 to 100
        assert 0 <= progress <= 100, f"Invalid progress: {progress}"
        # Assert that the message is not empty
        assert msg, "Message should not be empty"

    for c in doc.list_chunks(keywords="rag", offset=0, limit=12):
        print(c)
        assert c is not None, "Chunk is None"
        assert "rag" in c['content_with_weight'].lower(), f"Keyword 'rag' not found in chunk content: {c.content}"


def test_add_chunk_to_chunk_list(get_api_key_fixture):
    API_KEY = get_api_key_fixture
    rag = RAGFlow(API_KEY, HOST_ADDRESS)
    doc = rag.get_document(name='story.txt')
    chunk = doc.add_chunk(content="assssdd")
    assert chunk is not None, "Chunk is None"
    assert isinstance(chunk, Chunk), "Chunk was not added to chunk list"


def test_delete_chunk_of_chunk_list(get_api_key_fixture):
    API_KEY = get_api_key_fixture
    rag = RAGFlow(API_KEY, HOST_ADDRESS)
    doc = rag.get_document(name='story.txt')
    chunk = doc.add_chunk(content="assssdd")
    assert chunk is not None, "Chunk is None"
    assert isinstance(chunk, Chunk), "Chunk was not added to chunk list"
    doc = rag.get_document(name='story.txt')
    chunk_count_before = doc.chunk_count
    chunk.delete()
    doc = rag.get_document(name='story.txt')
    assert doc.chunk_count == chunk_count_before - 1, "Chunk was not deleted"


def test_update_chunk_content(get_api_key_fixture):
    API_KEY = get_api_key_fixture
    rag = RAGFlow(API_KEY, HOST_ADDRESS)
    doc = rag.get_document(name='story.txt')
    chunk = doc.add_chunk(content="assssddd")
    assert chunk is not None, "Chunk is None"
    assert isinstance(chunk, Chunk), "Chunk was not added to chunk list"
    chunk.content = "ragflow123"
    res = chunk.save()
    assert res is True, f"Failed to update chunk content, error: {res}"


def test_update_chunk_available(get_api_key_fixture):
    API_KEY = get_api_key_fixture
    rag = RAGFlow(API_KEY, HOST_ADDRESS)
    doc = rag.get_document(name='story.txt')
    chunk = doc.add_chunk(content="ragflow")
    assert chunk is not None, "Chunk is None"
    assert isinstance(chunk, Chunk), "Chunk was not added to chunk list"
    chunk.available = 0
    res = chunk.save()
    assert res is True, f"Failed to update chunk status, error: {res}"


def test_retrieval_chunks(get_api_key_fixture):
    API_KEY = get_api_key_fixture
    rag = RAGFlow(API_KEY, HOST_ADDRESS)
    ds = rag.create_dataset(name="God8")
    name = 'ragflow_test.txt'
    path = 'test_data/ragflow_test.txt'
    rag.create_document(ds, name=name, blob=open(path, "rb").read())
    doc = rag.get_document(name=name)
    doc.async_parse()
    # Wait for parsing to complete and get progress updates using join
    for progress, msg in doc.join(interval=5, timeout=30):
        print(progress, msg)
        assert 0 <= progress <= 100, f"Invalid progress: {progress}"
        assert msg, "Message should not be empty"
    for c in rag.retrieval(question="What's ragflow?",
                           datasets=[ds.id], documents=[doc],
                           offset=0, limit=6, similarity_threshold=0.1,
                           vector_similarity_weight=0.3,
                           top_k=1024
                           ):
        print(c)
        assert c is not None, "Chunk is None"
        assert "ragflow" in c.content.lower(), f"Keyword 'rag' not found in chunk content: {c.content}"
