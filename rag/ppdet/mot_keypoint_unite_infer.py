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
import json
import cv2
import math
import numpy as np
import paddle
import yaml
import copy
from collections import defaultdict

from mot_keypoint_unite_utils import argsparser
from preprocess import decode_image
from infer import print_arguments, get_test_images, bench_log
from mot_sde_infer import SDE_Detector
from mot_jde_infer import JDE_Detector, MOT_JDE_SUPPORT_MODELS
from keypoint_infer import KeyPointDetector, KEYPOINT_SUPPORT_MODELS
from det_keypoint_unite_infer import predict_with_given_det
from visualize import visualize_pose
from benchmark_utils import PaddleInferBenchmark
from utils import get_current_memory_mb
from keypoint_postprocess import translate_to_ori_images

# add python path
import sys
parent_path = os.path.abspath(os.path.join(__file__, *(['..'] * 2)))
sys.path.insert(0, parent_path)

from pptracking.python.mot.visualize import plot_tracking, plot_tracking_dict
from pptracking.python.mot.utils import MOTTimer as FPSTimer


def convert_mot_to_det(tlwhs, scores):
    results = {}
    num_mot = len(tlwhs)
    xyxys = copy.deepcopy(tlwhs)
    for xyxy in xyxys.copy():
        xyxy[2:] = xyxy[2:] + xyxy[:2]
    # support single class now
    results['boxes'] = np.vstack(
        [np.hstack([0, scores[i], xyxys[i]]) for i in range(num_mot)])
    results['boxes_num'] = np.array([num_mot])
    return results


def mot_topdown_unite_predict(mot_detector,
                              topdown_keypoint_detector,
                              image_list,
                              keypoint_batch_size=1,
                              save_res=False):
    det_timer = mot_detector.get_timer()
    store_res = []
    image_list.sort()
    num_classes = mot_detector.num_classes
    for i, img_file in enumerate(image_list):
        # Decode image in advance in mot + pose prediction
        det_timer.preprocess_time_s.start()
        image, _ = decode_image(img_file, {})
        det_timer.preprocess_time_s.end()

        if FLAGS.run_benchmark:
            mot_results = mot_detector.predict_image(
                [image], run_benchmark=True, repeats=10)

            cm, gm, gu = get_current_memory_mb()
            mot_detector.cpu_mem += cm
            mot_detector.gpu_mem += gm
            mot_detector.gpu_util += gu
        else:
            mot_results = mot_detector.predict_image([image], visual=False)

        online_tlwhs, online_scores, online_ids = mot_results[
            0]  # only support bs=1 in MOT model
        results = convert_mot_to_det(
            online_tlwhs[0],
            online_scores[0])  # only support single class for mot + pose
        if results['boxes_num'] == 0:
            continue

        keypoint_res = predict_with_given_det(
            image, results, topdown_keypoint_detector, keypoint_batch_size,
            FLAGS.run_benchmark)

        if save_res:
            save_name = img_file if isinstance(img_file, str) else i
            store_res.append([
                save_name, keypoint_res['bbox'],
                [keypoint_res['keypoint'][0], keypoint_res['keypoint'][1]]
            ])
        if FLAGS.run_benchmark:
            cm, gm, gu = get_current_memory_mb()
            topdown_keypoint_detector.cpu_mem += cm
            topdown_keypoint_detector.gpu_mem += gm
            topdown_keypoint_detector.gpu_util += gu
        else:
            if not os.path.exists(FLAGS.output_dir):
                os.makedirs(FLAGS.output_dir)
            visualize_pose(
                img_file,
                keypoint_res,
                visual_thresh=FLAGS.keypoint_threshold,
                save_dir=FLAGS.output_dir)

    if save_res:
        """
        1) store_res: a list of image_data
        2) image_data: [imageid, rects, [keypoints, scores]]
        3) rects: list of rect [xmin, ymin, xmax, ymax]
        4) keypoints: 17(joint numbers)*[x, y, conf], total 51 data in list
        5) scores: mean of all joint conf
        """
        with open("det_keypoint_unite_image_results.json", 'w') as wf:
            json.dump(store_res, wf, indent=4)


def mot_topdown_unite_predict_video(mot_detector,
                                    topdown_keypoint_detector,
                                    camera_id,
                                    keypoint_batch_size=1,
                                    save_res=False):
    video_name = 'output.mp4'
    if camera_id != -1:
        capture = cv2.VideoCapture(camera_id)
    else:
        capture = cv2.VideoCapture(FLAGS.video_file)
        video_name = os.path.split(FLAGS.video_file)[-1]
    # Get Video info : resolution, fps, frame count
    width = int(capture.get(cv2.CAP_PROP_FRAME_WIDTH))
    height = int(capture.get(cv2.CAP_PROP_FRAME_HEIGHT))
    fps = int(capture.get(cv2.CAP_PROP_FPS))
    frame_count = int(capture.get(cv2.CAP_PROP_FRAME_COUNT))
    print("fps: %d, frame_count: %d" % (fps, frame_count))

    if not os.path.exists(FLAGS.output_dir):
        os.makedirs(FLAGS.output_dir)
    out_path = os.path.join(FLAGS.output_dir, video_name)
    fourcc = cv2.VideoWriter_fourcc(* 'mp4v')
    writer = cv2.VideoWriter(out_path, fourcc, fps, (width, height))
    frame_id = 0
    timer_mot, timer_kp, timer_mot_kp = FPSTimer(), FPSTimer(), FPSTimer()

    num_classes = mot_detector.num_classes
    assert num_classes == 1, 'Only one category mot model supported for uniting keypoint deploy.'
    data_type = 'mot'

    while (1):
        ret, frame = capture.read()
        if not ret:
            break
        if frame_id % 10 == 0:
            print('Tracking frame: %d' % (frame_id))
        frame_id += 1
        timer_mot_kp.tic()

        # mot model
        timer_mot.tic()

        frame2 = cv2.cvtColor(frame, cv2.COLOR_BGR2RGB)

        mot_results = mot_detector.predict_image([frame2], visual=False)
        timer_mot.toc()
        online_tlwhs, online_scores, online_ids = mot_results[0]
        results = convert_mot_to_det(
            online_tlwhs[0],
            online_scores[0])  # only support single class for mot + pose
        if results['boxes_num'] == 0:
            continue

        # keypoint model
        timer_kp.tic()
        keypoint_res = predict_with_given_det(
            frame2, results, topdown_keypoint_detector, keypoint_batch_size,
            FLAGS.run_benchmark)
        timer_kp.toc()
        timer_mot_kp.toc()

        kp_fps = 1. / timer_kp.duration
        mot_kp_fps = 1. / timer_mot_kp.duration

        im = visualize_pose(
            frame,
            keypoint_res,
            visual_thresh=FLAGS.keypoint_threshold,
            returnimg=True,
            ids=online_ids[0])

        im = plot_tracking_dict(
            im,
            num_classes,
            online_tlwhs,
            online_ids,
            online_scores,
            frame_id=frame_id,
            fps=mot_kp_fps)

        writer.write(im)
        if camera_id != -1:
            cv2.imshow('Tracking and keypoint results', im)
            if cv2.waitKey(1) & 0xFF == ord('q'):
                break

    writer.release()
    print('output_video saved to: {}'.format(out_path))


def main():
    deploy_file = os.path.join(FLAGS.mot_model_dir, 'infer_cfg.yml')
    with open(deploy_file) as f:
        yml_conf = yaml.safe_load(f)
    arch = yml_conf['arch']
    mot_detector_func = 'SDE_Detector'
    if arch in MOT_JDE_SUPPORT_MODELS:
        mot_detector_func = 'JDE_Detector'

    mot_detector = eval(mot_detector_func)(FLAGS.mot_model_dir,
                                           FLAGS.tracker_config,
                                           device=FLAGS.device,
                                           run_mode=FLAGS.run_mode,
                                           batch_size=1,
                                           trt_min_shape=FLAGS.trt_min_shape,
                                           trt_max_shape=FLAGS.trt_max_shape,
                                           trt_opt_shape=FLAGS.trt_opt_shape,
                                           trt_calib_mode=FLAGS.trt_calib_mode,
                                           cpu_threads=FLAGS.cpu_threads,
                                           enable_mkldnn=FLAGS.enable_mkldnn,
                                           threshold=FLAGS.mot_threshold,
                                           output_dir=FLAGS.output_dir)

    topdown_keypoint_detector = KeyPointDetector(
        FLAGS.keypoint_model_dir,
        device=FLAGS.device,
        run_mode=FLAGS.run_mode,
        batch_size=FLAGS.keypoint_batch_size,
        trt_min_shape=FLAGS.trt_min_shape,
        trt_max_shape=FLAGS.trt_max_shape,
        trt_opt_shape=FLAGS.trt_opt_shape,
        trt_calib_mode=FLAGS.trt_calib_mode,
        cpu_threads=FLAGS.cpu_threads,
        enable_mkldnn=FLAGS.enable_mkldnn,
        threshold=FLAGS.keypoint_threshold,
        output_dir=FLAGS.output_dir,
        use_dark=FLAGS.use_dark)
    keypoint_arch = topdown_keypoint_detector.pred_config.arch
    assert KEYPOINT_SUPPORT_MODELS[
        keypoint_arch] == 'keypoint_topdown', 'MOT-Keypoint unite inference only supports topdown models.'

    # predict from video file or camera video stream
    if FLAGS.video_file is not None or FLAGS.camera_id != -1:
        mot_topdown_unite_predict_video(
            mot_detector, topdown_keypoint_detector, FLAGS.camera_id,
            FLAGS.keypoint_batch_size, FLAGS.save_res)
    else:
        # predict from image
        img_list = get_test_images(FLAGS.image_dir, FLAGS.image_file)
        mot_topdown_unite_predict(mot_detector, topdown_keypoint_detector,
                                  img_list, FLAGS.keypoint_batch_size,
                                  FLAGS.save_res)
        if not FLAGS.run_benchmark:
            mot_detector.det_times.info(average=True)
            topdown_keypoint_detector.det_times.info(average=True)
        else:
            mode = FLAGS.run_mode
            mot_model_dir = FLAGS.mot_model_dir
            mot_model_info = {
                'model_name': mot_model_dir.strip('/').split('/')[-1],
                'precision': mode.split('_')[-1]
            }
            bench_log(mot_detector, img_list, mot_model_info, name='MOT')

            keypoint_model_dir = FLAGS.keypoint_model_dir
            keypoint_model_info = {
                'model_name': keypoint_model_dir.strip('/').split('/')[-1],
                'precision': mode.split('_')[-1]
            }
            bench_log(topdown_keypoint_detector, img_list, keypoint_model_info,
                      FLAGS.keypoint_batch_size, 'KeyPoint')


if __name__ == '__main__':
    paddle.enable_static()
    parser = argsparser()
    FLAGS = parser.parse_args()
    print_arguments(FLAGS)
    FLAGS.device = FLAGS.device.upper()
    assert FLAGS.device in ['CPU', 'GPU', 'XPU', 'NPU'
                            ], "device should be CPU, GPU, NPU or XPU"

    main()
