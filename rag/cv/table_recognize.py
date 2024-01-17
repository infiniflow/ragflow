#
#  Copyright 2019 The RAG Flow Authors. All Rights Reserved.
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
import torch
from transformers import \
    TableTransformerForObjectDetection,\
    AutoImageProcessor
from PIL import ImageDraw
from random import randint


class TableTransformer:
    def __init__(self,
                 rec_mdlnm="microsoft/table-transformer-structure-recognition"):
        """
        If you have trouble downloading HuggingFace models, -_^ this might help!!

        For Linux:
        export HF_ENDPOINT=https://hf-mirror.com

        For Windows:
        Good luck
        ^_-

        """
        self.rec_img_pro = AutoImageProcessor.from_pretrained(rec_mdlnm)
        self.rec_mdl = TableTransformerForObjectDetection.from_pretrained(
            rec_mdlnm)

        if torch.cuda.is_available():
            self.rec_mdl.cuda()
        self.batch_size = 1  # batch_size

    def __friendly(self, batch_res, id2label):
        res = []
        for r in batch_res:
            feas = []
            for score, label, box in zip(r["scores"], r["labels"], r["boxes"]):
                if label.item() == 0:
                    continue
                box = [round(x, 2) for x in box.tolist()]
                feas.append({
                    "type": id2label[label.item()],
                    "score": score.item(),
                    "bbox": box
                })
            res.append(feas)
        return res

    def __draw(self, bres, imgs, id2label):
        for i, (img, r) in enumerate(zip(imgs, bres)):
            draw = ImageDraw.Draw(img, "RGB")
            for score, label, box in zip(r["scores"], r["labels"], r["boxes"]):
                if label.item() == 0:
                    continue
                r = randint(0, 255)
                g = randint(0, 255)
                b = randint(0, 255)
                x0, y0, x1, y1 = box[0], box[1], box[2], box[-1]
                draw.rectangle((x0, y0, x1, y1), outline=(r, g, b), width=1)
                draw.text((x0, y0), id2label[label.item(
                )] + ":{:.2f}".format(score), fill=(r, g, b))
            img.save(f"./t{i}.%d.jpg" % randint(0, 1000))

    def __call__(self, images, threshold=0.8):
        res = []
        for i in range(0, len(images), self.batch_size):
            imgs = images[i: i + self.batch_size]
            inputs = self.rec_img_pro(imgs, return_tensors="pt")
            inputs = {k: inputs[k].to(self.rec_mdl.device)
                      if isinstance(inputs[k], torch.Tensor)
                      else inputs[k] for k in inputs.keys()}
            outputs = self.rec_mdl(**inputs)
            target_sizes = torch.tensor([img.size[::-1] for img in imgs])
            # [scores, labels, boxes}]
            with torch.no_grad():
                bres = self.rec_img_pro.post_process_object_detection(outputs,
                                                                      threshold=threshold,
                                                                      target_sizes=target_sizes)
                #self.__draw(bres, imgs, self.rec_mdl.config.id2label)
                res.extend(self.__friendly(bres, self.rec_mdl.config.id2label))
        return res

    def detect(self, images):
        res = []
        for i in range(0, len(images), self.batch_size):
            imgs = images[i: i + self.batch_size]
            inputs = self.det_img_pro(imgs, return_tensors="pt")
            inputs = {k: inputs[k].to(self.det_mdl.device)
                      if isinstance(inputs[k], torch.Tensor)
                      else inputs[k] for k in inputs.keys()}
            outputs = self.det_mdl(**inputs)
            target_sizes = torch.tensor([img.size[::-1] for img in imgs])
            # [scores, labels, boxes}]
            with torch.no_grad():
                res.extend(self.__friendly(self.det_img_pro.post_process_object_detection(outputs,
                                                                                          threshold=0.9,
                                                                                          target_sizes=target_sizes),
                                           self.det_mdl.config.id2label
                                           ))
        return res
