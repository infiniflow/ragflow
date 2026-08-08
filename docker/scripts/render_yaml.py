import os
import re
import argparse
from pathlib import Path
from typing import Any

from ruamel.yaml import YAML

yaml = YAML(typ="safe", pure=True)
yaml.default_flow_style = False  # Block-style YAML output
yaml.indent(mapping=2, sequence=4, offset=2)

# Regex to match shell-style variables: ${VAR} or ${VAR:-default}
ENV_PATTERN = re.compile(r'\${([^}:]+)(:-([^}]*))?}')


def replace_env(value: Any) -> Any:
    """
    Replace environment variables in a string.

    All replaced values are treated as strings.

    Args:
        value: Input string possibly containing ${VAR} or ${VAR:-default}.

    Returns:
        String with environment variables replaced.
    """
    if not isinstance(value, str):
        return value

    def repl(match):
        var_name = match.group(1)
        default_value = match.group(3) or ''
        default_value = default_value.strip('\'"')  # Remove surrounding quotes from default
        env_val = os.environ.get(var_name)
        return env_val if env_val not in (None, '') else default_value

    return ENV_PATTERN.sub(repl, value)


def render_node(node):
    """
    Recursively render a YAML node.

    Supports dict, list, and scalar values.

    Args:
        node: YAML node (dict, list, or scalar)

    Returns:
        Rendered node with environment variables replaced.
    """
    if isinstance(node, dict):
        return {k: render_node(v) for k, v in node.items()}
    elif isinstance(node, list):
        return [render_node(v) for v in node]
    else:
        return replace_env(node)


def render_file(template_path: Path, output_path: Path):
    """
    Render a YAML template file and write the output.

    Args:
        template_path: Path to input YAML template
        output_path: Path to write rendered YAML
    """
    try:
        with open(template_path, 'r', encoding='utf-8') as f:
            data = yaml.load(f)
    except FileNotFoundError:
        raise RuntimeError(f"Template file {template_path} not found")
    except Exception as e:
        raise RuntimeError(f"YAML parsing error in {template_path}: {e}")

    rendered_data = render_node(data)

    with open(output_path, 'w', encoding='utf-8') as f:
        yaml.dump(rendered_data, f)


def main():
    parser = argparse.ArgumentParser(description="Render a YAML template using environment variables")
    parser.add_argument("template_file", type=Path, help="Path to the template YAML file")
    parser.add_argument("output_file", type=Path, help="Path to write the rendered YAML file")
    args = parser.parse_args()

    render_file(args.template_file, args.output_file)


if __name__ == "__main__":
    main()
