import requests

from web_server.db.service_registry import ServiceRegistry
from web_server.settings import RegistryServiceName
from web_server.hook import HookManager
from web_server.hook.common.parameters import PermissionCheckParameters, PermissionReturn
from web_server.settings import HOOK_SERVER_NAME


@HookManager.register_permission_check_hook
def permission(parm: PermissionCheckParameters) -> PermissionReturn:
    service_list = ServiceRegistry.load_service(server_name=HOOK_SERVER_NAME, service_name=RegistryServiceName.PERMISSION_CHECK.value)
    if not service_list:
        raise Exception(f"permission check error: no found server {HOOK_SERVER_NAME} service permission")
    service = service_list[0]
    response = getattr(requests, service.f_method.lower(), None)(
        url=service.f_url,
        json=parm.to_dict()
    )
    if response.status_code != 200:
        raise Exception(
            f"permission check error: request permission url failed, status code {response.status_code}")
    elif response.json().get("code") != 0:
        return PermissionReturn(code=response.json().get("code"), message=response.json().get("msg"))
    return PermissionReturn()
