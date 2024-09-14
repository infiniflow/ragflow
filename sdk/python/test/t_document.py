from ragflow import RAGFlow, DataSet, Document, Chunk

from common import API_KEY, HOST_ADDRESS
from test_sdkbase import TestSdk


class TestDocument(TestSdk):
    def test_upload_document_with_success(self):
        """
        Test ingesting a document into a dataset with success.
        """
        # Initialize RAGFlow instance
        rag = RAGFlow(API_KEY, HOST_ADDRESS)

        # Step 1: Create a new dataset
        ds = rag.create_dataset(name="God")

        # Ensure dataset creation was successful
        assert isinstance(ds, DataSet), f"Failed to create dataset, error: {ds}"
        assert ds.name == "God", "Dataset name does not match."

        # Step 2: Create a new document
        # The blob is the actual file content or a placeholder in this case
        name = "TestDocument.txt"
        blob = b"Sample document content for ingestion test."

        res = rag.create_document(ds, name=name, blob=blob)

        # Ensure document ingestion was successful
        assert res is True, f"Failed to create document, error: {res}"

    def test_get_detail_document_with_success(self):
        """
        Test getting a document's detail with success
        """
        rag = RAGFlow(API_KEY, HOST_ADDRESS)
        doc = rag.get_document(name="TestDocument.txt")
        assert isinstance(doc, Document), f"Failed to get dataset, error: {doc}."
        assert doc.name == "TestDocument.txt", "Name does not match"

    def test_update_document_with_success(self):
        """
        Test updating a document with success.
        """
        rag = RAGFlow(API_KEY, HOST_ADDRESS)
        doc = rag.get_document(name="TestDocument.txt")
        if isinstance(doc, Document):
            doc.parser_method = "manual"
            doc.name = "manual.txt"
            res = doc.save()
            assert res is True, f"Failed to update document, error: {res}"
        else:
            assert False, f"Failed to get document, error: {doc}"

    def test_download_document_with_success(self):
        """
        Test downloading a document with success.
        """
        # Initialize RAGFlow instance
        rag = RAGFlow(API_KEY, HOST_ADDRESS)

        # Retrieve a document
        doc = rag.get_document(name="TestDocument.txt")

        # Check if the retrieved document is of type Document
        if isinstance(doc, Document):
            # Download the document content and save it to a file
            try:
                with open("ragflow.txt", "wb+") as file:
                    file.write(doc.download())
                    # Print the document object for debugging
                print(doc)

                # Assert that the download was successful
                assert True, "Document downloaded successfully."
            except Exception as e:
                # If an error occurs, raise an assertion error
                assert False, f"Failed to download document, error: {str(e)}"
        else:
            # If the document retrieval fails, assert failure
            assert False, f"Failed to get document, error: {doc}"

    def test_list_all_documents_in_dataset_with_success(self):
        """
        Test list all documents into a dataset with success.
        """
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

        rag.create_document(ds, name=name1, blob=blob1)
        rag.create_document(ds, name=name2, blob=blob2)
        for d in ds.list_docs(keywords="test", offset=0, limit=12):
            assert isinstance(d, Document)
            print(d)

    def test_delete_documents_in_dataset_with_success(self):
        """
        Test list all documents into a dataset with success.
        """
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
        name3 = 'test.txt'
        path = 'test_data/test.txt'
        rag.create_document(ds, name=name3, blob=open(path, "rb").read())
        rag.create_document(ds, name=name1, blob=blob1)
        rag.create_document(ds, name=name2, blob=blob2)
        for d in ds.list_docs(keywords="document", offset=0, limit=12):
            assert isinstance(d, Document)
            d.delete()
            print(d)
        remaining_docs = ds.list_docs(keywords="rag", offset=0, limit=12)
        assert len(remaining_docs) == 0, "Documents were not properly deleted."

    def test_parse_and_cancel_document(self):
        # Initialize RAGFlow with API key and host address
        rag = RAGFlow(API_KEY, HOST_ADDRESS)

        # Create a dataset with a specific name
        ds = rag.create_dataset(name="God4")

        # Define the document name and path
        name3 = 'ai.pdf'
        path = 'test_data/ai.pdf'

        # Create a document in the dataset using the file path
        rag.create_document(ds, name=name3, blob=open(path, "rb").read())

        # Retrieve the document by name
        doc = rag.get_document(name="ai.pdf")

        # Initiate asynchronous parsing
        doc.async_parse()

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

    def test_bulk_parse_and_cancel_documents(self):
        # Initialize RAGFlow with API key and host address
        rag = RAGFlow(API_KEY, HOST_ADDRESS)

        # Create a dataset
        ds = rag.create_dataset(name="God5")
        assert ds is not None, "Dataset creation failed"
        assert ds.name == "God5", "Dataset name does not match"

        # Prepare a list of file names and paths
        documents = [
            {'name': 'ai1.pdf', 'path': 'test_data/ai1.pdf'},
            {'name': 'ai2.pdf', 'path': 'test_data/ai2.pdf'},
            {'name': 'ai3.pdf', 'path': 'test_data/ai3.pdf'}
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

    def test_parse_document_and_chunk_list(self):
        rag = RAGFlow(API_KEY, HOST_ADDRESS)
        ds = rag.create_dataset(name="God7")
        name='story.txt'
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
    def test_add_chunk_to_chunk_list(self):
        rag = RAGFlow(API_KEY, HOST_ADDRESS)
        doc = rag.get_document(name='story.txt')
        chunk = doc.add_chunk(content="assss")
        assert chunk is not None, "Chunk is None"
        assert isinstance(chunk, Chunk), "Chunk was not added to chunk list"

    def test_delete_chunk_of_chunk_list(self):
        rag = RAGFlow(API_KEY, HOST_ADDRESS)
        doc = rag.get_document(name='story.txt')

        chunk = doc.add_chunk(content="assss")
        assert chunk is not None, "Chunk is None"
        assert isinstance(chunk, Chunk), "Chunk was not added to chunk list"
        chunk_num_before=doc.chunk_num
        chunk.delete()
        assert doc.chunk_num == chunk_num_before-1, "Chunk was not deleted"


