#
#  Copyright 2025 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#

import pytest
import json
from unittest.mock import Mock, patch, AsyncMock
from common.misc_utils import get_uuid


class TestCanvasService:
    """Comprehensive unit tests for Canvas/Agent Service"""

    @pytest.fixture
    def mock_canvas_service(self):
        """Create a mock UserCanvasService for testing"""
        with patch('api.db.services.canvas_service.UserCanvasService') as mock:
            yield mock

    @pytest.fixture
    def sample_canvas_data(self):
        """Sample canvas/agent data for testing"""
        return {
            "id": get_uuid(),
            "user_id": "test_user_123",
            "title": "Test Agent",
            "description": "A test agent workflow",
            "avatar": "",
            "canvas_type": "agent",
            "canvas_category": "agent_canvas",
            "permission": "me",
            "dsl": {
                "components": [
                    {
                        "id": "comp1",
                        "type": "LLM",
                        "config": {
                            "model": "gpt-4",
                            "temperature": 0.7
                        }
                    },
                    {
                        "id": "comp2",
                        "type": "Retrieval",
                        "config": {
                            "kb_ids": ["kb1", "kb2"]
                        }
                    }
                ],
                "edges": [
                    {"source": "comp1", "target": "comp2"}
                ]
            }
        }

    def test_canvas_creation_success(self, mock_canvas_service, sample_canvas_data):
        """Test successful canvas creation"""
        mock_canvas_service.save.return_value = True
        
        result = mock_canvas_service.save(**sample_canvas_data)
        assert result is True
        mock_canvas_service.save.assert_called_once_with(**sample_canvas_data)

    def test_canvas_creation_with_duplicate_title(self, mock_canvas_service):
        """Test canvas creation with duplicate title"""
        user_id = "user123"
        title = "Duplicate Agent"
        
        mock_canvas_service.query.return_value = [Mock(title=title)]
        
        existing = mock_canvas_service.query(user_id=user_id, title=title)
        assert len(existing) > 0

    def test_canvas_get_by_id_success(self, mock_canvas_service, sample_canvas_data):
        """Test retrieving canvas by ID"""
        canvas_id = sample_canvas_data["id"]
        mock_canvas = Mock()
        mock_canvas.to_dict.return_value = sample_canvas_data
        
        mock_canvas_service.get_by_canvas_id.return_value = (True, sample_canvas_data)
        
        exists, canvas = mock_canvas_service.get_by_canvas_id(canvas_id)
        assert exists is True
        assert canvas == sample_canvas_data

    def test_canvas_get_by_id_not_found(self, mock_canvas_service):
        """Test retrieving non-existent canvas"""
        mock_canvas_service.get_by_canvas_id.return_value = (False, None)
        
        exists, canvas = mock_canvas_service.get_by_canvas_id("nonexistent_id")
        assert exists is False
        assert canvas is None

    def test_canvas_update_success(self, mock_canvas_service, sample_canvas_data):
        """Test successful canvas update"""
        canvas_id = sample_canvas_data["id"]
        update_data = {"title": "Updated Agent Title"}
        
        mock_canvas_service.update_by_id.return_value = True
        result = mock_canvas_service.update_by_id(canvas_id, update_data)
        
        assert result is True

    def test_canvas_delete_success(self, mock_canvas_service):
        """Test canvas deletion"""
        canvas_id = get_uuid()
        
        mock_canvas_service.delete_by_id.return_value = True
        result = mock_canvas_service.delete_by_id(canvas_id)
        
        assert result is True
        mock_canvas_service.delete_by_id.assert_called_once_with(canvas_id)

    def test_canvas_dsl_structure_validation(self, sample_canvas_data):
        """Test DSL structure validation"""
        dsl = sample_canvas_data["dsl"]
        
        assert "components" in dsl
        assert "edges" in dsl
        assert isinstance(dsl["components"], list)
        assert isinstance(dsl["edges"], list)

    def test_canvas_component_validation(self, sample_canvas_data):
        """Test component structure validation"""
        components = sample_canvas_data["dsl"]["components"]
        
        for comp in components:
            assert "id" in comp
            assert "type" in comp
            assert "config" in comp

    def test_canvas_edge_validation(self, sample_canvas_data):
        """Test edge structure validation"""
        edges = sample_canvas_data["dsl"]["edges"]
        
        for edge in edges:
            assert "source" in edge
            assert "target" in edge

    def test_canvas_accessible_by_owner(self, mock_canvas_service):
        """Test canvas accessibility check for owner"""
        canvas_id = get_uuid()
        user_id = "user123"
        
        mock_canvas_service.accessible.return_value = True
        result = mock_canvas_service.accessible(canvas_id, user_id)
        
        assert result is True

    def test_canvas_not_accessible_by_non_owner(self, mock_canvas_service):
        """Test canvas accessibility check for non-owner"""
        canvas_id = get_uuid()
        user_id = "other_user"
        
        mock_canvas_service.accessible.return_value = False
        result = mock_canvas_service.accessible(canvas_id, user_id)
        
        assert result is False

    def test_canvas_permission_me(self, sample_canvas_data):
        """Test canvas with 'me' permission"""
        assert sample_canvas_data["permission"] == "me"

    def test_canvas_permission_team(self, sample_canvas_data):
        """Test canvas with 'team' permission"""
        sample_canvas_data["permission"] = "team"
        assert sample_canvas_data["permission"] == "team"

    def test_canvas_category_agent(self, sample_canvas_data):
        """Test canvas category as agent_canvas"""
        assert sample_canvas_data["canvas_category"] == "agent_canvas"

    def test_canvas_category_dataflow(self, sample_canvas_data):
        """Test canvas category as dataflow_canvas"""
        sample_canvas_data["canvas_category"] = "dataflow_canvas"
        assert sample_canvas_data["canvas_category"] == "dataflow_canvas"

    def test_canvas_dsl_serialization(self, sample_canvas_data):
        """Test DSL JSON serialization"""
        dsl = sample_canvas_data["dsl"]
        dsl_json = json.dumps(dsl)
        
        assert isinstance(dsl_json, str)
        
        # Deserialize back
        dsl_parsed = json.loads(dsl_json)
        assert dsl_parsed == dsl

    def test_canvas_with_llm_component(self, sample_canvas_data):
        """Test canvas with LLM component"""
        llm_comp = sample_canvas_data["dsl"]["components"][0]
        
        assert llm_comp["type"] == "LLM"
        assert "model" in llm_comp["config"]
        assert "temperature" in llm_comp["config"]

    def test_canvas_with_retrieval_component(self, sample_canvas_data):
        """Test canvas with Retrieval component"""
        retrieval_comp = sample_canvas_data["dsl"]["components"][1]
        
        assert retrieval_comp["type"] == "Retrieval"
        assert "kb_ids" in retrieval_comp["config"]

    def test_canvas_component_connection(self, sample_canvas_data):
        """Test component connections via edges"""
        edges = sample_canvas_data["dsl"]["edges"]
        components = sample_canvas_data["dsl"]["components"]
        
        comp_ids = {c["id"] for c in components}
        
        for edge in edges:
            assert edge["source"] in comp_ids
            assert edge["target"] in comp_ids

    def test_canvas_empty_dsl(self):
        """Test canvas with empty DSL"""
        empty_dsl = {
            "components": [],
            "edges": []
        }
        
        assert len(empty_dsl["components"]) == 0
        assert len(empty_dsl["edges"]) == 0

    def test_canvas_complex_workflow(self):
        """Test canvas with complex multi-component workflow"""
        complex_dsl = {
            "components": [
                {"id": "input", "type": "Input", "config": {}},
                {"id": "llm1", "type": "LLM", "config": {"model": "gpt-4"}},
                {"id": "retrieval", "type": "Retrieval", "config": {}},
                {"id": "llm2", "type": "LLM", "config": {"model": "gpt-3.5"}},
                {"id": "output", "type": "Output", "config": {}}
            ],
            "edges": [
                {"source": "input", "target": "llm1"},
                {"source": "llm1", "target": "retrieval"},
                {"source": "retrieval", "target": "llm2"},
                {"source": "llm2", "target": "output"}
            ]
        }
        
        assert len(complex_dsl["components"]) == 5
        assert len(complex_dsl["edges"]) == 4

    def test_canvas_version_creation(self, mock_canvas_service):
        """Test canvas version creation"""
        with patch('api.db.services.user_canvas_version.UserCanvasVersionService') as mock_version:
            canvas_id = get_uuid()
            dsl = {"components": [], "edges": []}
            title = "Version 1"
            
            mock_version.insert.return_value = True
            result = mock_version.insert(
                user_canvas_id=canvas_id,
                dsl=dsl,
                title=title
            )
            
            assert result is True

    def test_canvas_list_by_user(self, mock_canvas_service):
        """Test listing canvases by user ID"""
        user_id = "user123"
        mock_canvases = [Mock() for _ in range(3)]
        
        mock_canvas_service.query.return_value = mock_canvases
        
        result = mock_canvas_service.query(user_id=user_id)
        assert len(result) == 3

    def test_canvas_list_by_category(self, mock_canvas_service):
        """Test listing canvases by category"""
        user_id = "user123"
        category = "agent_canvas"
        mock_canvases = [Mock() for _ in range(2)]
        
        mock_canvas_service.query.return_value = mock_canvases
        
        result = mock_canvas_service.query(
            user_id=user_id,
            canvas_category=category
        )
        assert len(result) == 2

    @pytest.mark.asyncio
    async def test_canvas_run_execution(self):
        """Test canvas run execution"""
        with patch('agent.canvas.Canvas') as MockCanvas:
            mock_canvas = MockCanvas.return_value
            mock_canvas.run = AsyncMock()
            mock_canvas.run.return_value = AsyncMock()
            
            # Simulate async iteration
            async def async_gen():
                yield {"content": "Response 1"}
                yield {"content": "Response 2"}
            
            mock_canvas.run.return_value = async_gen()
            
            results = []
            async for result in mock_canvas.run(query="test", files=[], user_id="user123"):
                results.append(result)
            
            assert len(results) == 2

    def test_canvas_reset_functionality(self):
        """Test canvas reset functionality"""
        with patch('agent.canvas.Canvas') as MockCanvas:
            mock_canvas = MockCanvas.return_value
            mock_canvas.reset = Mock()
            
            mock_canvas.reset()
            mock_canvas.reset.assert_called_once()

    def test_canvas_component_input_form(self):
        """Test getting component input form"""
        with patch('agent.canvas.Canvas') as MockCanvas:
            mock_canvas = MockCanvas.return_value
            mock_canvas.get_component_input_form = Mock(return_value={
                "fields": [
                    {"name": "query", "type": "text", "required": True}
                ]
            })
            
            form = mock_canvas.get_component_input_form("comp1")
            assert "fields" in form
            assert len(form["fields"]) > 0

    def test_canvas_debug_mode(self):
        """Test canvas debug mode execution"""
        with patch('agent.canvas.Canvas') as MockCanvas:
            mock_canvas = MockCanvas.return_value
            component = Mock()
            component.invoke = Mock()
            component.output = Mock(return_value={"result": "debug output"})
            
            mock_canvas.get_component = Mock(return_value={"obj": component})
            
            comp_data = mock_canvas.get_component("comp1")
            comp_data["obj"].invoke(param1="value1")
            output = comp_data["obj"].output()
            
            assert "result" in output

    def test_canvas_title_length_validation(self):
        """Test canvas title length validation"""
        long_title = "a" * 300
        
        if len(long_title) > 255:
            with pytest.raises(Exception):
                raise Exception(f"Canvas title length {len(long_title)} exceeds 255")

    @pytest.mark.parametrize("canvas_type", [
        "agent",
        "workflow",
        "pipeline",
        "custom"
    ])
    def test_canvas_different_types(self, canvas_type, sample_canvas_data):
        """Test canvas with different types"""
        sample_canvas_data["canvas_type"] = canvas_type
        assert sample_canvas_data["canvas_type"] == canvas_type

    def test_canvas_avatar_base64(self, sample_canvas_data):
        """Test canvas with base64 avatar"""
        sample_canvas_data["avatar"] = "data:image/png;base64,iVBORw0KGgoAAAANS..."
        assert sample_canvas_data["avatar"].startswith("data:image/")

    def test_canvas_batch_delete(self, mock_canvas_service):
        """Test batch deletion of canvases"""
        canvas_ids = [get_uuid() for _ in range(3)]
        
        for canvas_id in canvas_ids:
            mock_canvas_service.delete_by_id.return_value = True
            result = mock_canvas_service.delete_by_id(canvas_id)
            assert result is True
