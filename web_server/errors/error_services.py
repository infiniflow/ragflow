from web_server.errors import FateFlowError

__all__ = ['ServicesError', 'ServiceNotSupported', 'ZooKeeperNotConfigured',
           'MissingZooKeeperUsernameOrPassword', 'ZooKeeperBackendError']


class ServicesError(FateFlowError):
    message = 'Unknown services error'


class ServiceNotSupported(ServicesError):
    message = 'The service {service_name} is not supported'

