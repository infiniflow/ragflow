#
#  Copyright 2019 The InfiniFlow Authors. All Rights Reserved.
#
#  Licensed under the Apache License, Version 2.0 (the "License");
#  you may not use this file except in compliance with the License.
#  You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
#  Unless required by applicable law or agreed to in writing, software
#  distributed under the License is distributed on an "AS IS" BASIS,
#  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#  See the License for the specific language governing permissions and
#  limitations under the License.
#
from rag.ppdet.infer import Detector
import os
import yaml
import logging
from huggingface_hub import snapshot_download


class PPDet:
    def __init__(self):
        model_dir = snapshot_download(
            repo_id="InfiniFlow/picodet_lcnet_x1_0_fgd_layout_cdla")
        self.mdl = Detector(model_dir)
        with open(os.path.join(model_dir, 'infer_cfg.yml'), "r") as f:
            self.conf = yaml.safe_load(f)

    def __call__(self, image_list: list, thr=0.7):
        result = self.mdl.predict_image(image_list, visual=False)
        cate = self.conf['label_list']
        start_idx = 0
        res = []
        for idx, img in enumerate(image_list):
            im_bboxes_num = result['boxes_num'][idx]
            bb = []
            for b in result["boxes"][start_idx:start_idx + im_bboxes_num]:
                clsid, bbox, score = int(b[0]), b[2:], b[1]
                if score < thr:
                    continue
                if clsid >= len(cate):
                    logging.warning(f"bad category id")
                    continue
                bb.append({
                    "type": cate[clsid].lower(),
                    "bbox": bbox,
                    "score": score
                })

            res.append(bb)
            start_idx += im_bboxes_num

        return res
