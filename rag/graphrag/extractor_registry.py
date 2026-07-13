EXTRACTOR_REGISTRY = {}


def register_graphrag_extractor(name):
    def decorator(cls):
        EXTRACTOR_REGISTRY[name] = cls
        return cls

    return decorator


def get_graphrag_extractor(name):
    return EXTRACTOR_REGISTRY.get(name)