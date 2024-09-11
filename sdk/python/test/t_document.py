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
        # 初始化 RAGFlow 实例
        rag = RAGFlow(API_KEY, HOST_ADDRESS)

        # 获取文档
        doc = rag.get_document(name="TestDocument.txt")

        # 判断获取的文档是否为 Document 类型
        if isinstance(doc, Document):
            # 下载文档内容并保存到文件
            try:
                with open("ragflow.txt", "wb+") as file:
                    file.write(doc.download())
                # 打印文档对象供调试
                print(doc)

                # 断言下载成功
                assert True, "Document downloaded successfully."
            except Exception as e:
                # 如果发生错误则抛出异常
                assert False, f"Failed to download document, error: {str(e)}"
        else:
            # 如果获取文档失败，则断言失败
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
        path='document 111.txt'
        rag.create_document(ds, name=path, blob=open(path, "rb").read())
        rag.create_document(ds, name=name1, blob=blob1)
        rag.create_document(ds, name=name2, blob=blob2)
        for d in ds.list_docs(keywords="document", offset=0, limit=12):
            assert isinstance(d, Document)
            d.delete()
            print(d)
        remaining_docs = ds.list_docs(keywords="rag", offset=0, limit=12)
        assert len(remaining_docs) == 0, "Documents were not properly deleted."

    def test_parse_document_with_success(self):
        """
        Test parse a document with success.
        """
        rag = RAGFlow(API_KEY, HOST_ADDRESS)
        doc = rag.get_document(name="TestDocument.txt")
        # Start parsing the document (synchronous parsing)
        for progress, msg in doc.parse(interval=15, timeout=30):
            print(f"Progress: {progress}, Message: {msg}")
            # Assert that progress is within the correct range
            assert 0 <= progress <= 100, f"Unexpected progress value: {progress}"
            # Check that a message is returned
            assert msg is not None, "Message during parse should not be None"

        # Ensure that the document parsing is completed
        assert progress == 100, "Document parsing did not complete to 100%."

        # Start async parsing (asynchronous parsing)
        doc.async_parse()

        # Wait for parsing to finish using the join method
        for progress, msg in doc.join(interval=15, timeout=30):
            print(f"Async Progress: {progress}, Message: {msg}")
            assert 0 <= progress <= 100, f"Unexpected async progress value: {progress}"
            assert msg is not None, "Message during async parse should not be None"

        # Ensure async parsing is complete
        assert progress == 100, "Asynchronous document parsing did not complete to 100%."

        # Step 6: Cancel the parsing (if applicable)
        cancel_result = doc.cancel()

        # Ensure cancellation was successful
        assert cancel_result is True, "Document parsing cancellation failed."



