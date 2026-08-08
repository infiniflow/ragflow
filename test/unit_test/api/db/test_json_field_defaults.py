from api.db.db_models import JSONField, ListField


def test_json_field_empty_values_do_not_share_default_dict():
    field = JSONField()
    field.default_value = {"nested": {"items": [1]}}

    first = field.python_value(None)
    first["nested"]["items"].append(2)
    second = field.python_value(None)

    assert second == {"nested": {"items": [1]}}
    assert second is not first
    assert second["nested"] is not first["nested"]


def test_list_field_empty_values_do_not_share_default_list():
    field = ListField()
    field.default_value = [{"nested": [1]}]

    first = field.python_value("")
    first[0]["nested"].append(2)
    second = field.python_value("")

    assert second == [{"nested": [1]}]
    assert second is not first
    assert second[0] is not first[0]
