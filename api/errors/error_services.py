from api.errors import RagFlowError

__all__ = ['ServicesError', 'ServiceNotSupported', 'ZooKeeperNotConfigured',
           'MissingZooKeeperUsernameOrPassword', 'ZooKeeperBackendError']


class ServicesError(RagFlowError):
    message = 'Unknown services error'


class ServiceNotSupported(ServicesError):
    message = 'The service {service_name} is not supported'

