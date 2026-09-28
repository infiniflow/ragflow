from pathlib import Path

from ruamel.yaml import YAML


def test_go_compose_bootstraps_admin_for_cpu_and_gpu_profiles():
    compose_path = Path(__file__).parents[3] / "docker" / "docker-compose-go.yml"
    compose = YAML(typ="safe").load(compose_path.read_text(encoding="utf-8"))

    for service_name in ("ragflow-cpu", "ragflow-gpu"):
        command = compose["services"][service_name]["command"]
        assert "--enable-adminserver" in command
        assert "--init-superuser" in command
