from ragflow import RAGFlow, DataSet, Document

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
        name3='test.txt'
        path='test_data/test.txt'
        rag.create_document(ds, name=name3, blob=open(path, "rb").read())
        rag.create_document(ds, name=name1, blob=blob1)
        rag.create_document(ds, name=name2, blob=blob2)
        for d in ds.list_docs(keywords="document", offset=0, limit=12):
            assert isinstance(d, Document)
            d.delete()
            print(d)
        remaining_docs = ds.list_docs(keywords="rag", offset=0, limit=12)
        assert len(remaining_docs) == 0, "Documents were not properly deleted."





