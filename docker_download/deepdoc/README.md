---
license: apache-2.0
---

### Model Loading
```python
import xgboost as xgb
import torch

model = xgb.Booster()
if torch.cuda.is_available():
model.set_param({"device": "cuda"})
model.load_model('InfiniFlow/text_concat_xgb_v1.0')
```

### Prediction
```python
model.predict(xgb.DMatrix([feature]))[0]
```