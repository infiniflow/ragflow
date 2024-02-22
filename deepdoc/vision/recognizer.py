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

import os
from copy import deepcopy

import onnxruntime as ort
from huggingface_hub import snapshot_download

from . import seeit
from .operators import *
from rag.settings import cron_logger


class Recognizer(object):
    def __init__(self, label_list, task_name, model_dir=None):
        """
        If you have trouble downloading HuggingFace models, -_^ this might help!!

        For Linux:
        export HF_ENDPOINT=https://hf-mirror.com

        For Windows:
        Good luck
        ^_-

        """
        if not model_dir:
            model_dir = snapshot_download(repo_id="InfiniFlow/ocr")

        model_file_path = os.path.join(model_dir, task_name + ".onnx")
        if not os.path.exists(model_file_path):
            raise ValueError("not find model file path {}".format(
                model_file_path))
        if ort.get_device() == "GPU":
            self.ort_sess = ort.InferenceSession(model_file_path, providers=['CUDAExecutionProvider'])
        else:
            self.ort_sess = ort.InferenceSession(model_file_path, providers=['CPUExecutionProvider'])
        self.label_list = label_list

    @staticmethod
    def sort_Y_firstly(arr, threashold):
        # sort using y1 first and then x1
        arr = sorted(arr, key=lambda r: (r["top"], r["x0"]))
        for i in range(len(arr) - 1):
            for j in range(i, -1, -1):
                # restore the order using th
                if abs(arr[j + 1]["top"] - arr[j]["top"]) < threashold \
                        and arr[j + 1]["x0"] < arr[j]["x0"]:
                    tmp = deepcopy(arr[j])
                    arr[j] = deepcopy(arr[j + 1])
                    arr[j + 1] = deepcopy(tmp)
        return arr

    @staticmethod
    def sort_X_firstly(arr, threashold, copy=True):
        # sort using y1 first and then x1
        arr = sorted(arr, key=lambda r: (r["x0"], r["top"]))
        for i in range(len(arr) - 1):
            for j in range(i, -1, -1):
                # restore the order using th
                if abs(arr[j + 1]["x0"] - arr[j]["x0"]) < threashold \
                        and arr[j + 1]["top"] < arr[j]["top"]:
                    tmp = deepcopy(arr[j]) if copy else arr[j]
                    arr[j] = deepcopy(arr[j + 1]) if copy else arr[j + 1]
                    arr[j + 1] = deepcopy(tmp) if copy else tmp
        return arr

    @staticmethod
    def sort_C_firstly(arr, thr=0):
        # sort using y1 first and then x1
        # sorted(arr, key=lambda r: (r["x0"], r["top"]))
        arr = Recognizer.sort_X_firstly(arr, thr)
        for i in range(len(arr) - 1):
            for j in range(i, -1, -1):
                # restore the order using th
                if "C" not in arr[j] or "C" not in arr[j + 1]:
                    continue
                if arr[j + 1]["C"] < arr[j]["C"] \
                        or (
                        arr[j + 1]["C"] == arr[j]["C"]
                        and arr[j + 1]["top"] < arr[j]["top"]
                ):
                    tmp = arr[j]
                    arr[j] = arr[j + 1]
                    arr[j + 1] = tmp
        return arr

        return sorted(arr, key=lambda r: (r.get("C", r["x0"]), r["top"]))

    @staticmethod
    def sort_R_firstly(arr, thr=0):
        # sort using y1 first and then x1
        # sorted(arr, key=lambda r: (r["top"], r["x0"]))
        arr = Recognizer.sort_Y_firstly(arr, thr)
        for i in range(len(arr) - 1):
            for j in range(i, -1, -1):
                if "R" not in arr[j] or "R" not in arr[j + 1]:
                    continue
                if arr[j + 1]["R"] < arr[j]["R"] \
                        or (
                        arr[j + 1]["R"] == arr[j]["R"]
                        and arr[j + 1]["x0"] < arr[j]["x0"]
                ):
                    tmp = arr[j]
                    arr[j] = arr[j + 1]
                    arr[j + 1] = tmp
        return arr

    @staticmethod
    def overlapped_area(a, b, ratio=True):
        tp, btm, x0, x1 = a["top"], a["bottom"], a["x0"], a["x1"]
        if b["x0"] > x1 or b["x1"] < x0:
            return 0
        if b["bottom"] < tp or b["top"] > btm:
            return 0
        x0_ = max(b["x0"], x0)
        x1_ = min(b["x1"], x1)
        assert x0_ <= x1_, "Fuckedup! T:{},B:{},X0:{},X1:{} ==> {}".format(
            tp, btm, x0, x1, b)
        tp_ = max(b["top"], tp)
        btm_ = min(b["bottom"], btm)
        assert tp_ <= btm_, "Fuckedup! T:{},B:{},X0:{},X1:{} => {}".format(
            tp, btm, x0, x1, b)
        ov = (btm_ - tp_) * (x1_ - x0_) if x1 - \
                                           x0 != 0 and btm - tp != 0 else 0
        if ov > 0 and ratio:
            ov /= (x1 - x0) * (btm - tp)
        return ov

    @staticmethod
    def layouts_cleanup(boxes, layouts, far=2, thr=0.7):
        def notOverlapped(a, b):
            return any([a["x1"] < b["x0"],
                        a["x0"] > b["x1"],
                        a["bottom"] < b["top"],
                        a["top"] > b["bottom"]])

        i = 0
        while i + 1 < len(layouts):
            j = i + 1
            while j < min(i + far, len(layouts)) \
                    and (layouts[i].get("type", "") != layouts[j].get("type", "")
                         or notOverlapped(layouts[i], layouts[j])):
                j += 1
            if j >= min(i + far, len(layouts)):
                i += 1
                continue
            if Recognizer.overlapped_area(layouts[i], layouts[j]) < thr \
                    and Recognizer.overlapped_area(layouts[j], layouts[i]) < thr:
                i += 1
                continue

            if layouts[i].get("score") and layouts[j].get("score"):
                if layouts[i]["score"] > layouts[j]["score"]:
                    layouts.pop(j)
                else:
                    layouts.pop(i)
                continue

            area_i, area_i_1 = 0, 0
            for b in boxes:
                if not notOverlapped(b, layouts[i]):
                    area_i += Recognizer.overlapped_area(b, layouts[i], False)
                if not notOverlapped(b, layouts[j]):
                    area_i_1 += Recognizer.overlapped_area(b, layouts[j], False)

            if area_i > area_i_1:
                layouts.pop(j)
            else:
                layouts.pop(i)

        return layouts

    def create_inputs(self, imgs, im_info):
        """generate input for different model type
        Args:
            imgs (list(numpy)): list of images (np.ndarray)
            im_info (list(dict)): list of image info
        Returns:
            inputs (dict): input of model
        """
        inputs = {}

        im_shape = []
        scale_factor = []
        if len(imgs) == 1:
            inputs['image'] = np.array((imgs[0],)).astype('float32')
            inputs['im_shape'] = np.array(
                (im_info[0]['im_shape'],)).astype('float32')
            inputs['scale_factor'] = np.array(
                (im_info[0]['scale_factor'],)).astype('float32')
            return inputs

        for e in im_info:
            im_shape.append(np.array((e['im_shape'],)).astype('float32'))
            scale_factor.append(np.array((e['scale_factor'],)).astype('float32'))

        inputs['im_shape'] = np.concatenate(im_shape, axis=0)
        inputs['scale_factor'] = np.concatenate(scale_factor, axis=0)

        imgs_shape = [[e.shape[1], e.shape[2]] for e in imgs]
        max_shape_h = max([e[0] for e in imgs_shape])
        max_shape_w = max([e[1] for e in imgs_shape])
        padding_imgs = []
        for img in imgs:
            im_c, im_h, im_w = img.shape[:]
            padding_im = np.zeros(
                (im_c, max_shape_h, max_shape_w), dtype=np.float32)
            padding_im[:, :im_h, :im_w] = img
            padding_imgs.append(padding_im)
        inputs['image'] = np.stack(padding_imgs, axis=0)
        return inputs

    @staticmethod
    def find_overlapped(self, box, boxes_sorted_by_y, naive=False):
        if not boxes_sorted_by_y:
            return
        bxs = boxes_sorted_by_y
        s, e, ii = 0, len(bxs), 0
        while s < e and not naive:
            ii = (e + s) // 2
            pv = bxs[ii]
            if box["bottom"] < pv["top"]:
                e = ii
                continue
            if box["top"] > pv["bottom"]:
                s = ii + 1
                continue
            break
        while s < ii:
            if box["top"] > bxs[s]["bottom"]:
                s += 1
            break
        while e - 1 > ii:
            if box["bottom"] < bxs[e - 1]["top"]:
                e -= 1
            break

        max_overlaped_i, max_overlaped = None, 0
        for i in range(s, e):
            ov = self.__overlapped_area(bxs[i], box)
            if ov <= max_overlaped:
                continue
            max_overlaped_i = i
            max_overlaped = ov

        return max_overlaped_i

    @staticmethod
    def find_overlapped_with_threashold(box, boxes, thr=0.3):
        if not boxes:
            return
        max_overlaped_i, max_overlaped, _max_overlaped = None, thr, 0
        s, e = 0, len(boxes)
        for i in range(s, e):
            ov = Recognizer.overlapped_area(box, boxes[i])
            _ov = Recognizer.overlapped_area(boxes[i], box)
            if (ov, _ov) < (max_overlaped, _max_overlaped):
                continue
            max_overlaped_i = i
            max_overlaped = ov
            _max_overlaped = _ov

        return max_overlaped_i

    def preprocess(self, image_list):
        preprocess_ops = []
        for op_info in [
            {'interp': 2, 'keep_ratio': False, 'target_size': [800, 608], 'type': 'LinearResize'},
            {'is_scale': True, 'mean': [0.485, 0.456, 0.406], 'std': [0.229, 0.224, 0.225], 'type': 'StandardizeImage'},
            {'type': 'Permute'},
            {'stride': 32, 'type': 'PadStride'}
        ]:
            new_op_info = op_info.copy()
            op_type = new_op_info.pop('type')
            preprocess_ops.append(eval(op_type)(**new_op_info))

        inputs = []
        for im_path in image_list:
            im, im_info = preprocess(im_path, preprocess_ops)
            inputs.append({"image": np.array((im,)).astype('float32'), "scale_factor": np.array((im_info["scale_factor"],)).astype('float32')})
        return inputs

    def __call__(self, image_list, thr=0.7, batch_size=16):
        res = []
        imgs = []
        for i in range(len(image_list)):
            if not isinstance(image_list[i], np.ndarray):
                imgs.append(np.array(image_list[i]))
            else: imgs.append(image_list[i])

        batch_loop_cnt = math.ceil(float(len(imgs)) / batch_size)
        for i in range(batch_loop_cnt):
            start_index = i * batch_size
            end_index = min((i + 1) * batch_size, len(imgs))
            batch_image_list = imgs[start_index:end_index]
            inputs = self.preprocess(batch_image_list)
            for ins in inputs:
                bb = []
                for b in self.ort_sess.run(None, ins)[0]:
                    clsid, bbox, score = int(b[0]), b[2:], b[1]
                    if score < thr:
                        continue
                    if clsid >= len(self.label_list):
                        cron_logger.warning(f"bad category id")
                        continue
                    bb.append({
                        "type": self.label_list[clsid].lower(),
                        "bbox": [float(t) for t in bbox.tolist()],
                        "score": float(score)
                    })
                res.append(bb)

        #seeit.save_results(image_list, res, self.label_list, threshold=thr)

        return res
