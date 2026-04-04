#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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

import copy
import re


# Keep all legacy chunker renames in one place so the migration rule stays readable.
COMPONENT_RENAMES = {
    "Splitter": "TokenChunker",
    "HierarchicalMerger": "TitleChunker",
}

NODE_TYPE_RENAMES = {
    "splitterNode": "chunkerNode",
}

VARIABLE_REF_PATTERN = re.compile(r"(\{+\s*)([A-Za-z0-9:_-]+)(@[A-Za-z0-9_.-]+)(\s*\}+)")


def normalize_chunker_dsl(dsl: dict) -> dict:
    """
    Rewrite legacy chunker component names and ids into the current DSL schema.

    This is intentionally a pure migration step:
    - it does not change business params
    - it only rewrites structural identifiers used by the canvas/runtime
    - custom human-authored names are preserved unless they are still the exact
      built-in legacy operator name
    """
    if not isinstance(dsl, dict):
        return dsl

    normalized = copy.deepcopy(dsl)
    components = normalized.get("components")
    if not isinstance(components, dict):
        return normalized

    component_id_map: dict[str, str] = {}
    for component_id in components.keys():
        new_component_id = component_id
        for old_name, new_name in COMPONENT_RENAMES.items():
            prefix = f"{old_name}:"
            if component_id.startswith(prefix):
                new_component_id = f"{new_name}:{component_id[len(prefix):]}"
                break
        component_id_map[component_id] = new_component_id

    def rewrite_variable_refs(text: str) -> str:
        if text in component_id_map:
            return component_id_map[text]

        def repl(match: re.Match[str]) -> str:
            component_id = match.group(2)
            return (
                match.group(1)
                + component_id_map.get(component_id, component_id)
                + match.group(3)
                + match.group(4)
            )

        return VARIABLE_REF_PATTERN.sub(repl, text)

    def rewrite_value(value):
        if isinstance(value, str):
            return rewrite_variable_refs(value)
        if isinstance(value, list):
            return [rewrite_value(item) for item in value]
        if isinstance(value, dict):
            return {key: rewrite_value(item) for key, item in value.items()}
        return value

    rewritten_components = {}
    for old_component_id, component in components.items():
        new_component_id = component_id_map[old_component_id]
        new_component = rewrite_value(component)

        if isinstance(new_component, dict):
            obj = new_component.get("obj")
            if isinstance(obj, dict):
                component_name = obj.get("component_name")
                obj["component_name"] = COMPONENT_RENAMES.get(component_name, component_name)

            if isinstance(new_component.get("downstream"), list):
                new_component["downstream"] = [
                    component_id_map.get(component_id, component_id)
                    for component_id in new_component["downstream"]
                ]
            if isinstance(new_component.get("upstream"), list):
                new_component["upstream"] = [
                    component_id_map.get(component_id, component_id)
                    for component_id in new_component["upstream"]
                ]

            parent_id = new_component.get("parent_id")
            if isinstance(parent_id, str):
                new_component["parent_id"] = component_id_map.get(parent_id, parent_id)

        rewritten_components[new_component_id] = new_component

    normalized["components"] = rewritten_components

    if isinstance(normalized.get("path"), list):
        normalized["path"] = [
            component_id_map.get(component_id, component_id)
            for component_id in normalized["path"]
        ]

    graph = normalized.get("graph")
    if isinstance(graph, dict):
        nodes = graph.get("nodes")
        if isinstance(nodes, list):
            for node in nodes:
                if not isinstance(node, dict):
                    continue
                node_id = node.get("id")
                if isinstance(node_id, str):
                    node["id"] = component_id_map.get(node_id, node_id)

                parent_id = node.get("parentId")
                if isinstance(parent_id, str):
                    node["parentId"] = component_id_map.get(parent_id, parent_id)

                node_type = node.get("type")
                if isinstance(node_type, str):
                    node["type"] = NODE_TYPE_RENAMES.get(node_type, node_type)

                data = node.get("data")
                if not isinstance(data, dict):
                    continue

                label = data.get("label")
                if isinstance(label, str):
                    data["label"] = COMPONENT_RENAMES.get(label, label)

                name = data.get("name")
                if isinstance(name, str) and name in COMPONENT_RENAMES:
                    data["name"] = COMPONENT_RENAMES[name]

                if "form" in data:
                    data["form"] = rewrite_value(data["form"])

        edges = graph.get("edges")
        if isinstance(edges, list):
            replacements = sorted(component_id_map.items(), key=lambda item: len(item[0]), reverse=True)
            for edge in edges:
                if not isinstance(edge, dict):
                    continue
                for key in ("source", "target"):
                    value = edge.get(key)
                    if isinstance(value, str):
                        edge[key] = component_id_map.get(value, value)

                edge_id = edge.get("id")
                if isinstance(edge_id, str):
                    for old_component_id, new_component_id in replacements:
                        edge_id = edge_id.replace(old_component_id, new_component_id)
                    edge["id"] = edge_id

    for key in ("history", "messages", "reference"):
        if key in normalized:
            normalized[key] = rewrite_value(normalized[key])

    return normalized
