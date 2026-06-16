from api.db.services import task_service


class FakeRedis:
    def __init__(self):
        self.keys = {}
        self.decrements = []
        self.deleted = []

    def set_if_absent(self, key, value, exp=3600):
        if key in self.keys:
            return False
        self.keys[key] = value
        return True

    def decrby(self, key, decrement):
        self.decrements.append((key, decrement))
        self.keys[key] = int(self.keys.get(key, 0)) - decrement
        return self.keys[key]

    def delete(self, key):
        self.deleted.append(key)
        self.keys.pop(key, None)
        return True


def test_credit_doc_chunking_task_decrements_once(monkeypatch):
    fake = FakeRedis()
    fake.keys["doc:chunking_pending:doc-1"] = 2
    monkeypatch.setattr(task_service, "REDIS_CONN", fake)

    first_remaining = task_service.credit_doc_chunking_task("doc-1", "task-1")
    second_remaining = task_service.credit_doc_chunking_task("doc-1", "task-1")

    assert first_remaining == 1
    assert second_remaining == 1
    assert fake.decrements == [("doc:chunking_pending:doc-1", 1)]


def test_clear_doc_chunking_counter_deletes_pending_key(monkeypatch):
    fake = FakeRedis()
    fake.keys["doc:chunking_pending:doc-1"] = 3
    monkeypatch.setattr(task_service, "REDIS_CONN", fake)

    task_service.clear_doc_chunking_counter("doc-1")

    assert "doc:chunking_pending:doc-1" not in fake.keys
    assert fake.deleted == ["doc:chunking_pending:doc-1"]
