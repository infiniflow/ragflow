import xxhash

from rag.svr.parent_chunk import parent_chunk_id


def test_parent_chunk_id_is_document_scoped_and_matches_wire_format():
    mom = "same parent text"

    first = parent_chunk_id("doc-a", mom)
    second = parent_chunk_id("doc-b", mom)

    assert first != second
    assert first == xxhash.xxh64(b"doc-a\x00same parent text").hexdigest()
