from types import SimpleNamespace


def test_validate_agent_target_rejects_inaccessible_agent():
    from api.channels.targets import validate_agent_target

    class FakeUserCanvasService:
        @staticmethod
        def get_by_id(_agent_id):
            return True, SimpleNamespace(canvas_category="agent_canvas")

        @staticmethod
        def accessible(_agent_id, _tenant_id):
            return False

    assert validate_agent_target("agent-1", "tenant-1", service=FakeUserCanvasService) == "forbidden"


def test_validate_agent_target_rejects_non_agent_canvas():
    from api.channels.targets import validate_agent_target

    class FakeUserCanvasService:
        @staticmethod
        def get_by_id(_agent_id):
            return True, SimpleNamespace(canvas_category="pipeline")

    assert validate_agent_target("canvas-1", "tenant-1", service=FakeUserCanvasService) == "not_found"


def test_validate_agent_target_accepts_accessible_agent():
    from api.channels.targets import validate_agent_target

    class FakeUserCanvasService:
        @staticmethod
        def get_by_id(_agent_id):
            return True, SimpleNamespace(canvas_category="agent_canvas")

        @staticmethod
        def accessible(_agent_id, _tenant_id):
            return True

    assert validate_agent_target("agent-1", "tenant-1", service=FakeUserCanvasService) is None
