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
import onnxruntime as ort
from huggingface_hub import snapshot_download

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
