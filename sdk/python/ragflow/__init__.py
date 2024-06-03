import importlib.metadata

__version__ = importlib.metadata.version("ragflow")

'''
from ragflow.common import URI, NetworkAddress, LOCAL_HOST, RAGFLowException
from ragflow.ragflow import RAGFlowConnection
from ragflow.remote_thrift.ragflow import RemoteThriftRAGFlowConnection


def connect(
        uri: URI = LOCAL_HOST
) -> RAGFlowConnection:
    if isinstance(uri, NetworkAddress) and (uri.port == 9090 or uri.port == 23817 or uri.port == 9070):
        return RemoteThriftRAGFlowConnection(uri)
    else:
        raise RAGFLowException(7016, f"Unknown uri: {uri}")
'''
