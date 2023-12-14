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

from scipy.optimize import linear_sum_assignment
from collections import abc, defaultdict
import cv2
import numpy as np
import math
import paddle
import paddle.nn as nn
from keypoint_preprocess import get_affine_mat_kernel, get_affine_transform


class HrHRNetPostProcess(object):
    """
    HrHRNet postprocess contain:
        1) get topk keypoints in the output heatmap
        2) sample the tagmap's value corresponding to each of the topk coordinate
        3) match different joints to combine to some people with Hungary algorithm
        4) adjust the coordinate by +-0.25 to decrease error std
        5) salvage missing joints by check positivity of heatmap - tagdiff_norm
    Args:
        max_num_people (int): max number of people support in postprocess
        heat_thresh (float): value of topk below this threshhold will be ignored
        tag_thresh (float): coord's value sampled in tagmap below this threshold belong to same people for init

        inputs(list[heatmap]): the output list of model, [heatmap, heatmap_maxpool, tagmap], heatmap_maxpool used to get topk
        original_height, original_width (float): the original image size
    """

    def __init__(self, max_num_people=30, heat_thresh=0.2, tag_thresh=1.):
        self.max_num_people = max_num_people
        self.heat_thresh = heat_thresh
        self.tag_thresh = tag_thresh

    def lerp(self, j, y, x, heatmap):
        H, W = heatmap.shape[-2:]
        left = np.clip(x - 1, 0, W - 1)
        right = np.clip(x + 1, 0, W - 1)
        up = np.clip(y - 1, 0, H - 1)
        down = np.clip(y + 1, 0, H - 1)
        offset_y = np.where(heatmap[j, down, x] > heatmap[j, up, x], 0.25,
                            -0.25)
        offset_x = np.where(heatmap[j, y, right] > heatmap[j, y, left], 0.25,
                            -0.25)
        return offset_y + 0.5, offset_x + 0.5

    def __call__(self, heatmap, tagmap, heat_k, inds_k, original_height,
                 original_width):

        N, J, H, W = heatmap.shape
        assert N == 1, "only support batch size 1"
        heatmap = heatmap[0]
        tagmap = tagmap[0]
        heats = heat_k[0]
        inds_np = inds_k[0]
        y = inds_np // W
        x = inds_np % W
        tags = tagmap[np.arange(J)[None, :].repeat(self.max_num_people),
                      y.flatten(), x.flatten()].reshape(J, -1, tagmap.shape[-1])
        coords = np.stack((y, x), axis=2)
        # threshold
        mask = heats > self.heat_thresh
        # cluster
        cluster = defaultdict(lambda: {
            'coords': np.zeros((J, 2), dtype=np.float32),
            'scores': np.zeros(J, dtype=np.float32),
            'tags': []
        })
        for jid, m in enumerate(mask):
            num_valid = m.sum()
            if num_valid == 0:
                continue
            valid_inds = np.where(m)[0]
            valid_tags = tags[jid, m, :]
            if len(cluster) == 0:  # initialize
                for i in valid_inds:
                    tag = tags[jid, i]
                    key = tag[0]
                    cluster[key]['tags'].append(tag)
                    cluster[key]['scores'][jid] = heats[jid, i]
                    cluster[key]['coords'][jid] = coords[jid, i]
                continue
            candidates = list(cluster.keys())[:self.max_num_people]
            centroids = [
                np.mean(
                    cluster[k]['tags'], axis=0) for k in candidates
            ]
            num_clusters = len(centroids)
            # shape is (num_valid, num_clusters, tag_dim)
            dist = valid_tags[:, None, :] - np.array(centroids)[None, ...]
            l2_dist = np.linalg.norm(dist, ord=2, axis=2)
            # modulate dist with heat value, see `use_detection_val`
            cost = np.round(l2_dist) * 100 - heats[jid, m, None]
            # pad the cost matrix, otherwise new pose are ignored
            if num_valid > num_clusters:
                cost = np.pad(cost, ((0, 0), (0, num_valid - num_clusters)),
                              'constant',
                              constant_values=((0, 0), (0, 1e-10)))
            rows, cols = linear_sum_assignment(cost)
            for y, x in zip(rows, cols):
                tag = tags[jid, y]
                if y < num_valid and x < num_clusters and \
                   l2_dist[y, x] < self.tag_thresh:
                    key = candidates[x]  # merge to cluster
                else:
                    key = tag[0]  # initialize new cluster
                cluster[key]['tags'].append(tag)
                cluster[key]['scores'][jid] = heats[jid, y]
                cluster[key]['coords'][jid] = coords[jid, y]

        # shape is [k, J, 2] and [k, J]
        pose_tags = np.array([cluster[k]['tags'] for k in cluster])
        pose_coords = np.array([cluster[k]['coords'] for k in cluster])
        pose_scores = np.array([cluster[k]['scores'] for k in cluster])
        valid = pose_scores > 0

        pose_kpts = np.zeros((pose_scores.shape[0], J, 3), dtype=np.float32)
        if valid.sum() == 0:
            return pose_kpts, pose_kpts

        # refine coords
        valid_coords = pose_coords[valid].astype(np.int32)
        y = valid_coords[..., 0].flatten()
        x = valid_coords[..., 1].flatten()
        _, j = np.nonzero(valid)
        offsets = self.lerp(j, y, x, heatmap)
        pose_coords[valid, 0] += offsets[0]
        pose_coords[valid, 1] += offsets[1]

        # mean score before salvage
        mean_score = pose_scores.mean(axis=1)
        pose_kpts[valid, 2] = pose_scores[valid]

        # salvage missing joints
        if True:
            for pid, coords in enumerate(pose_coords):
                tag_mean = np.array(pose_tags[pid]).mean(axis=0)
                norm = np.sum((tagmap - tag_mean)**2, axis=3)**0.5
                score = heatmap - np.round(norm)  # (J, H, W)
                flat_score = score.reshape(J, -1)
                max_inds = np.argmax(flat_score, axis=1)
                max_scores = np.max(flat_score, axis=1)
                salvage_joints = (pose_scores[pid] == 0) & (max_scores > 0)
                if salvage_joints.sum() == 0:
                    continue
                y = max_inds[salvage_joints] // W
                x = max_inds[salvage_joints] % W
                offsets = self.lerp(salvage_joints.nonzero()[0], y, x, heatmap)
                y = y.astype(np.float32) + offsets[0]
                x = x.astype(np.float32) + offsets[1]
                pose_coords[pid][salvage_joints, 0] = y
                pose_coords[pid][salvage_joints, 1] = x
                pose_kpts[pid][salvage_joints, 2] = max_scores[salvage_joints]
        pose_kpts[..., :2] = transpred(pose_coords[..., :2][..., ::-1],
                                       original_height, original_width,
                                       min(H, W))
        return pose_kpts, mean_score


def transpred(kpts, h, w, s):
    trans, _ = get_affine_mat_kernel(h, w, s, inv=True)

    return warp_affine_joints(kpts[..., :2].copy(), trans)


def warp_affine_joints(joints, mat):
    """Apply affine transformation defined by the transform matrix on the
    joints.

    Args:
        joints (np.ndarray[..., 2]): Origin coordinate of joints.
        mat (np.ndarray[3, 2]): The affine matrix.

    Returns:
        matrix (np.ndarray[..., 2]): Result coordinate of joints.
    """
    joints = np.array(joints)
    shape = joints.shape
    joints = joints.reshape(-1, 2)
    return np.dot(np.concatenate(
        (joints, joints[:, 0:1] * 0 + 1), axis=1),
                  mat.T).reshape(shape)


class HRNetPostProcess(object):
    def __init__(self, use_dark=True):
        self.use_dark = use_dark

    def flip_back(self, output_flipped, matched_parts):
        assert output_flipped.ndim == 4,\
                'output_flipped should be [batch_size, num_joints, height, width]'

        output_flipped = output_flipped[:, :, :, ::-1]

        for pair in matched_parts:
            tmp = output_flipped[:, pair[0], :, :].copy()
            output_flipped[:, pair[0], :, :] = output_flipped[:, pair[1], :, :]
            output_flipped[:, pair[1], :, :] = tmp

        return output_flipped

    def get_max_preds(self, heatmaps):
        """get predictions from score maps

        Args:
            heatmaps: numpy.ndarray([batch_size, num_joints, height, width])

        Returns:
            preds: numpy.ndarray([batch_size, num_joints, 2]), keypoints coords
            maxvals: numpy.ndarray([batch_size, num_joints, 2]), the maximum confidence of the keypoints
        """
        assert isinstance(heatmaps,
                          np.ndarray), 'heatmaps should be numpy.ndarray'
        assert heatmaps.ndim == 4, 'batch_images should be 4-ndim'

        batch_size = heatmaps.shape[0]
        num_joints = heatmaps.shape[1]
        width = heatmaps.shape[3]
        heatmaps_reshaped = heatmaps.reshape((batch_size, num_joints, -1))
        idx = np.argmax(heatmaps_reshaped, 2)
        maxvals = np.amax(heatmaps_reshaped, 2)

        maxvals = maxvals.reshape((batch_size, num_joints, 1))
        idx = idx.reshape((batch_size, num_joints, 1))

        preds = np.tile(idx, (1, 1, 2)).astype(np.float32)

        preds[:, :, 0] = (preds[:, :, 0]) % width
        preds[:, :, 1] = np.floor((preds[:, :, 1]) / width)

        pred_mask = np.tile(np.greater(maxvals, 0.0), (1, 1, 2))
        pred_mask = pred_mask.astype(np.float32)

        preds *= pred_mask

        return preds, maxvals

    def gaussian_blur(self, heatmap, kernel):
        border = (kernel - 1) // 2
        batch_size = heatmap.shape[0]
        num_joints = heatmap.shape[1]
        height = heatmap.shape[2]
        width = heatmap.shape[3]
        for i in range(batch_size):
            for j in range(num_joints):
                origin_max = np.max(heatmap[i, j])
                dr = np.zeros((height + 2 * border, width + 2 * border))
                dr[border:-border, border:-border] = heatmap[i, j].copy()
                dr = cv2.GaussianBlur(dr, (kernel, kernel), 0)
                heatmap[i, j] = dr[border:-border, border:-border].copy()
                heatmap[i, j] *= origin_max / np.max(heatmap[i, j])
        return heatmap

    def dark_parse(self, hm, coord):
        heatmap_height = hm.shape[0]
        heatmap_width = hm.shape[1]
        px = int(coord[0])
        py = int(coord[1])
        if 1 < px < heatmap_width - 2 and 1 < py < heatmap_height - 2:
            dx = 0.5 * (hm[py][px + 1] - hm[py][px - 1])
            dy = 0.5 * (hm[py + 1][px] - hm[py - 1][px])
            dxx = 0.25 * (hm[py][px + 2] - 2 * hm[py][px] + hm[py][px - 2])
            dxy = 0.25 * (hm[py+1][px+1] - hm[py-1][px+1] - hm[py+1][px-1] \
                + hm[py-1][px-1])
            dyy = 0.25 * (
                hm[py + 2 * 1][px] - 2 * hm[py][px] + hm[py - 2 * 1][px])
            derivative = np.matrix([[dx], [dy]])
            hessian = np.matrix([[dxx, dxy], [dxy, dyy]])
            if dxx * dyy - dxy**2 != 0:
                hessianinv = hessian.I
                offset = -hessianinv * derivative
                offset = np.squeeze(np.array(offset.T), axis=0)
                coord += offset
        return coord

    def dark_postprocess(self, hm, coords, kernelsize):
        """
        refer to https://github.com/ilovepose/DarkPose/lib/core/inference.py

        """
        hm = self.gaussian_blur(hm, kernelsize)
        hm = np.maximum(hm, 1e-10)
        hm = np.log(hm)
        for n in range(coords.shape[0]):
            for p in range(coords.shape[1]):
                coords[n, p] = self.dark_parse(hm[n][p], coords[n][p])
        return coords

    def get_final_preds(self, heatmaps, center, scale, kernelsize=3):
        """the highest heatvalue location with a quarter offset in the
        direction from the highest response to the second highest response.

        Args:
            heatmaps (numpy.ndarray): The predicted heatmaps
            center (numpy.ndarray): The boxes center
            scale (numpy.ndarray): The scale factor

        Returns:
            preds: numpy.ndarray([batch_size, num_joints, 2]), keypoints coords
            maxvals: numpy.ndarray([batch_size, num_joints, 1]), the maximum confidence of the keypoints
        """

        coords, maxvals = self.get_max_preds(heatmaps)

        heatmap_height = heatmaps.shape[2]
        heatmap_width = heatmaps.shape[3]

        if self.use_dark:
            coords = self.dark_postprocess(heatmaps, coords, kernelsize)
        else:
            for n in range(coords.shape[0]):
                for p in range(coords.shape[1]):
                    hm = heatmaps[n][p]
                    px = int(math.floor(coords[n][p][0] + 0.5))
                    py = int(math.floor(coords[n][p][1] + 0.5))
                    if 1 < px < heatmap_width - 1 and 1 < py < heatmap_height - 1:
                        diff = np.array([
                            hm[py][px + 1] - hm[py][px - 1],
                            hm[py + 1][px] - hm[py - 1][px]
                        ])
                        coords[n][p] += np.sign(diff) * .25
        preds = coords.copy()

        # Transform back
        for i in range(coords.shape[0]):
            preds[i] = transform_preds(coords[i], center[i], scale[i],
                                       [heatmap_width, heatmap_height])

        return preds, maxvals

    def __call__(self, output, center, scale):
        preds, maxvals = self.get_final_preds(output, center, scale)
        return np.concatenate(
            (preds, maxvals), axis=-1), np.mean(
                maxvals, axis=1)


def transform_preds(coords, center, scale, output_size):
    target_coords = np.zeros(coords.shape)
    trans = get_affine_transform(center, scale * 200, 0, output_size, inv=1)
    for p in range(coords.shape[0]):
        target_coords[p, 0:2] = affine_transform(coords[p, 0:2], trans)
    return target_coords


def affine_transform(pt, t):
    new_pt = np.array([pt[0], pt[1], 1.]).T
    new_pt = np.dot(t, new_pt)
    return new_pt[:2]


def translate_to_ori_images(keypoint_result, batch_records):
    kpts = keypoint_result['keypoint']
    scores = keypoint_result['score']
    kpts[..., 0] += batch_records[:, 0:1]
    kpts[..., 1] += batch_records[:, 1:2]
    return kpts, scores
