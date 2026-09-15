import ast
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import Mock

SERVER_PATH = Path(__file__).resolve().parents[3] / "api" / "ragflow_server.py"


def test_signal_handler_forces_exit_on_second_signal():
    tree = ast.parse(SERVER_PATH.read_text(encoding="utf-8"), filename=str(SERVER_PATH))
    handler = next(node for node in tree.body if isinstance(node, ast.FunctionDef) and node.name == "signal_handler")
    namespace = {
        "shutdown_requested": False,
        "os": SimpleNamespace(getpid=Mock(return_value=1234), kill=Mock()),
        "signal": SimpleNamespace(SIGKILL=9),
        "sys": SimpleNamespace(exit=Mock(side_effect=SystemExit(0))),
    }
    exec(compile(ast.fix_missing_locations(ast.Module(body=[handler], type_ignores=[])), str(SERVER_PATH), "exec"), namespace)

    try:
        namespace["signal_handler"](2, None)
    except SystemExit as exc:
        assert exc.code == 0
    else:
        raise AssertionError("the first shutdown signal must request graceful process exit")

    assert namespace["shutdown_requested"] is True
    namespace["os"].kill.assert_not_called()

    namespace["signal_handler"](15, None)

    namespace["os"].getpid.assert_called_once_with()
    namespace["os"].kill.assert_called_once_with(1234, 9)
    namespace["sys"].exit.assert_called_once_with(0)


def test_server_shutdown_wraps_full_startup_and_forces_cleanup_before_kill():
    tree = ast.parse(SERVER_PATH.read_text(encoding="utf-8"), filename=str(SERVER_PATH))
    main = next(node for node in tree.body if isinstance(node, ast.FunctionDef) and node.name == "main")
    main_try = next(node for node in main.body if isinstance(node, ast.Try))

    startup_calls = [ast.unparse(statement.value) for statement in main_try.body]
    assert startup_calls == [
        "signal.signal(signal.SIGINT, signal_handler)",
        "signal.signal(signal.SIGTERM, signal_handler)",
        "run_server()",
    ]

    exception_handler = main_try.handlers[0]
    assert ast.unparse(exception_handler.body[0]) == "force_kill = True"

    mcp_cleanup = next(node for node in main_try.finalbody if isinstance(node, ast.Try))
    assert ast.unparse(mcp_cleanup.body[0].value) == "shutdown_all_mcp_sessions()"

    background_cleanup = mcp_cleanup.finalbody[0]
    assert isinstance(background_cleanup, ast.Try)
    assert ast.unparse(background_cleanup.body[0].value) == "stop_background_services()"

    force_kill = background_cleanup.finalbody[0]
    assert isinstance(force_kill, ast.If)
    assert ast.unparse(force_kill.test) == "force_kill"
    assert ast.unparse(force_kill.body[0].value) == "os.kill(os.getpid(), signal.SIGKILL)"
