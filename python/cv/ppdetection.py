from paddle.inference import create_predictor
from paddle.inference import Config
from ppdet.infer import Detector
from ppdet.visualize import visualize_box_mask
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
