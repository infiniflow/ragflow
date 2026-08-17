#
#  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
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
from pathlib import Path

from common.i18n import get_locale, msg, parse_accept_language, render_go_msg, resolve_locale, set_locale, t


def test_normalize_display_name_and_bcp47():
    assert resolve_locale(None, "Chinese", None) == "zh-Hans"
    assert resolve_locale(None, "简体中文", None) == "zh-Hans"
    assert resolve_locale(None, "Traditional Chinese", None) == "zh-Hant"
    assert resolve_locale(None, "English", None) == "en"
    assert resolve_locale(None, "zh-Hans", None) == "zh-Hans"
    assert resolve_locale(None, "Portuguese BR", None) == "pt-BR"


def test_parse_accept_language_q_order():
    assert parse_accept_language("zh-CN,zh;q=0.9,en;q=0.8") == "zh-Hans"
    assert parse_accept_language("fr-FR,en;q=0.4") == "fr"
    assert parse_accept_language("en-US") == "en"
    assert parse_accept_language("xx-YY") is None
    assert parse_accept_language("fr;q=0") is None
    assert parse_accept_language("fr;q=0,en;q=0.5") == "en"
    assert parse_accept_language("en;q=1.1,ja;q=0.8") == "ja"


def test_resolve_priority_header_over_user_over_lang():
    assert resolve_locale("ja", "Chinese", "zh_CN.UTF-8") == "ja"
    assert resolve_locale("zh-Hans", "English", "en_US.UTF-8") == "zh-Hans"
    assert resolve_locale(None, "Chinese", "en_US.UTF-8") == "zh-Hans"
    assert resolve_locale(None, "zh-Hans", "C.UTF-8") == "zh-Hans"
    assert resolve_locale(None, None, "zh_CN.UTF-8") == "zh-Hans"
    assert resolve_locale(None, None, "C.UTF-8") == "en"
    assert resolve_locale(None, None, None) == "en"


def test_browser_accept_language_does_not_override_user():
    assert resolve_locale("en-US,en;q=0.9", "zh-Hans", "C.UTF-8") == "zh-Hans"
    assert resolve_locale("en-US", "zh-Hans", "C.UTF-8") == "zh-Hans"
    assert resolve_locale("en-US,en;q=0.9", None, "C.UTF-8") == "en"


def test_t_interpolates_and_falls_back_to_english():
    set_locale("zh-Hans")
    assert t("error.unauthorized") == "未授权！"
    assert t("error.email_not_registered", email="a@b.com") == "邮箱 a@b.com 尚未注册！"
    assert t("error.required", field="dataset_id") == "dataset_id 为必填项"
    missing = t("error.key.that.does.not.exist")
    assert missing == "error.key.that.does.not.exist"
    set_locale("en")
    assert t("error.unauthorized") == "Unauthorized!"
    assert get_locale() == "en"


def test_english_literal_lookup():
    set_locale("zh-Hans")
    assert t("Internal server error") == "内部服务器错误"
    assert t("no authorization") == "无权限"
    dataset_id = "710d27e67c0411f1b9aeed5d4991b280"
    assert t(msg.dataset.not_owned, id=dataset_id) == f"你不是该数据集 {dataset_id} 的所有者"
    assert t(f"You don't own the dataset {dataset_id}.") == f"你不是该数据集 {dataset_id} 的所有者"
    set_locale("en")
    assert t("Internal server error") == "Internal server error"
    assert t(msg.dataset.not_owned, id=dataset_id) == f"You don't own the dataset {dataset_id}."
    set_locale("zh-Hans")
    assert t(msg.document.not_owned, id="doc-1") == "你不是该文档 doc-1 的所有者"
    assert t(msg.chat.not_owned, id="chat-1") == "你不是该聊天 chat-1 的所有者"
    assert t(msg.agent.not_owned, id="agent-1") == "你不是该 Agent agent-1 的所有者"
    assert t(msg.dataset.not_found_id, id="kb-1") == "找不到 ID 为 kb-1 的数据集！"
    assert t(msg.dataset.not_found) == "找不到此数据集！"
    assert t(msg.dataset.document_not_owned) == "该数据集不包含此文档"
    assert t(msg.connector.not_found) == "找不到此连接器！"
    assert t(msg.search.not_found) == "找不到此搜索应用！"
    assert t(msg.folder.not_found) == "找不到此文件夹！"
    assert t(msg.dataset.parsed_file_not_owned, id="kb-1") == "数据集 kb-1 没有已解析的文件"
    assert t(msg.chat.session_not_owned, id="sid-1") == "该聊天不包含会话 sid-1"
    assert t("Document abc not found") == "未找到文档 abc"


def test_msg_enum_from_catalog():
    assert str(msg.dataset.not_owned) == "error.dataset.not_owned"
    assert str(msg.dataset.not_found) == "error.dataset.not_found"
    assert str(msg.document.not_owned) == "error.document.not_owned"
    assert str(msg.unauthorized) == "error.unauthorized"
    try:
        msg.dataset.not_own
        raise AssertionError("typo should raise AttributeError")
    except AttributeError as exc:
        assert "not_own" in str(exc)


def test_go_msg_file_matches_catalog():
    path = Path(__file__).resolve().parents[3] / "internal" / "i18n" / "msg.go"
    assert path.read_text(encoding="utf-8") == render_go_msg()


def test_error_code_and_argument_catalogs_are_translated():
    set_locale("en")
    assert t("error.code.timeout") == "Timeout"
    assert t("error.invalid_argument_values", fields="limit={1,2}") == "required argument values: limit={1,2}"
    for loc in ("zh-Hans", "zh-Hant", "ar", "de", "es", "fr", "id", "ja"):
        set_locale(loc)
        assert t("error.code.timeout") != "Timeout"
        assert t("error.code.timeout")
        assert t("error.invalid_argument_values", fields="x") != "required argument values: x"
    set_locale("zh-Hans")
    assert t("Commit not found in workspace") == "工作区中找不到该提交"
    set_locale("zh-Hant")
    assert t("Commit not found in workspace") == "工作區中找不到該提交"
    set_locale("en")

