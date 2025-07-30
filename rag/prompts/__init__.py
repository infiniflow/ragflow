from . import prompts

__all__ = [name for name in dir(prompts)
           if not name.startswith('_')]

globals().update({name: getattr(prompts, name) for name in __all__})