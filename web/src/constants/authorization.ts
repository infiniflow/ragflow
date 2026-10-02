/*
 *  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

export const Authorization = 'Authorization';
export const Token = 'token';
export const UserInfo = 'userInfo';
export const ThinkingLevel = 'thinkingLevel';

/** Separate localStorage keys for the admin UI (/admin) so it does not collide with the main app session on the same origin. */
export const AdminAuthorization = 'ragflow_admin_Authorization';
export const AdminToken = 'ragflow_admin_token';
export const AdminUserInfo = 'ragflow_admin_userInfo';
