# Copyright (c) 2022 PaddlePaddle Authors. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import os
import copy
import math
import time
import yaml
import cv2
import numpy as np
from collections import defaultdict
import paddle

from benchmark_utils import PaddleInferBenchmark
from utils import gaussian_radius, gaussian2D, draw_umich_gaussian
from preprocess import preprocess, decode_image, WarpAffine, NormalizeImage, Permute
from utils import argsparser, Timer, get_current_memory_mb
from infer import Detector, get_test_images, print_arguments, bench_log, PredictConfig
from keypoint_preprocess import get_affine_transform

# add python path
import sys
parent_path = os.path.abspath(os.path.join(__file__, *(['..'] * 2)))
sys.path.insert(0, parent_path)

from pptracking.python.mot import CenterTracker
from pptracking.python.mot.utils import MOTTimer, write_mot_results
from pptracking.python.mot.visualize import plot_tracking


def transform_preds_with_trans(coords, trans):
    target_coords = np.ones((coords.shape[0], 3), np.float32)
    target_coords[:, :2] = coords
    target_coords = np.dot(trans, target_coords.transpose()).transpose()
    return target_coords[:, :2]


def affine_transform(pt, t):
    new_pt = np.array([pt[0], pt[1], 1.]).T
    new_pt = np.dot(t, new_pt)
    return new_pt[:2]


def affine_transform_bbox(bbox, trans, width, height):
    bbox = np.array(copy.deepcopy(bbox), dtype=np.float32)
    bbox[:2] = affine_transform(bbox[:2], trans)
    bbox[2:] = affine_transform(bbox[2:], trans)
    bbox[[0, 2]] = np.clip(bbox[[0, 2]], 0, width - 1)
    bbox[[1, 3]] = np.clip(bbox[[1, 3]], 0, height - 1)
    return bbox


class CenterTrack(Detector):
    """
    Args:
        model_dir (str): root path of model.pdiparams, model.pdmodel and infer_cfg.yml
        device (str): Choose the device you want to run, it can be: CPU/GPU/XPU/NPU, default is CPU
        run_mode (str): mode of running(paddle/trt_fp32/trt_fp16)
        batch_size (int): size of pre batch in inference
        trt_min_shape (int): min shape for dynamic shape in trt
        trt_max_shape (int): max shape for dynamic shape in trt
        trt_opt_shape (int): opt shape for dynamic shape in trt
        trt_calib_mode (bool): If the model is produced by TRT offline quantitative
            calibration, trt_calib_mode need to set True
        cpu_threads (int): cpu threads
        enable_mkldnn (bool): whether to open MKLDNN
        output_dir (string): The path of output, default as 'output'
        threshold (float): Score threshold of the detected bbox, default as 0.5
        save_images (bool): Whether to save visualization image results, default as False
        save_mot_txts (bool): Whether to save tracking results (txt), default as False
    """

    def __init__(
            self,
            model_dir,
            tracker_config=None,
            device='CPU',
            run_mode='paddle',
            batch_size=1,
            trt_min_shape=1,
            trt_max_shape=960,
            trt_opt_shape=544,
            trt_calib_mode=False,
            cpu_threads=1,
            enable_mkldnn=False,
            output_dir='output',
            threshold=0.5,
            save_images=False,
            save_mot_txts=False, ):
        super(CenterTrack, self).__init__(
            model_dir=model_dir,
            device=device,
            run_mode=run_mode,
            batch_size=batch_size,
            trt_min_shape=trt_min_shape,
            trt_max_shape=trt_max_shape,
            trt_opt_shape=trt_opt_shape,
            trt_calib_mode=trt_calib_mode,
            cpu_threads=cpu_threads,
            enable_mkldnn=enable_mkldnn,
            output_dir=output_dir,
            threshold=threshold, )
        self.save_images = save_images
        self.save_mot_txts = save_mot_txts
        assert batch_size == 1, "MOT model only supports batch_size=1."
        self.det_times = Timer(with_tracker=True)
        self.num_classes = len(self.pred_config.labels)

        # tracker config
        cfg = self.pred_config.tracker
        min_box_area = cfg.get('min_box_area', -1)
        vertical_ratio = cfg.get('vertical_ratio', -1)
        track_thresh = cfg.get('track_thresh', 0.4)
        pre_thresh = cfg.get('pre_thresh', 0.5)

        self.tracker = CenterTracker(
            num_classes=self.num_classes,
            min_box_area=min_box_area,
            vertical_ratio=vertical_ratio,
            track_thresh=track_thresh,
            pre_thresh=pre_thresh)

        self.pre_image = None

    def get_additional_inputs(self, dets, meta, with_hm=True):
        # Render input heatmap from previous trackings.
        trans_input = meta['trans_input']
        inp_width, inp_height = int(meta['inp_width']), int(meta['inp_height'])
        input_hm = np.zeros((1, inp_height, inp_width), dtype=np.float32)

        for det in dets:
            if det['score'] < self.tracker.pre_thresh:
                continue
            bbox = affine_transform_bbox(det['bbox'], trans_input, inp_width,
                                         inp_height)
            h, w = bbox[3] - bbox[1], bbox[2] - bbox[0]
            if (h > 0 and w > 0):
                radius = gaussian_radius(
                    (math.ceil(h), math.ceil(w)), min_overlap=0.7)
                radius = max(0, int(radius))
                ct = np.array(
                    [(bbox[0] + bbox[2]) / 2, (bbox[1] + bbox[3]) / 2],
                    dtype=np.float32)
                ct_int = ct.astype(np.int32)
                if with_hm:
                    input_hm[0] = draw_umich_gaussian(input_hm[0], ct_int,
                                                      radius)
        if with_hm:
            input_hm = input_hm[np.newaxis]
        return input_hm

    def preprocess(self, image_list):
        preprocess_ops = []
        for op_info in self.pred_config.preprocess_infos:
            new_op_info = op_info.copy()
            op_type = new_op_info.pop('type')
            preprocess_ops.append(eval(op_type)(**new_op_info))

        assert len(image_list) == 1, 'MOT only support bs=1'
        im_path = image_list[0]
        im, im_info = preprocess(im_path, preprocess_ops)
        #inputs = create_inputs(im, im_info)
        inputs = {}
        inputs['image'] = np.array((im, )).astype('float32')
        inputs['im_shape'] = np.array((im_info['im_shape'], )).astype('float32')
        inputs['scale_factor'] = np.array(
            (im_info['scale_factor'], )).astype('float32')

        inputs['trans_input'] = im_info['trans_input']
        inputs['inp_width'] = im_info['inp_width']
        inputs['inp_height'] = im_info['inp_height']
        inputs['center'] = im_info['center']
        inputs['scale'] = im_info['scale']
        inputs['out_height'] = im_info['out_height']
        inputs['out_width'] = im_info['out_width']

        if self.pre_image is None:
            self.pre_image = inputs['image']
            # initializing tracker for the first frame
            self.tracker.init_track([])
        inputs['pre_image'] = self.pre_image
        self.pre_image = inputs['image']  # Note: update for next image

        # render input heatmap from tracker status
        pre_hm = self.get_additional_inputs(
            self.tracker.tracks, inputs, with_hm=True)
        inputs['pre_hm'] = pre_hm  #.to_tensor(pre_hm)

        input_names = self.predictor.get_input_names()
        for i in range(len(input_names)):
            input_tensor = self.predictor.get_input_handle(input_names[i])
            if input_names[i] == 'x':
                input_tensor.copy_from_cpu(inputs['image'])
            else:
                input_tensor.copy_from_cpu(inputs[input_names[i]])

        return inputs

    def postprocess(self, inputs, result):
        # postprocess output of predictor
        np_bboxes = result['bboxes']
        if np_bboxes.shape[0] <= 0:
            print('[WARNNING] No object detected and tracked.')
            result = {'bboxes': np.zeros([0, 6]), 'cts': None, 'tracking': None}
            return result
        result = {k: v for k, v in result.items() if v is not None}
        return result

    def centertrack_post_process(self, dets, meta, out_thresh):
        if not ('bboxes' in dets):
            return [{}]

        preds = []
        c, s = meta['center'], meta['scale']
        h, w = meta['out_height'], meta['out_width']
        trans = get_affine_transform(
            center=c,
            input_size=s,
            rot=0,
            output_size=[w, h],
            shift=(0., 0.),
            inv=True).astype(np.float32)
        for i, dets_bbox in enumerate(dets['bboxes']):
            if dets_bbox[1] < out_thresh:
                break
            item = {}
            item['score'] = dets_bbox[1]
            item['class'] = int(dets_bbox[0]) + 1
            item['ct'] = transform_preds_with_trans(
                dets['cts'][i].reshape([1, 2]), trans).reshape(2)

            if 'tracking' in dets:
                tracking = transform_preds_with_trans(
                    (dets['tracking'][i] + dets['cts'][i]).reshape([1, 2]),
                    trans).reshape(2)
                item['tracking'] = tracking - item['ct']

            if 'bboxes' in dets:
                bbox = transform_preds_with_trans(
                    dets_bbox[2:6].reshape([2, 2]), trans).reshape(4)
                item['bbox'] = bbox

            preds.append(item)
        return preds

    def tracking(self, inputs, det_results):
        result = self.centertrack_post_process(det_results, inputs,
                                               self.tracker.out_thresh)
        online_targets = self.tracker.update(result)

        online_tlwhs, online_scores, online_ids = [], [], []
        for t in online_targets:
            bbox = t['bbox']
            tlwh = [bbox[0], bbox[1], bbox[2] - bbox[0], bbox[3] - bbox[1]]
            tscore = float(t['score'])
            tid = int(t['tracking_id'])
            if tlwh[2] * tlwh[3] > 0:
                online_tlwhs.append(tlwh)
                online_ids.append(tid)
                online_scores.append(tscore)
        return online_tlwhs, online_scores, online_ids

    def predict(self, repeats=1):
        '''
        Args:
            repeats (int): repeats number for prediction
        Returns:
            result (dict): include 'bboxes', 'cts' and 'tracking':
                np.ndarray: shape:[N,6],[N,2] and [N,2], N: number of box
        '''
        # model prediction
        np_bboxes, np_cts, np_tracking = None, None, None
        for i in range(repeats):
            self.predictor.run()
            output_names = self.predictor.get_output_names()
            bboxes_tensor = self.predictor.get_output_handle(output_names[0])
            np_bboxes = bboxes_tensor.copy_to_cpu()
            cts_tensor = self.predictor.get_output_handle(output_names[1])
            np_cts = cts_tensor.copy_to_cpu()
            tracking_tensor = self.predictor.get_output_handle(output_names[2])
            np_tracking = tracking_tensor.copy_to_cpu()

        result = dict(bboxes=np_bboxes, cts=np_cts, tracking=np_tracking)
        return result

    def predict_image(self,
                      image_list,
                      run_benchmark=False,
                      repeats=1,
                      visual=True,
                      seq_name=None):
        mot_results = []
        num_classes = self.num_classes
        image_list.sort()
        ids2names = self.pred_config.labels
        data_type = 'mcmot' if num_classes > 1 else 'mot'
        for frame_id, img_file in enumerate(image_list):
            batch_image_list = [img_file]  # bs=1 in MOT model
            if run_benchmark:
                # preprocess
                inputs = self.preprocess(batch_image_list)  # warmup
                self.det_times.preprocess_time_s.start()
                inputs = self.preprocess(batch_image_list)
                self.det_times.preprocess_time_s.end()

                # model prediction
                result_warmup = self.predict(repeats=repeats)  # warmup
                self.det_times.inference_time_s.start()
                result = self.predict(repeats=repeats)
                self.det_times.inference_time_s.end(repeats=repeats)

                # postprocess
                result_warmup = self.postprocess(inputs, result)  # warmup
                self.det_times.postprocess_time_s.start()
                det_result = self.postprocess(inputs, result)
                self.det_times.postprocess_time_s.end()

                # tracking
                result_warmup = self.tracking(inputs, det_result)
                self.det_times.tracking_time_s.start()
                online_tlwhs, online_scores, online_ids = self.tracking(
                    inputs, det_result)
                self.det_times.tracking_time_s.end()
                self.det_times.img_num += 1

                cm, gm, gu = get_current_memory_mb()
                self.cpu_mem += cm
                self.gpu_mem += gm
                self.gpu_util += gu

            else:
                self.det_times.preprocess_time_s.start()
                inputs = self.preprocess(batch_image_list)
                self.det_times.preprocess_time_s.end()

                self.det_times.inference_time_s.start()
                result = self.predict()
                self.det_times.inference_time_s.end()

                self.det_times.postprocess_time_s.start()
                det_result = self.postprocess(inputs, result)
                self.det_times.postprocess_time_s.end()

                # tracking process
                self.det_times.tracking_time_s.start()
                online_tlwhs, online_scores, online_ids = self.tracking(
                    inputs, det_result)
                self.det_times.tracking_time_s.end()
                self.det_times.img_num += 1

            if visual:
                if len(image_list) > 1 and frame_id % 10 == 0:
                    print('Tracking frame {}'.format(frame_id))
                frame, _ = decode_image(img_file, {})

                im = plot_tracking(
                    frame,
                    online_tlwhs,
                    online_ids,
                    online_scores,
                    frame_id=frame_id,
                    ids2names=ids2names)
                if seq_name is None:
                    seq_name = image_list[0].split('/')[-2]
                save_dir = os.path.join(self.output_dir, seq_name)
                if not os.path.exists(save_dir):
                    os.makedirs(save_dir)
                cv2.imwrite(
                    os.path.join(save_dir, '{:05d}.jpg'.format(frame_id)), im)

            mot_results.append([online_tlwhs, online_scores, online_ids])
        return mot_results

    def predict_video(self, video_file, camera_id):
        video_out_name = 'mot_output.mp4'
        if camera_id != -1:
            capture = cv2.VideoCapture(camera_id)
        else:
            capture = cv2.VideoCapture(video_file)
            video_out_name = os.path.split(video_file)[-1]
        # Get Video info : resolution, fps, frame count
        width = int(capture.get(cv2.CAP_PROP_FRAME_WIDTH))
        height = int(capture.get(cv2.CAP_PROP_FRAME_HEIGHT))
        fps = int(capture.get(cv2.CAP_PROP_FPS))
        frame_count = int(capture.get(cv2.CAP_PROP_FRAME_COUNT))
        print("fps: %d, frame_count: %d" % (fps, frame_count))

        if not os.path.exists(self.output_dir):
            os.makedirs(self.output_dir)
        out_path = os.path.join(self.output_dir, video_out_name)
        video_format = 'mp4v'
        fourcc = cv2.VideoWriter_fourcc(*video_format)
        writer = cv2.VideoWriter(out_path, fourcc, fps, (width, height))

        frame_id = 1
        timer = MOTTimer()
        results = defaultdict(list)  # centertrack onpy support single class
        num_classes = self.num_classes
        data_type = 'mcmot' if num_classes > 1 else 'mot'
        ids2names = self.pred_config.labels
        while (1):
            ret, frame = capture.read()
            if not ret:
                break
            if frame_id % 10 == 0:
                print('Tracking frame: %d' % (frame_id))
            frame_id += 1

            timer.tic()
            seq_name = video_out_name.split('.')[0]
            mot_results = self.predict_image(
                [frame[:, :, ::-1]], visual=False, seq_name=seq_name)
            timer.toc()

            fps = 1. / timer.duration
            online_tlwhs, online_scores, online_ids = mot_results[0]
            results[0].append(
                (frame_id + 1, online_tlwhs, online_scores, online_ids))
            im = plot_tracking(
                frame,
                online_tlwhs,
                online_ids,
                online_scores,
                frame_id=frame_id,
                fps=fps,
                ids2names=ids2names)

            writer.write(im)
            if camera_id != -1:
                cv2.imshow('Mask Detection', im)
                if cv2.waitKey(1) & 0xFF == ord('q'):
                    break

        if self.save_mot_txts:
            result_filename = os.path.join(
                self.output_dir, video_out_name.split('.')[-2] + '.txt')

            write_mot_results(result_filename, results, data_type, num_classes)

        writer.release()


def main():
    detector = CenterTrack(
        FLAGS.model_dir,
        tracker_config=None,
        device=FLAGS.device,
        run_mode=FLAGS.run_mode,
        batch_size=1,
        trt_min_shape=FLAGS.trt_min_shape,
        trt_max_shape=FLAGS.trt_max_shape,
        trt_opt_shape=FLAGS.trt_opt_shape,
        trt_calib_mode=FLAGS.trt_calib_mode,
        cpu_threads=FLAGS.cpu_threads,
        enable_mkldnn=FLAGS.enable_mkldnn,
        output_dir=FLAGS.output_dir,
        threshold=FLAGS.threshold,
        save_images=FLAGS.save_images,
        save_mot_txts=FLAGS.save_mot_txts)

    # predict from video file or camera video stream
    if FLAGS.video_file is not None or FLAGS.camera_id != -1:
        detector.predict_video(FLAGS.video_file, FLAGS.camera_id)
    else:
        # predict from image
        img_list = get_test_images(FLAGS.image_dir, FLAGS.image_file)
        detector.predict_image(img_list, FLAGS.run_benchmark, repeats=10)

        if not FLAGS.run_benchmark:
            detector.det_times.info(average=True)
        else:
            mode = FLAGS.run_mode
            model_dir = FLAGS.model_dir
            model_info = {
                'model_name': model_dir.strip('/').split('/')[-1],
                'precision': mode.split('_')[-1]
            }
            bench_log(detector, img_list, model_info, name='MOT')


if __name__ == '__main__':
    paddle.enable_static()
    parser = argsparser()
    FLAGS = parser.parse_args()
    print_arguments(FLAGS)
    FLAGS.device = FLAGS.device.upper()
    assert FLAGS.device in ['CPU', 'GPU', 'XPU', 'NPU'
                            ], "device should be CPU, GPU, NPU or XPU"

    main()
