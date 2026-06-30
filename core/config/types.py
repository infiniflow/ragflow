from enum import Enum


class DatabaseType(str, Enum):
    MYSQL = "mysql"
    POSTGRES = "postgres"
    OCEANBASE = "oceanbase"


class DocumentEngineType(str, Enum):
    ELASTICSEARCH = "elasticsearch"
    OPENSEARCH = "opensearch"
    INFINITY = "infinity"


class ObjectStorageType(str, Enum):
    MINIO = "minio"
    S3 = "s3"
    GCS = "gcs"
    OSS = "oss"
    AZURE_SAS = "azure_sas"
    AZURE_SPN = "azure_spn"
    OPENDAL = "opendal"


class CacheType(str, Enum):
    REDIS = "redis"
