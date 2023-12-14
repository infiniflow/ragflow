# Copyright (c) 2021 PaddlePaddle Authors. All Rights Reserved.
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
import time
import yaml
import cv2
import numpy as np
from collections import defaultdict
import paddle

from benchmark_utils import PaddleInferBenchmark
from preprocess import decode_image
from utils import argsparser, Timer, get_current_memory_mb
from infer import Detector, get_test_images, print_arguments, bench_log, PredictConfig, load_predictor

# add python path
import sys
parent_path = os.path.abspath(os.path.join(__file__, *(['..'] * 2)))
sys.path.insert(0, parent_path)

from pptracking.python.mot import JDETracker, DeepSORTTracker
from pptracking.python.mot.utils import MOTTimer, write_mot_results, get_crops, clip_box
from pptracking.python.mot.visualize import plot_tracking, plot_tracking_dict


class SDE_Detector(Detector):
    """
    Args:
        model_dir (str): root path of model.pdiparams, model.pdmodel and infer_cfg.yml
        tracker_config (str): tracker config path
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
        reid_model_dir (str): reid model dir, default None for ByteTrack, but set for DeepSORT
    """

    def __init__(self,
                 model_dir,
                 tracker_config,
                 device='CPU',
                 run_mode='paddle',
                 batch_size=1,
                 trt_min_shape=1,
                 trt_max_shape=1280,
                 trt_opt_shape=640,
                 trt_calib_mode=False,
                 cpu_threads=1,
                 enable_mkldnn=False,
                 output_dir='output',
                 threshold=0.5,
                 save_images=False,
                 save_mot_txts=False,
                 reid_model_dir=None):
        super(SDE_Detector, self).__init__(
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

        # reid config
        self.use_reid = False if reid_model_dir is None else True
        if self.use_reid:
            self.reid_pred_config = self.set_config(reid_model_dir)
            self.reid_predictor, self.config = load_predictor(
                reid_model_dir,
                run_mode=run_mode,
                batch_size=50,  # reid_batch_size
                min_subgraph_size=self.reid_pred_config.min_subgraph_size,
                device=device,
                use_dynamic_shape=self.reid_pred_config.use_dynamic_shape,
                trt_min_shape=trt_min_shape,
                trt_max_shape=trt_max_shape,
                trt_opt_shape=trt_opt_shape,
                trt_calib_mode=trt_calib_mode,
                cpu_threads=cpu_threads,
                enable_mkldnn=enable_mkldnn)
        else:
            self.reid_pred_config = None
            self.reid_predictor = None

        assert tracker_config is not None, 'Note that tracker_config should be set.'
        self.tracker_config = tracker_config
        tracker_cfg = yaml.safe_load(open(self.tracker_config))
        cfg = tracker_cfg[tracker_cfg['type']]

        # tracker config
        self.use_deepsort_tracker = True if tracker_cfg[
            'type'] == 'DeepSORTTracker' else False
        if self.use_deepsort_tracker:
            # use DeepSORTTracker
            if self.reid_pred_config is not None and hasattr(
                    self.reid_pred_config, 'tracker'):
                cfg = self.reid_pred_config.tracker
            budget = cfg.get('budget', 100)
            max_age = cfg.get('max_age', 30)
            max_iou_distance = cfg.get('max_iou_distance', 0.7)
            matching_threshold = cfg.get('matching_threshold', 0.2)
            min_box_area = cfg.get('min_box_area', 0)
            vertical_ratio = cfg.get('vertical_ratio', 0)

            self.tracker = DeepSORTTracker(
                budget=budget,
                max_age=max_age,
                max_iou_distance=max_iou_distance,
                matching_threshold=matching_threshold,
                min_box_area=min_box_area,
                vertical_ratio=vertical_ratio, )
        else:
            # use ByteTracker
            use_byte = cfg.get('use_byte', False)
            det_thresh = cfg.get('det_thresh', 0.3)
            min_box_area = cfg.get('min_box_area', 0)
            vertical_ratio = cfg.get('vertical_ratio', 0)
            match_thres = cfg.get('match_thres', 0.9)
            conf_thres = cfg.get('conf_thres', 0.6)
            low_conf_thres = cfg.get('low_conf_thres', 0.1)

            self.tracker = JDETracker(
                use_byte=use_byte,
                det_thresh=det_thresh,
                num_classes=self.num_classes,
                min_box_area=min_box_area,
                vertical_ratio=vertical_ratio,
                match_thres=match_thres,
                conf_thres=conf_thres,
                low_conf_thres=low_conf_thres, )

    def postprocess(self, inputs, result):
        # postprocess output of predictor
        np_boxes_num = result['boxes_num']
        if np_boxes_num[0] <= 0:
            print('[WARNNING] No object detected.')
            result = {'boxes': np.zeros([0, 6]), 'boxes_num': [0]}
        result = {k: v for k, v in result.items() if v is not None}
        return result

    def reidprocess(self, det_results, repeats=1):
        pred_dets = det_results['boxes']
        pred_xyxys = pred_dets[:, 2:6]

        ori_image = det_results['ori_image']
        ori_image_shape = ori_image.shape[:2]
        pred_xyxys, keep_idx = clip_box(pred_xyxys, ori_image_shape)

        if len(keep_idx[0]) == 0:
            det_results['boxes'] = np.zeros((1, 6), dtype=np.float32)
            det_results['embeddings'] = None
            return det_results

        pred_dets = pred_dets[keep_idx[0]]
        pred_xyxys = pred_dets[:, 2:6]

        w, h = self.tracker.input_size
        crops = get_crops(pred_xyxys, ori_image, w, h)

        # to keep fast speed, only use topk crops
        crops = crops[:50]  # reid_batch_size
        det_results['crops'] = np.array(crops).astype('float32')
        det_results['boxes'] = pred_dets[:50]

        input_names = self.reid_predictor.get_input_names()
        for i in range(len(input_names)):
            input_tensor = self.reid_predictor.get_input_handle(input_names[i])
            input_tensor.copy_from_cpu(det_results[input_names[i]])

        # model prediction
        for i in range(repeats):
            self.reid_predictor.run()
            output_names = self.reid_predictor.get_output_names()
            feature_tensor = self.reid_predictor.get_output_handle(output_names[
                0])
            pred_embs = feature_tensor.copy_to_cpu()

        det_results['embeddings'] = pred_embs
        return det_results

    def tracking(self, det_results):
        pred_dets = det_results['boxes']  # 'cls_id, score, x0, y0, x1, y1'
        pred_embs = det_results.get('embeddings', None)

        if self.use_deepsort_tracker:
            # use DeepSORTTracker, only support singe class
            self.tracker.predict()
            online_targets = self.tracker.update(pred_dets, pred_embs)
            online_tlwhs, online_scores, online_ids = [], [], []
            for t in online_targets:
                if not t.is_confirmed() or t.time_since_update > 1:
                    continue
                tlwh = t.to_tlwh()
                tscore = t.score
                tid = t.track_id
                if self.tracker.vertical_ratio > 0 and tlwh[2] / tlwh[
                        3] > self.tracker.vertical_ratio:
                    continue
                online_tlwhs.append(tlwh)
                online_scores.append(tscore)
                online_ids.append(tid)

            tracking_outs = {
                'online_tlwhs': online_tlwhs,
                'online_scores': online_scores,
                'online_ids': online_ids,
            }
            return tracking_outs
        else:
            # use ByteTracker, support multiple class
            online_tlwhs = defaultdict(list)
            online_scores = defaultdict(list)
            online_ids = defaultdict(list)
            online_targets_dict = self.tracker.update(pred_dets, pred_embs)
            for cls_id in range(self.num_classes):
                online_targets = online_targets_dict[cls_id]
                for t in online_targets:
                    tlwh = t.tlwh
                    tid = t.track_id
                    tscore = t.score
                    if tlwh[2] * tlwh[3] <= self.tracker.min_box_area:
                        continue
                    if self.tracker.vertical_ratio > 0 and tlwh[2] / tlwh[
                            3] > self.tracker.vertical_ratio:
                        continue
                    online_tlwhs[cls_id].append(tlwh)
                    online_ids[cls_id].append(tid)
                    online_scores[cls_id].append(tscore)

            tracking_outs = {
                'online_tlwhs': online_tlwhs,
                'online_scores': online_scores,
                'online_ids': online_ids,
            }
            return tracking_outs

    def predict_image(self,
                      image_list,
                      run_benchmark=False,
                      repeats=1,
                      visual=True,
                      seq_name=None):
        num_classes = self.num_classes
        image_list.sort()
        ids2names = self.pred_config.labels
        mot_results = []
        for frame_id, img_file in enumerate(image_list):
            batch_image_list = [img_file]  # bs=1 in MOT model
            frame, _ = decode_image(img_file, {})
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
                if self.use_reid:
                    det_result['frame_id'] = frame_id
                    det_result['seq_name'] = seq_name
                    det_result['ori_image'] = frame
                    det_result = self.reidprocess(det_result)
                result_warmup = self.tracking(det_result)
                self.det_times.tracking_time_s.start()
                if self.use_reid:
                    det_result = self.reidprocess(det_result)
                tracking_outs = self.tracking(det_result)
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
                if self.use_reid:
                    det_result['frame_id'] = frame_id
                    det_result['seq_name'] = seq_name
                    det_result['ori_image'] = frame
                    det_result = self.reidprocess(det_result)
                tracking_outs = self.tracking(det_result)
                self.det_times.tracking_time_s.end()
                self.det_times.img_num += 1

            online_tlwhs = tracking_outs['online_tlwhs']
            online_scores = tracking_outs['online_scores']
            online_ids = tracking_outs['online_ids']

            mot_results.append([online_tlwhs, online_scores, online_ids])

            if visual:
                if len(image_list) > 1 and frame_id % 10 == 0:
                    print('Tracking frame {}'.format(frame_id))
                frame, _ = decode_image(img_file, {})
                if isinstance(online_tlwhs, defaultdict):
                    im = plot_tracking_dict(
                        frame,
                        num_classes,
                        online_tlwhs,
                        online_ids,
                        online_scores,
                        frame_id=frame_id,
                        ids2names=ids2names)
                else:
                    im = plot_tracking(
                        frame,
                        online_tlwhs,
                        online_ids,
                        online_scores,
                        frame_id=frame_id,
                        ids2names=ids2names)
                save_dir = os.path.join(self.output_dir, seq_name)
                if not os.path.exists(save_dir):
                    os.makedirs(save_dir)
                cv2.imwrite(
                    os.path.join(save_dir, '{:05d}.jpg'.format(frame_id)), im)

        return mot_results

    def predict_video(self, video_file, camera_id):
        video_out_name = 'output.mp4'
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
        results = defaultdict(list)
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

            # bs=1 in MOT model
            online_tlwhs, online_scores, online_ids = mot_results[0]

            fps = 1. / timer.duration
            if self.use_deepsort_tracker:
                # use DeepSORTTracker, only support singe class
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
            else:
                # use ByteTracker, support multiple class
                for cls_id in range(num_classes):
                    results[cls_id].append(
                        (frame_id + 1, online_tlwhs[cls_id],
                         online_scores[cls_id], online_ids[cls_id]))
                im = plot_tracking_dict(
                    frame,
                    num_classes,
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
            write_mot_results(result_filename, results)

        writer.release()


def main():
    deploy_file = os.path.join(FLAGS.model_dir, 'infer_cfg.yml')
    with open(deploy_file) as f:
        yml_conf = yaml.safe_load(f)
    arch = yml_conf['arch']
    detector = SDE_Detector(
        FLAGS.model_dir,
        tracker_config=FLAGS.tracker_config,
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
        save_mot_txts=FLAGS.save_mot_txts, )

    # predict from video file or camera video stream
    if FLAGS.video_file is not None or FLAGS.camera_id != -1:
        detector.predict_video(FLAGS.video_file, FLAGS.camera_id)
    else:
        # predict from image
        if FLAGS.image_dir is None and FLAGS.image_file is not None:
            assert FLAGS.batch_size == 1, "--batch_size should be 1 in MOT models."
        img_list = get_test_images(FLAGS.image_dir, FLAGS.image_file)
        seq_name = FLAGS.image_dir.split('/')[-1]
        detector.predict_image(
            img_list, FLAGS.run_benchmark, repeats=10, seq_name=seq_name)

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
