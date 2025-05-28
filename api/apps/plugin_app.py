from flask import Response
from flask_login import login_required
from api.utils.api_utils import get_json_result
from plugin import GlobalPluginManager

@manager.route('/llm_tools', methods=['GET'])  # noqa: F821
@login_required
def llm_tools() -> Response:
    tools = GlobalPluginManager.get_llm_tools()
    tools_metadata = [t.get_metadata() for t in tools]

    return get_json_result(data=tools_metadata)
