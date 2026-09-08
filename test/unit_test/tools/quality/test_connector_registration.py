"""Isolated exact registration contract for the owned connector HTTP surface."""

import importlib.util
from pathlib import Path

import pytest


ROOT = Path(__file__).resolve().parents[4]
FIXTURE_PATH = ROOT / "test/testcases/restful_api/test_connector_routes_unit.py"
SPEC = importlib.util.spec_from_file_location("connector_route_contract_fixture", FIXTURE_PATH)
ROUTE_FIXTURE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(ROUTE_FIXTURE)


def _assert_exact_inventory(module):
    assert len(module.manager.routes) == len(ROUTE_FIXTURE.EXPECTED_CONNECTOR_ROUTES)
    assert set(module.manager.routes) == ROUTE_FIXTURE.EXPECTED_CONNECTOR_ROUTES


def test_connector_route_inventory_is_exact(monkeypatch):
    _assert_exact_inventory(ROUTE_FIXTURE._load_connector_app(monkeypatch))


def test_connector_route_inventory_rejects_changed_registration(monkeypatch):
    module = ROUTE_FIXTURE._load_connector_app(monkeypatch)
    module.manager.routes[-1] = ("/connectors/unexpected", ("POST",), "unexpected")

    with pytest.raises(AssertionError):
        _assert_exact_inventory(module)
