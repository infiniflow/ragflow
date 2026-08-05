from api.db.db_models import JSONField, ListField


def test_json_field_empty_values_do_not_share_default_dict():
    field = JSONField()

    first = field.python_value(None)
    first["changed"] = True
    second = field.python_value(None)

    assert second == {}
    assert second is not first


def test_list_field_empty_values_do_not_share_default_list():
    field = ListField()

    first = field.python_value("")
    first.append("changed")
    second = field.python_value("")

    assert second == []
    assert second is not first
